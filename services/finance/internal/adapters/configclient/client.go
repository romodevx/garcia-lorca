// Package configclient implements the finance CycleReader port over a Connect
// client to the config (academic catalog) bounded context. Every charge is
// stamped with the school cycle it belongs to, so the ledger validates that
// reference by RPC while staying testable with a fake, and this adapter is the
// only place in finance the generated config proto types are touched.
//
// Error mapping mirrors the port contract: a config NotFound (the cycle does not
// exist) becomes glerrors.ErrNotFound so the use case can turn it into a
// KindInvalid ("cycle not found"); any other transport failure is wrapped as
// KindInternal (no config-internal detail leaks upward).
package configclient

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// Compile-time proof the adapter satisfies the consumer-defined port.
var _ ports.CycleReader = (*Client)(nil)

// And of the calendar port: config owns the school calendar, finance reads it.
var _ ports.CalendarReader = (*Client)(nil)

// Client reads school cycles from config by RPC. It depends only on the
// generated Connect client and translates to the small, finance-owned CycleRef
// view the port speaks in.
type Client struct {
	rpc configv1connect.ConfigServiceClient
}

// New builds a Client targeting the config base URL over httpClient. A nil
// httpClient falls back to http.DefaultClient (mirrors the grades config
// client).
func New(baseURL string, httpClient connect.HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{rpc: configv1connect.NewConfigServiceClient(httpClient, baseURL)}
}

// ByID fetches a school cycle by id and projects it to the finance-owned view —
// its identity is all the ledger needs to stamp a charge. A missing cycle is
// glerrors.ErrNotFound.
func (c *Client) ByID(ctx context.Context, id string) (ports.CycleRef, error) {
	res, err := c.rpc.GetCycle(ctx, connect.NewRequest(&configv1.GetCycleRequest{Id: id}))
	if err != nil {
		return ports.CycleRef{}, mapConfigErr(err, "cycle")
	}
	cycle := res.Msg.GetCycle()
	return ports.CycleRef{
		ID:       cycle.GetId(),
		StartsOn: cycle.GetStartsOn().AsTime(),
	}, nil
}

// ListLevels returns the school's levels. Finance reads them only to report the
// ones nobody has priced yet — by name, because the reader is a person.
func (c *Client) ListLevels(ctx context.Context) ([]ports.LevelRef, error) {
	res, err := c.rpc.ListLevels(ctx, connect.NewRequest(&configv1.ListLevelsRequest{}))
	if err != nil {
		return nil, mapConfigErr(err, "levels")
	}
	out := make([]ports.LevelRef, 0, len(res.Msg.GetLevels()))
	for _, l := range res.Msg.GetLevels() {
		out = append(out, ports.LevelRef{ID: l.GetId(), Name: l.GetName()})
	}
	return out, nil
}

// GroupLevels resolves every group of a cycle to the level it belongs to.
//
// It is three reads and not one, because config exposes the path in pieces:
// groups name a GRADE, grades name a LEVEL, and grades are listed per level. The
// alternative — a round trip per enrolment during the billing run — would turn a
// school of six hundred into six hundred RPCs.
func (c *Client) GroupLevels(ctx context.Context, cycleID string) ([]ports.GroupLevel, error) {
	levels, err := c.ListLevels(ctx)
	if err != nil {
		return nil, err
	}
	levelOfGrade := map[string]string{}
	for _, level := range levels {
		res, err := c.rpc.ListGradeLevels(ctx, connect.NewRequest(&configv1.ListGradeLevelsRequest{
			LevelId: level.ID,
		}))
		if err != nil {
			return nil, mapConfigErr(err, "grade levels")
		}
		for _, g := range res.Msg.GetGradeLevels() {
			levelOfGrade[g.GetId()] = level.ID
		}
	}

	res, err := c.rpc.ListGroups(ctx, connect.NewRequest(&configv1.ListGroupsRequest{CycleId: cycleID}))
	if err != nil {
		return nil, mapConfigErr(err, "groups")
	}
	out := make([]ports.GroupLevel, 0, len(res.Msg.GetGroups()))
	for _, g := range res.Msg.GetGroups() {
		levelID, ok := levelOfGrade[g.GetGradeLevelId()]
		if !ok {
			// A group whose grade config no longer lists. Skipping it means its
			// pupils go unbilled and get reported as a level without a plan,
			// which is louder than a silent zero-peso charge.
			continue
		}
		out = append(out, ports.GroupLevel{GroupID: g.GetId(), LevelID: levelID})
	}
	return out, nil
}

// mapConfigErr translates a Connect transport error to the port's error
// vocabulary: NotFound → the shared glerrors.ErrNotFound sentinel; anything else
// → KindInternal wrapping the cause (the caller never sees config-internal
// detail beyond the wrapped error's own message).
func mapConfigErr(err error, what string) error {
	if connect.CodeOf(err) == connect.CodeNotFound {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "config "+what+" lookup failed", err)
}

// NonWorkingDays reads the cycle's closed periods from config and expands them
// into individual days, which is the shape the working-day arithmetic wants.
// config stores ranges because that is how a person enters "vacaciones de
// invierno"; nobody downstream of here cares that it was one decision.
//
// A failure comes back as an error rather than an empty calendar: an empty
// calendar is a claim that the school never closes, and CachedCalendar is the
// one place allowed to decide what to do when the read fails.
func (c *Client) NonWorkingDays(ctx context.Context, cycleID string) (ports.CalendarSnapshot, error) {
	res, err := c.rpc.ListNonWorkingPeriods(ctx, connect.NewRequest(&configv1.ListNonWorkingPeriodsRequest{
		CycleId: cycleID,
	}))
	if err != nil {
		return ports.CalendarSnapshot{}, mapConfigErr(err, "school calendar")
	}
	var days []time.Time
	for _, p := range res.Msg.GetPeriods() {
		from, ferr := time.Parse(calendarDateLayout, p.GetStartsOn())
		to, terr := time.Parse(calendarDateLayout, p.GetEndsOn())
		if ferr != nil || terr != nil {
			// config is the only writer and validates on the way in, so this is
			// a contract break rather than bad user input. Skipping the period
			// would silently shorten the calendar; say so instead.
			return ports.CalendarSnapshot{}, glerrors.New(glerrors.KindInternal,
				"config returned a non-working period with an unreadable date")
		}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			days = append(days, d)
		}
	}
	return ports.CalendarSnapshot{Days: days}, nil
}

// calendarDateLayout matches how config puts a calendar day on the wire.
const calendarDateLayout = "2006-01-02"
