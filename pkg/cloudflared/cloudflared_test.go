package cloudflared

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCommander is a mock for the Commander interface.
type MockCommander struct {
	mock.Mock
}

// CombinedOutput is the mock implementation that allows us to fake the command execution.
func (m *MockCommander) CombinedOutput(name string, arg ...string) ([]byte, error) {
	// The arguments passed to `On` and `AssertCalled` must match the
	// arguments passed to the method.
	// Because `arg` is a variadic parameter, we need to handle it carefully.
	// We convert it to a slice of `any` to pass to `m.Called`.
	args := make([]any, len(arg)+1)
	args[0] = name
	for i, v := range arg {
		args[i+1] = v
	}
	ret := m.Called(args...)

	// Handle the return values.
	// The first return value can be nil if an error is returned.
	var r0 []byte
	if ret.Get(0) != nil {
		r0 = ret.Get(0).([]byte)
	}

	return r0, ret.Error(1)
}

func TestGetCloudflareAccessTokenForAppWithMock(t *testing.T) {
	// Save the original commander and restore it after the test.
	originalCmdr := cmdr
	t.Cleanup(func() {
		cmdr = originalCmdr
	})

	t.Run("success", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to be called and return success (nil error).
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "login", "app.example.com").Return([]byte(""), nil)
		// Expect the token command to be called and return a mock token.
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "token", "-app=app.example.com").Return([]byte("mock-token"), nil)

		token, err := GetCloudflareAccessTokenForApp("app.example.com")

		assert.NoError(t, err)
		assert.Equal(t, "mock-token", token)
		mockCmdr.AssertExpectations(t)
	})

	t.Run("cloudflared not installed", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to fail with exec.ErrNotFound.
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "login", "app.example.com/not-installed").Return(nil, exec.ErrNotFound)

		_, err := GetCloudflareAccessTokenForApp("app.example.com/not-installed")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cloudflared is not installed")
		mockCmdr.AssertExpectations(t)
	})

	t.Run("access app not found", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to fail with the specific error message.
		errOutput := []byte(accessAppNotFoundMsg)
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "login", "app.example.com/not-found").Return(errOutput, errors.New("exit status 1"))

		_, err := GetCloudflareAccessTokenForApp("app.example.com/not-found")

		assert.Error(t, err)
		assert.Equal(t, ErrAccessAppNotFound, err)
		mockCmdr.AssertExpectations(t)
	})

	t.Run("login fails", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		errOutput := []byte("some generic login error")
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "login", "app.example.com/login-fails").Return(errOutput, errors.New("exit status 1"))

		_, err := GetCloudflareAccessTokenForApp("app.example.com/login-fails")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cloudflared login failed: some generic login error")
		mockCmdr.AssertExpectations(t)
	})

	t.Run("token fails", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to succeed.
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "login", "app.example.com/token-fails").Return([]byte(""), nil)
		// Expect the token command to fail.
		errOutput := []byte("some generic token error")
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "token", "-app=app.example.com/token-fails").Return(errOutput, errors.New("exit status 1"))

		_, err := GetCloudflareAccessTokenForApp("app.example.com/token-fails")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cloudflared token failed: some generic token error")
		mockCmdr.AssertExpectations(t)
	})
}
