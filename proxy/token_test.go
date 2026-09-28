package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
)

func TestTokenHolder(t *testing.T) {
	h := newTokenHolder("initial")
	assert.Equal(t, "initial", h.load())

	h.store("updated")
	assert.Equal(t, "updated", h.load())
}

func TestTokenManagerTriggerRenewalSingleFlight(t *testing.T) {
	const concurrent = 10

	var calls int32
	release := make(chan struct{})
	fetch := func(ctx context.Context, address string) (string, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return "new-token", nil
	}

	holder := newTokenHolder("old-token")
	tm := newTokenManager("app.example.com:443", fetch, holder, slog.New(slog.DiscardHandler))

	// DoChan registers a call and returns immediately without blocking the
	// caller (the first one starts renewOnce in its own goroutine; every
	// later one just joins it) — see golang.org/x/sync/singleflight's
	// implementation. Calling it here in a plain sequential loop, before
	// release is closed, therefore deterministically joins every one of
	// these onto the same in-flight fetch: none of them can have returned
	// yet, since fetch is still blocked on release. This avoids depending on
	// goroutine-scheduling timing, which a concurrent-goroutines version of
	// this test previously did and which could flake (each goroutine calling
	// triggerRenewal after signalling readiness, but before actually
	// reaching singleflight.Group.Do, racing against the leader's fetch
	// already unblocking).
	ctx := context.Background()
	results := make([]<-chan singleflight.Result, concurrent)
	for i := range results {
		results[i] = tm.group.DoChan(renewGroupKey, func() (any, error) { return tm.renewOnce(ctx) })
	}

	close(release)

	for _, res := range results {
		r := <-res
		require.NoError(t, r.Err)
	}

	assert.Equal(t, int32(1), calls, "concurrent triggers should be coalesced into one fetch")
	assert.Equal(t, "new-token", holder.load())
}

func TestTokenManagerRenewsAgainAfterSuccess(t *testing.T) {
	var calls int32
	fetch := func(ctx context.Context, address string) (string, error) {
		n := atomic.AddInt32(&calls, 1)
		return fmt.Sprintf("token-%d", n), nil
	}
	holder := newTokenHolder("old-token")
	tm := newTokenManager("app.example.com:443", fetch, holder, slog.New(slog.DiscardHandler))

	tm.triggerRenewal(context.Background())
	tm.triggerRenewal(context.Background())

	assert.Equal(t, int32(2), calls)
	assert.Equal(t, "token-2", holder.load())
}

func TestTokenManagerCooldownAfterFailure(t *testing.T) {
	var calls int32
	fetch := func(ctx context.Context, address string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", errors.New("login failed")
	}
	holder := newTokenHolder("old-token")
	tm := newTokenManager("app.example.com:443", fetch, holder, slog.New(slog.DiscardHandler))

	tm.triggerRenewal(context.Background())
	tm.triggerRenewal(context.Background())

	assert.Equal(t, int32(1), calls, "a second trigger during the cooldown should not fetch again")
	assert.Equal(t, "old-token", holder.load(), "a failed renewal must not clear the still-usable token")
}

func TestTokenManagerCancelledContextSkipsCooldown(t *testing.T) {
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fetch := func(ctx context.Context, address string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", ctx.Err()
	}
	holder := newTokenHolder("old-token")
	tm := newTokenManager("app.example.com:443", fetch, holder, slog.New(slog.DiscardHandler))

	tm.triggerRenewal(ctx)
	tm.triggerRenewal(context.Background())

	assert.Equal(t, int32(2), calls, "a shutdown-cancelled attempt must not start a cooldown for later runs")
}

func TestTokenManagerDoesNotLogTokenValue(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fetch := func(ctx context.Context, address string) (string, error) {
		return "super-secret-token", nil
	}
	holder := newTokenHolder("old-secret-token")
	tm := newTokenManager("app.example.com:443", fetch, holder, log)

	tm.triggerRenewal(context.Background())

	assert.NotEmpty(t, buf.String())
	assert.NotContains(t, buf.String(), "super-secret-token")
	assert.NotContains(t, buf.String(), "old-secret-token")
}
