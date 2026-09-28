package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAccessLoginRedirect(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		loc    string
		want   bool
	}{
		{"302 to cloudflareaccess.com", http.StatusFound, "https://myteam.cloudflareaccess.com/cdn-cgi/access/login", true},
		{"401 to cloudflareaccess.com", http.StatusUnauthorized, "https://myteam.cloudflareaccess.com/cdn-cgi/access/login", true},
		{"403 to cloudflareaccess.com", http.StatusForbidden, "https://myteam.cloudflareaccess.com/cdn-cgi/access/login", true},
		{"exact cloudflareaccess.com host", http.StatusFound, "https://cloudflareaccess.com/login", true},
		{"302 to an unrelated host", http.StatusFound, "https://example.com/login", false},
		{"302 to a lookalike host", http.StatusFound, "https://notcloudflareaccess.com/login", false},
		{"200 with the same location header", http.StatusOK, "https://myteam.cloudflareaccess.com/cdn-cgi/access/login", false},
		{"302 without a location", http.StatusFound, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status, Header: http.Header{}}
			if tc.loc != "" {
				resp.Header.Set("Location", tc.loc)
			}
			assert.Equal(t, tc.want, isAccessLoginRedirect(resp))
		})
	}
}

func TestAcceptsHTML(t *testing.T) {
	htmlReq := httptest.NewRequest("GET", "http://localhost/", nil)
	htmlReq.Header.Set("Accept", "text/html,application/xhtml+xml")
	nonHTMLReq := httptest.NewRequest("GET", "http://localhost/", nil)
	nonHTMLReq.Header.Set("Accept", "*/*")
	noAcceptReq := httptest.NewRequest("GET", "http://localhost/", nil)

	assert.True(t, acceptsHTML(htmlReq))
	assert.False(t, acceptsHTML(nonHTMLReq))
	assert.False(t, acceptsHTML(noAcceptReq))
	assert.False(t, acceptsHTML(nil))
}

func waitForToken(t *testing.T, holder *tokenHolder, want string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if holder.load() == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("token was never renewed to %q, got %q", want, holder.load())
		case <-time.After(time.Millisecond):
		}
	}
}

func TestNewModifyResponseTriggersRenewalAndRewritesForNonBrowserClient(t *testing.T) {
	holder := newTokenHolder("old-token")
	fetch := func(ctx context.Context, address string) (string, error) { return "new-token", nil }
	tm := newTokenManager("app.example.com:443", fetch, holder, slog.New(slog.DiscardHandler))
	mr := newModifyResponse(context.Background(), slog.New(slog.DiscardHandler), "app.example.com:443", tm)

	req := httptest.NewRequest("GET", "http://localhost/", nil) // no Accept header: a curl/git-like client
	resp := &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"https://myteam.cloudflareaccess.com/cdn-cgi/access/login"}},
		Body:       io.NopCloser(strings.NewReader("<html>login</html>")),
		Request:    req,
	}

	require.NoError(t, mr(resp))

	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "session for this target expired")
	assert.NotContains(t, string(body), "<html>")

	waitForToken(t, holder, "new-token")
}

func TestNewModifyResponsePassesThroughForBrowserClient(t *testing.T) {
	holder := newTokenHolder("old-token")
	fetch := func(ctx context.Context, address string) (string, error) { return "new-token", nil }
	tm := newTokenManager("app.example.com:443", fetch, holder, slog.New(slog.DiscardHandler))
	mr := newModifyResponse(context.Background(), slog.New(slog.DiscardHandler), "app.example.com:443", tm)

	req := httptest.NewRequest("GET", "http://localhost/", nil)
	req.Header.Set("Accept", "text/html")
	resp := &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"https://myteam.cloudflareaccess.com/cdn-cgi/access/login"}},
		Body:       io.NopCloser(strings.NewReader("<html>login</html>")),
		Request:    req,
	}

	require.NoError(t, mr(resp))

	assert.Equal(t, http.StatusFound, resp.StatusCode, "a browser should still see the real redirect so it can log in")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "<html>login</html>", string(body))

	waitForToken(t, holder, "new-token")
}

func TestNewModifyResponseIgnoresUnrelatedResponses(t *testing.T) {
	holder := newTokenHolder("old-token")
	fetched := false
	fetch := func(ctx context.Context, address string) (string, error) {
		fetched = true
		return "new-token", nil
	}
	tm := newTokenManager("app.example.com:443", fetch, holder, slog.New(slog.DiscardHandler))
	mr := newModifyResponse(context.Background(), slog.New(slog.DiscardHandler), "app.example.com:443", tm)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    httptest.NewRequest("GET", "http://localhost/", nil),
	}

	require.NoError(t, mr(resp))

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))
	assert.False(t, fetched)
	assert.Equal(t, "old-token", holder.load())
}
