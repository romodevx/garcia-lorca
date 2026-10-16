package expo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/garcia-lorca/services/comms/internal/service"
)

const (
	// DefaultReceiptsBaseURL is the production Expo API host used by receipts.
	DefaultReceiptsBaseURL = "https://exp.host"
	receiptsPath           = "/--/api/v2/push/getReceipts"
	maxReceiptBatch        = 1000
	maxReceiptAttempts     = 5
	minReceiptAge          = 15 * time.Minute
	receiptRetention       = 24 * time.Hour
	receiptTimeout         = 10 * time.Second
	defaultReceiptInterval = time.Minute
	ticketStatusOK         = "ok"
)

// ReceiptChecker polls Expo receipts for successful push tickets.
type ReceiptChecker struct {
	store       service.PushReceiptStore
	baseURL     string
	accessToken string
	client      *http.Client
	log         *slog.Logger
	now         func() time.Time
}

// ReceiptOption configures a ReceiptChecker.
type ReceiptOption func(*ReceiptChecker)

// WithReceiptsBaseURL changes the Expo API base URL, primarily for tests.
func WithReceiptsBaseURL(url string) ReceiptOption {
	return func(c *ReceiptChecker) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithReceiptAccessToken configures optional Expo enhanced push authorization.
func WithReceiptAccessToken(token string) ReceiptOption {
	return func(c *ReceiptChecker) { c.accessToken = token }
}

// WithReceiptLogger sets the logger used for receipt failures.
func WithReceiptLogger(log *slog.Logger) ReceiptOption {
	return func(c *ReceiptChecker) {
		if log != nil {
			c.log = log
		}
	}
}

// WithReceiptClock injects the current time source.
func WithReceiptClock(now func() time.Time) ReceiptOption {
	return func(c *ReceiptChecker) {
		if now != nil {
			c.now = now
		}
	}
}

// NewReceiptChecker builds a checker using Expo's production endpoint by default.
func NewReceiptChecker(store service.PushReceiptStore, opts ...ReceiptOption) *ReceiptChecker {
	c := &ReceiptChecker{
		store: store, baseURL: DefaultReceiptsBaseURL,
		client: &http.Client{Timeout: receiptTimeout}, log: slog.Default(), now: time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type receipt struct {
	Status  string `json:"status"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type receiptsResponse struct {
	Data map[string]receipt `json:"data"`
}

// Check processes all tickets at least 15 minutes old in bounded requests.
func (c *ReceiptChecker) Check(ctx context.Context) error {
	now := c.now()
	if err := c.store.DeleteExpiredPushTickets(ctx, now.Add(-receiptRetention)); err != nil {
		return fmt.Errorf("deleting expired expo push tickets: %w", err)
	}
	cutoff := now.Add(-minReceiptAge)
	afterID := ""
	var afterCreatedAt time.Time
	for {
		tickets, err := c.store.ListPushTickets(ctx, cutoff, afterCreatedAt, afterID, maxReceiptBatch)
		if err != nil {
			return fmt.Errorf("listing expo push tickets: %w", err)
		}
		if len(tickets) == 0 {
			return nil
		}
		if err := c.checkBatch(ctx, tickets); err != nil {
			return err
		}
		afterID = tickets[len(tickets)-1].ID
		afterCreatedAt = tickets[len(tickets)-1].CreatedAt
		if len(tickets) < maxReceiptBatch {
			return nil
		}
	}
}

func (c *ReceiptChecker) checkBatch(ctx context.Context, tickets []service.PushTicket) error {
	ids := make([]string, 0, len(tickets))
	for _, t := range tickets {
		ids = append(ids, t.TicketID)
	}
	body, err := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{IDs: ids})
	if err != nil {
		return fmt.Errorf("encoding expo receipt request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+receiptsPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building expo receipt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("requesting expo receipts: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("expo receipts refused: HTTP %d", res.StatusCode)
	}
	var parsed receiptsResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("decoding expo receipts: %w", err)
	}
	checkedAt := c.now()
	for _, t := range tickets {
		r, found := parsed.Data[t.TicketID]
		if !found { // Expo may not have produced the receipt yet.
			attempts, err := c.store.RecordPushTicketFailure(ctx, t.ID, checkedAt)
			if err != nil {
				return fmt.Errorf("recording missing expo receipt: %w", err)
			}
			if attempts >= maxReceiptAttempts {
				if err := c.store.DeletePushTicket(ctx, t.ID); err != nil {
					return fmt.Errorf("deleting exhausted expo ticket: %w", err)
				}
			}
			continue
		}
		if r.Status == ticketStatusOK {
			if err := c.store.DeletePushTicket(ctx, t.ID); err != nil {
				return fmt.Errorf("deleting completed expo ticket: %w", err)
			}
			continue
		}
		c.log.Warn("expo receipt reported a delivery error", slog.String("error_code", r.Details.Error))
		if r.Details.Error == errDeviceNotRegistered {
			deviceErr := c.store.DeletePushDevice(ctx, t.DeviceToken)
			ticketErr := c.store.DeletePushTicket(ctx, t.ID)
			if deviceErr != nil || ticketErr != nil {
				return errors.Join(
					wrapIfError("deleting unregistered expo device", deviceErr),
					wrapIfError("deleting unregistered expo ticket", ticketErr),
				)
			}
			continue
		}
		if err := c.store.DeletePushTicket(ctx, t.ID); err != nil {
			return fmt.Errorf("deleting failed expo ticket: %w", err)
		}
	}
	return nil
}

func wrapIfError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// Run performs one startup sweep, then checks periodically until ctx is cancelled.
func (c *ReceiptChecker) Run(ctx context.Context) {
	if err := c.Check(ctx); err != nil && ctx.Err() == nil {
		c.log.Warn("expo receipt sweep failed", slog.Any("error", err))
	}
	ticker := time.NewTicker(defaultReceiptInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Check(ctx); err != nil && ctx.Err() == nil {
				c.log.Warn("expo receipt sweep failed", slog.Any("error", err))
			}
		}
	}
}
