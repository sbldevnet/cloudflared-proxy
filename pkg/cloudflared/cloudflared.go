package cloudflared

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"
)

// Commander executes a command and returns its combined output.
type Commander interface {
	CombinedOutput(name string, arg ...string) ([]byte, error)
}

// execCommander is the default implementation of Commander that executes real commands.
type execCommander struct{}

func (c *execCommander) CombinedOutput(name string, arg ...string) ([]byte, error) {
	cmd := exec.Command(name, arg...)
	return cmd.CombinedOutput()
}

// cmdr is the command runner used by the package. It can be replaced in tests.
var cmdr Commander = &execCommander{}

const (
	accessAppNotFoundMsg = "failed to find Access application"
	cloudflaredDocURL    = "https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation"
)

var ErrAccessAppNotFound = errors.New("access application not found")

func GetCloudflareAccessTokenForApp(url string) (string, error) {
	output, err := cmdr.CombinedOutput("cloudflared", "access", "login", url)
	logger.Debug("cloudflared.GetCloudflareAccessTokenForApp", "executing cloudflared access login command for %s", url)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			logger.Error("cloudflared.GetCloudflareAccessTokenForApp", err, "cloudflared is not installed")
			return "", fmt.Errorf("cloudflared is not installed. Please install it first: %s", cloudflaredDocURL)
		}

		// Check if the host does not have an Access application
		outputStr := string(output)
		if strings.Contains(outputStr, accessAppNotFoundMsg) {
			return "", ErrAccessAppNotFound
		}

		logger.Error("cloudflared.GetCloudflareAccessTokenForApp", err, "cloudflared login failed: %s", outputStr)
		return "", fmt.Errorf("cloudflared login failed: %s", outputStr)
	}

	output, err = cmdr.CombinedOutput("cloudflared", "access", "token", fmt.Sprintf("-app=%s", url))
	logger.Debug("cloudflared.GetCloudflareAccessTokenForApp", "executing cloudflared access token command for %s", url)
	if err != nil {
		logger.Error("cloudflared.GetCloudflareAccessTokenForApp", err, "cloudflared token failed: %s", string(output))
		return "", fmt.Errorf("cloudflared token failed: %s", string(output))
	}

	return string(output), nil
}
