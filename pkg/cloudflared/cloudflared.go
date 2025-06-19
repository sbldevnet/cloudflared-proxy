package cloudflared

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sbldevnet/cloudflared-proxy/pkg/logger"
)

const (
	accessAppNotFoundMsg = "failed to find Access application"
	cloudflaredDocURL    = "https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation"
)

var ErrAccessAppNotFound = errors.New("access application not found")

func GetCloudflareAccessTokenForApp(url string) (string, error) {
	cmd := exec.Command("cloudflared", "access", "login", url)
	output, err := cmd.CombinedOutput()
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

	cmd = exec.Command("cloudflared", "access", "token", fmt.Sprintf("-app=%s", url))
	output, err = cmd.CombinedOutput()
	logger.Debug("cloudflared.GetCloudflareAccessTokenForApp", "executing cloudflared access token command for %s", url)
	if err != nil {
		logger.Error("cloudflared.GetCloudflareAccessTokenForApp", err, "cloudflared token failed: %s", string(output))
		return "", fmt.Errorf("cloudflared token failed: %s", string(output))
	}

	return string(output), nil
}
