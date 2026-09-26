package cloudflared

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Commander executes commands and returns their combined output, optionally
// forwarding stderr live.
type Commander interface {
	CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error)
	// StreamStderr behaves like CombinedOutput but also forwards the command's
	// stderr to w as it is produced. Stdout is captured but never forwarded.
	StreamStderr(ctx context.Context, w io.Writer, name string, arg ...string) ([]byte, error)
}

// lockedBuffer is a bytes buffer safe for the concurrent stdout and stderr copiers.
type lockedBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// execCommander is the default implementation of Commander that executes real commands.
type execCommander struct{}

func (c *execCommander) CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, arg...)
	return cmd.CombinedOutput()
}

func (c *execCommander) StreamStderr(ctx context.Context, w io.Writer, name string, arg ...string) ([]byte, error) {
	var captured lockedBuffer
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stdout = &captured
	cmd.Stderr = io.MultiWriter(&captured, w)
	err := cmd.Run()
	return captured.buf, err
}

// cmdr is the command runner used by the package. It can be replaced in tests.
var cmdr Commander = &execCommander{}

const (
	accessAppNotFoundMsg = "failed to find Access application"
	cloudflaredDocURL    = "https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation"
)

var ErrAccessAppNotFound = errors.New("access application not found")

// loginOutput receives the live output of `cloudflared access login`, which is
// where cloudflared prints the URL to open manually on headless machines.
var loginOutput io.Writer = os.Stderr

func CloudflareAccessTokenForApp(ctx context.Context, url string) (string, error) {
	// --quiet keeps the JWT off stdout; only stderr is forwarded to the user.
	output, err := cmdr.StreamStderr(ctx, loginOutput, "cloudflared", "access", "login", "--quiet", url)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("cloudflared is not installed. Please install it first: %s", cloudflaredDocURL)
		}

		// Check if the host does not have an Access application
		outputStr := string(output)
		if strings.Contains(outputStr, accessAppNotFoundMsg) {
			return "", ErrAccessAppNotFound
		}

		// The output was already streamed to the user, so it is not repeated here.
		return "", fmt.Errorf("cloudflared login failed: %w", err)
	}

	output, err = cmdr.CombinedOutput(ctx, "cloudflared", "access", "token", fmt.Sprintf("-app=%s", url))
	if err != nil {
		return "", fmt.Errorf("cloudflared token failed: %s", string(output))
	}

	return string(output), nil
}
