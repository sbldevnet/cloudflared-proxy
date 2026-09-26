package cloudflared

import (
	"bytes"
	"context"
	"errors"
	"io"
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
func (m *MockCommander) CombinedOutput(_ context.Context, name string, arg ...string) ([]byte, error) {
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

// StreamStderr is the mock implementation of the streaming call.
func (m *MockCommander) StreamStderr(_ context.Context, w io.Writer, name string, arg ...string) ([]byte, error) {
	args := make([]any, len(arg)+1)
	args[0] = name
	for i, v := range arg {
		args[i+1] = v
	}
	ret := m.Called(append([]any{w}, args...)...)
	var r0 []byte
	if ret.Get(0) != nil {
		r0 = ret.Get(0).([]byte)
	}
	return r0, ret.Error(1)
}

func TestCloudflareAccessTokenForAppWithMock(t *testing.T) {
	// Save the original commander and restore it after the test.
	originalCmdr := cmdr
	t.Cleanup(func() {
		cmdr = originalCmdr
	})

	t.Run("success", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to be called and return success (nil error).
		mockCmdr.On("StreamStderr", mock.Anything, "cloudflared", "access", "login", "--quiet", "app.example.com").Return([]byte(""), nil)
		// Expect the token command to be called and return a mock token.
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "token", "-app=app.example.com").Return([]byte("mock-token"), nil)

		token, err := CloudflareAccessTokenForApp(context.Background(), "app.example.com")

		assert.NoError(t, err)
		assert.Equal(t, "mock-token", token)
		mockCmdr.AssertExpectations(t)
	})

	t.Run("cloudflared not installed", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to fail with exec.ErrNotFound.
		mockCmdr.On("StreamStderr", mock.Anything, "cloudflared", "access", "login", "--quiet", "app.example.com/not-installed").Return(nil, exec.ErrNotFound)

		_, err := CloudflareAccessTokenForApp(context.Background(), "app.example.com/not-installed")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cloudflared is not installed")
		mockCmdr.AssertExpectations(t)
	})

	t.Run("access app not found", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to fail with the specific error message.
		errOutput := []byte(accessAppNotFoundMsg)
		mockCmdr.On("StreamStderr", mock.Anything, "cloudflared", "access", "login", "--quiet", "app.example.com/not-found").Return(errOutput, errors.New("exit status 1"))

		_, err := CloudflareAccessTokenForApp(context.Background(), "app.example.com/not-found")

		assert.Error(t, err)
		assert.Equal(t, ErrAccessAppNotFound, err)
		mockCmdr.AssertExpectations(t)
	})

	t.Run("login fails", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		errOutput := []byte("some generic login error")
		mockCmdr.On("StreamStderr", mock.Anything, "cloudflared", "access", "login", "--quiet", "app.example.com/login-fails").Return(errOutput, errors.New("exit status 1"))

		_, err := CloudflareAccessTokenForApp(context.Background(), "app.example.com/login-fails")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cloudflared login failed: some generic login error")
		mockCmdr.AssertExpectations(t)
	})

	t.Run("token fails", func(t *testing.T) {
		mockCmdr := new(MockCommander)
		cmdr = mockCmdr

		// Expect the login command to succeed.
		mockCmdr.On("StreamStderr", mock.Anything, "cloudflared", "access", "login", "--quiet", "app.example.com/token-fails").Return([]byte(""), nil)
		// Expect the token command to fail.
		errOutput := []byte("some generic token error")
		mockCmdr.On("CombinedOutput", "cloudflared", "access", "token", "-app=app.example.com/token-fails").Return(errOutput, errors.New("exit status 1"))

		_, err := CloudflareAccessTokenForApp(context.Background(), "app.example.com/token-fails")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cloudflared token failed: some generic token error")
		mockCmdr.AssertExpectations(t)
	})
}

// streamingCommander is a fake that writes to the forwarded stream and records
// what was written to it before the command returned.
type streamingCommander struct {
	loginStderr string
	loginStdout string
	loginErr    error
	tokenOut    string
}

func (s *streamingCommander) CombinedOutput(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(s.tokenOut), nil
}

func (s *streamingCommander) StreamStderr(_ context.Context, w io.Writer, _ string, _ ...string) ([]byte, error) {
	_, _ = io.WriteString(w, s.loginStderr)
	return []byte(s.loginStdout + s.loginStderr), s.loginErr
}

func TestLoginOutputIsForwarded(t *testing.T) {
	originalCmdr, originalOut := cmdr, loginOutput
	t.Cleanup(func() { cmdr, loginOutput = originalCmdr, originalOut })

	t.Run("stderr is forwarded, token output is not", func(t *testing.T) {
		var out bytes.Buffer
		loginOutput = &out
		cmdr = &streamingCommander{loginStderr: "open https://login.example/abc\n", tokenOut: "secret-token"}

		token, err := CloudflareAccessTokenForApp(context.Background(), "app.example.com")

		assert.NoError(t, err)
		assert.Equal(t, "secret-token", token)
		assert.Equal(t, "open https://login.example/abc\n", out.String())
		assert.NotContains(t, out.String(), "secret-token")
	})

	t.Run("output is still captured for error detection", func(t *testing.T) {
		var out bytes.Buffer
		loginOutput = &out
		cmdr = &streamingCommander{loginStderr: accessAppNotFoundMsg, loginErr: errors.New("exit status 1")}

		_, err := CloudflareAccessTokenForApp(context.Background(), "app.example.com")

		assert.Equal(t, ErrAccessAppNotFound, err)
		assert.Equal(t, accessAppNotFoundMsg, out.String())
	})
}

func TestExecCommanderStreamStderr(t *testing.T) {
	var fwd bytes.Buffer
	c := &execCommander{}

	got, err := c.StreamStderr(context.Background(), &fwd, "sh", "-c", "echo out; echo err >&2")

	assert.NoError(t, err)
	assert.Equal(t, "err\n", fwd.String())
	assert.Contains(t, string(got), "out\n")
	assert.Contains(t, string(got), "err\n")
}

func TestExecCommanderStreamStderrCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&execCommander{}).StreamStderr(ctx, io.Discard, "sleep", "30")

	assert.Error(t, err)
}
