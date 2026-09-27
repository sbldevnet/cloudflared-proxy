package proxy

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// loginCooldown keeps a failed or abandoned renewal from reopening a browser
// login prompt on every subsequent rejected request.
const loginCooldown = 30 * time.Second

// tokenHolder is a concurrency-safe holder for one target's current Access
// token, read by the director on every request and swapped by tokenManager
// whenever the token is renewed.
type tokenHolder struct {
	ptr atomic.Pointer[string]
}

func newTokenHolder(value string) *tokenHolder {
	h := &tokenHolder{}
	h.store(value)
	return h
}

func (h *tokenHolder) load() string { return *h.ptr.Load() }

func (h *tokenHolder) store(v string) { h.ptr.Store(&v) }

// tokenFetchFunc obtains a fresh Access token for an address, as
// cloudflared.CloudflareAccessTokenForApp does.
type tokenFetchFunc func(ctx context.Context, address string) (string, error)

// tokenManager renews one target's tokenHolder when triggered by the edge
// rejecting a request (see newModifyResponse). A singleflight.Group coalesces
// concurrent triggers into one fetch, and a cooldown after a failed fetch
// keeps a login prompt from reopening on every subsequent rejected request.
type tokenManager struct {
	address string
	fetch   tokenFetchFunc
	holder  *tokenHolder
	log     *slog.Logger

	group singleflight.Group

	mu          sync.Mutex
	cooldownEnd time.Time
}

func newTokenManager(address string, fetch tokenFetchFunc, holder *tokenHolder, log *slog.Logger) *tokenManager {
	return &tokenManager{address: address, fetch: fetch, holder: holder, log: log}
}

// renewGroupKey is the singleflight.Group key used for the single renewal
// call each tokenManager ever has in flight at a time.
const renewGroupKey = "renew"

// triggerRenewal fetches a fresh token and stores it, unless a previous
// attempt is still in its cooldown. Concurrent callers are coalesced onto a
// single fetch.
func (m *tokenManager) triggerRenewal(ctx context.Context) {
	m.mu.Lock()
	inCooldown := time.Now().Before(m.cooldownEnd)
	m.mu.Unlock()
	if inCooldown {
		return
	}

	_, _, _ = m.group.Do(renewGroupKey, func() (any, error) { return m.renewOnce(ctx) })
}

// renewOnce fetches a fresh token and stores it, or starts a cooldown if it
// failed. It is meant to run behind m.group, so it executes at most once per
// in-flight renewal regardless of how many callers triggered it.
func (m *tokenManager) renewOnce(ctx context.Context) (any, error) {
	m.log.Debug("renewing Access token", "address", m.address)
	token, err := m.fetch(ctx, m.address)
	if err != nil {
		if ctx.Err() == nil {
			m.log.Error("failed to renew Access token", "address", m.address, "error", err)
			m.mu.Lock()
			m.cooldownEnd = time.Now().Add(loginCooldown)
			m.mu.Unlock()
		}
		return nil, err
	}

	m.holder.store(token)
	m.log.Debug("renewed Access token", "address", m.address)
	return nil, nil
}
