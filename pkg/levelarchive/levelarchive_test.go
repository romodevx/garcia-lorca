package levelarchive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

func TestCountsSortedIsDeterministic(t *testing.T) {
	c := Counts{"b": 2, "a": 1, "c": 0}
	got := c.Sorted()
	if len(got) != 3 || got[0].Kind != "a" || got[1].Kind != "b" || got[2].Kind != "c" {
		t.Fatalf("Sorted() = %+v, want a,b,c", got)
	}
}

func TestCountsAddMerges(t *testing.T) {
	c := Counts{"a": 1}
	c.Add(Counts{"a": 2, "b": 3})
	if c["a"] != 3 || c["b"] != 3 {
		t.Fatalf("Add() = %+v", c)
	}
}

func TestPurgeCutoffIsSevenDaysBack(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if got, want := Cutoff(now), now.Add(-7*24*time.Hour); !got.Equal(want) {
		t.Fatalf("Cutoff = %v, want %v", got, want)
	}
}

func TestPurgerRunsAtStartAndOnEveryTickUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	calls := make(chan struct{}, 4)
	done := make(chan struct{})
	go func() {
		runPurger(ctx, tick, func(context.Context) (int64, error) {
			calls <- struct{}{}
			return 1, nil
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	<-calls // at start
	tick <- time.Now()
	<-calls // on the tick
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("purger did not stop on cancel")
	}
}

func TestPurgerSurvivesAFailedRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tick := make(chan time.Time)
	calls := make(chan struct{}, 4)
	go runPurger(ctx, tick, func(context.Context) (int64, error) {
		calls <- struct{}{}
		return 0, errors.New("db down")
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	<-calls
	tick <- time.Now()
	<-calls
}

func TestLevelBeingDeletedIsACodedConflict(t *testing.T) {
	err := LevelBeingDeleted()
	if got := CodeOf(fmt.Errorf("wrapped: %w", err)); got != CodeLevelBeingDeleted {
		t.Fatalf("CodeOf = %q, want %q", got, CodeLevelBeingDeleted)
	}
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Fatalf("kind = %v, want conflict", glerrors.KindOf(err))
	}
	if msg := glerrors.SafeMessage(err); msg != "Este nivel se está eliminando" {
		t.Fatalf("message = %q", msg)
	}
	if CodeOf(errors.New("plain")) != "" {
		t.Fatal("a plain error carries no code")
	}
}

func TestBatchNotFoundIsNotFound(t *testing.T) {
	if glerrors.KindOf(ErrBatchNotFound) != glerrors.KindNotFound {
		t.Fatal("a missing archive must be NotFound")
	}
}
