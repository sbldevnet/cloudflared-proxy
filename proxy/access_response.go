package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// accessSessionExpiredMsg is returned to non-browser clients instead of
// Cloudflare's login redirect while a new login is pending.
const accessSessionExpiredMsg = "cloudflared-proxy: the Cloudflare Access session for this target expired; a login is pending in the proxy's terminal\n"

// isAccessLoginRedirect reports whether resp is the edge signalling an
// invalid or missing Access token: a 302, 401 or 403 whose Location points
// under cloudflareaccess.com.
func isAccessLoginRedirect(resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusFound, http.StatusUnauthorized, http.StatusForbidden:
	default:
		return false
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		return false
	}
	u, err := url.Parse(loc)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "cloudflareaccess.com" || strings.HasSuffix(host, ".cloudflareaccess.com")
}

// acceptsHTML reports whether req looks like it came from a browser, based on
// its Accept header. This is content negotiation, not a security boundary:
// the client already gets the same Access rejection either way, and the
// header is client-controlled, so it must not be relied on to withhold the
// redirect or target details from a non-browser client.
func acceptsHTML(req *http.Request) bool {
	if req == nil {
		return false
	}
	for _, v := range req.Header.Values("Accept") {
		if strings.Contains(v, "text/html") {
			return true
		}
	}
	return false
}

// rewriteAsAccessExpiredError replaces resp with a short, clear error so a
// non-browser client (git, curl) does not receive an HTML login redirect it
// cannot follow.
func rewriteAsAccessExpiredError(resp *http.Response) {
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	resp.StatusCode = http.StatusBadGateway
	resp.Status = strconv.Itoa(resp.StatusCode) + " " + http.StatusText(resp.StatusCode)
	resp.Body = io.NopCloser(strings.NewReader(accessSessionExpiredMsg))
	resp.ContentLength = int64(len(accessSessionExpiredMsg))
	resp.Header = http.Header{
		"Content-Type":   []string{"text/plain; charset=utf-8"},
		"Content-Length": []string{strconv.Itoa(len(accessSessionExpiredMsg))},
	}
}

// newModifyResponse returns a ReverseProxy.ModifyResponse hook that triggers
// a token renewal when the edge rejects a request, and, for non-browser
// clients, replaces the HTML login redirect with a clear error instead of
// forwarding it.
func newModifyResponse(ctx context.Context, log *slog.Logger, address string, tm *tokenManager) func(*http.Response) error {
	return func(resp *http.Response) error {
		if !isAccessLoginRedirect(resp) {
			return nil
		}

		log.Warn("Cloudflare Access rejected the request; renewing the token", "address", address, "status", resp.StatusCode)
		go tm.triggerRenewal(ctx)

		if acceptsHTML(resp.Request) {
			return nil
		}

		rewriteAsAccessExpiredError(resp)
		return nil
	}
}
