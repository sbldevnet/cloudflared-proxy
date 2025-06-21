package internal

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/sbldevnet/cloudflared-proxy/internal/config"
	"github.com/sbldevnet/cloudflared-proxy/pkg/cloudflared"
	"github.com/sbldevnet/cloudflared-proxy/pkg/proxy"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockProxyService struct {
	mock.Mock
}

func (m *MockProxyService) GetCloudflareAccessTokenForApp(url string) (string, error) {
	args := m.Called(url)
	return args.String(0), args.Error(1)
}

func (m *MockProxyService) StartMultipleProxies(ctx context.Context, configs []proxy.CFAccessProxyConfig) error {
	args := m.Called(ctx, configs)
	return args.Error(0)
}

func TestProxyCFAccess(t *testing.T) {
	var logOutput bytes.Buffer
	log.SetOutput(&logOutput)
	t.Cleanup(func() {
		log.SetOutput(nil)
	})

	testCases := []struct {
		name                 string
		configs              []config.ProxyConfig
		setupMocks           func(service *MockProxyService)
		expectedErr          error
		expectedLogContains  string
		unexpectedLogContain string
	}{
		{
			name: "Success",
			configs: []config.ProxyConfig{
				{Hostname: "app1.example.com", DestinationPort: 443, LocalPort: 8080},
			},
			setupMocks: func(service *MockProxyService) {
				service.On("GetCloudflareAccessTokenForApp", "app1.example.com:443").Return("token123", nil)
				service.On("StartMultipleProxies", mock.Anything, mock.AnythingOfType("[]proxy.CFAccessProxyConfig")).Return(nil).Run(func(args mock.Arguments) {
					configs := args.Get(1).([]proxy.CFAccessProxyConfig)
					assert.Len(t, configs, 1)
					assert.Equal(t, "app1.example.com:443", configs[0].Url.Host)
					assert.Equal(t, "token123", configs[0].Token)
				})
			},
		},
		{
			name: "Access app not found",
			configs: []config.ProxyConfig{
				{Hostname: "app1.example.com", DestinationPort: 443, LocalPort: 8080},
			},
			setupMocks: func(service *MockProxyService) {
				service.On("GetCloudflareAccessTokenForApp", "app1.example.com:443").Return("", cloudflared.ErrAccessAppNotFound)
				service.On("StartMultipleProxies", mock.Anything, mock.Anything).Return(nil)
			},
			expectedLogContains: "Access application not found at app1.example.com:443, continuing without authentication",
		},
		{
			name: "Error getting token",
			configs: []config.ProxyConfig{
				{Hostname: "app1.example.com", DestinationPort: 443, LocalPort: 8080},
			},
			setupMocks: func(service *MockProxyService) {
				service.On("GetCloudflareAccessTokenForApp", "app1.example.com:443").Return("", errors.New("some-cf-error"))
			},
			expectedErr: errors.New("some-cf-error"),
		},
		{
			name: "Error starting proxies",
			configs: []config.ProxyConfig{
				{Hostname: "app1.example.com", DestinationPort: 443, LocalPort: 8080},
			},
			setupMocks: func(service *MockProxyService) {
				service.On("GetCloudflareAccessTokenForApp", "app1.example.com:443").Return("token123", nil)
				service.On("StartMultipleProxies", mock.Anything, mock.Anything).Return(errors.New("proxy-start-error"))
			},
			expectedErr: errors.New("proxy-start-error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logOutput.Reset()
			mockService := new(MockProxyService)
			tc.setupMocks(mockService)

			err := ProxyCFAccess(context.Background(), tc.configs, mockService)

			if tc.expectedErr != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}

			if tc.expectedLogContains != "" {
				assert.Contains(t, logOutput.String(), tc.expectedLogContains)
			}

			mockService.AssertExpectations(t)
		})
	}
}
