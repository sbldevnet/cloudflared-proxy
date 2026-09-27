package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
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

	// calling reaches zero only once every goroutine below is about to call
	// triggerRenewal; release, closed only afterwards, keeps whichever one
	// becomes the leader blocked in fetch until then, so every other one
	// joins that single in-flight call instead of starting its own.
	var calling sync.WaitGroup
	calling.Add(concurrent)

	var wg sync.WaitGroup
	wg.Add(concurrent)
	for range concurrent {
		go func() {
			defer wg.Done()
			calling.Done()
			tm.triggerRenewal(context.Background())
		}()
	}

	calling.Wait()
	close(release)
	wg.Wait()

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
