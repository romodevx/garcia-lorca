package expo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/garcia-lorca/services/comms/internal/service"
)

const (
	defaultReceiptsBaseURL = "https://exp.host"
	receiptsPath           = "/--/api/v2/push/getReceipts"
	maxReceiptBatch        = 1000
	maxReceiptAttempts     = 5
	minReceiptAge          = 15 * time.Minute
	receiptTimeout         = 10 * time.Second
	defaultReceiptInterval = time.Minute
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
		store: store, baseURL: defaultReceiptsBaseURL,
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
	cutoff := c.now().Add(-minReceiptAge)
	afterID := ""
	for {
		tickets, err := c.store.ListPushTickets(ctx, cutoff, afterID, maxReceiptBatch)
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
			if err := c.store.MarkPushTicketChecked(ctx, t.ID, checkedAt); err != nil {
				return fmt.Errorf("marking pending expo receipt checked: %w", err)
			}
			continue
		}
		if r.Status == "ok" {
			if err := c.store.DeletePushTicket(ctx, t.ID); err != nil {
				return fmt.Errorf("deleting completed expo ticket: %w", err)
			}
			continue
		}
		if r.Details.Error == errDeviceNotRegistered {
			if err := c.store.DeletePushDevice(ctx, t.DeviceToken); err != nil {
				return fmt.Errorf("deleting unregistered expo device: %w", err)
			}
			if err := c.store.DeletePushTicket(ctx, t.ID); err != nil {
				return fmt.Errorf("deleting unregistered expo ticket: %w", err)
			}
			continue
		}
		attempts, err := c.store.RecordPushTicketFailure(ctx, t.ID, maxReceiptAttempts, checkedAt)
		if err != nil {
			return fmt.Errorf("recording expo receipt failure: %w", err)
		}
		c.log.Warn("expo receipt reported a delivery error", slog.String("error_code", r.Details.Error), slog.Int("attempts", attempts))
		if attempts >= maxReceiptAttempts {
			if err := c.store.DeletePushTicket(ctx, t.ID); err != nil {
				return fmt.Errorf("deleting exhausted expo ticket: %w", err)
			}
		}
	}
	return nil
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
