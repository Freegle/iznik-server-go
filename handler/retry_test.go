package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

// helper: create a Fiber app with a retry-wrapped test handler.
func testApp(handler fiber.Handler, maxRetries ...int) *fiber.App {
	app := fiber.New()

	n := DefaultMaxRetries
	if len(maxRetries) > 0 {
		n = maxRetries[0]
	}
	app.Get("/test", WithRetryN(n, handler))
	app.Post("/test", WithRetryN(n, handler))

	return app
}

func body(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// --- isRetryable tests ---

func TestIsRetryable_Nil(t *testing.T) {
	assert.False(t, isRetryable(nil))
}

func TestIsRetryable_ExplicitMarker(t *testing.T) {
	err := Retryable(errors.New("some third-party timeout"))
	assert.True(t, isRetryable(err))
}

func TestIsRetryable_ExplicitMarkerNil(t *testing.T) {
	assert.Nil(t, Retryable(nil))
}

func TestIsRetryable_DeadlockMessage(t *testing.T) {
	assert.True(t, isRetryable(errors.New("Deadlock found when trying to get lock")))
}

func TestIsRetryable_Error1213(t *testing.T) {
	assert.True(t, isRetryable(errors.New("Error 1213: Deadlock")))
}

func TestIsRetryable_HasGoneAway(t *testing.T) {
	assert.True(t, isRetryable(errors.New("MySQL server has gone away")))
}

func TestIsRetryable_LostConnection(t *testing.T) {
	assert.True(t, isRetryable(errors.New("Lost connection to MySQL server during query")))
}

func TestIsRetryable_LockWaitTimeout(t *testing.T) {
	assert.True(t, isRetryable(errors.New("Lock wait timeout exceeded")))
}

func TestIsRetryable_WSREP(t *testing.T) {
	assert.True(t, isRetryable(errors.New("WSREP has not yet prepared node for application use")))
}

func TestIsRetryable_CaseInsensitive(t *testing.T) {
	assert.True(t, isRetryable(errors.New("DEADLOCK FOUND WHEN TRYING TO GET LOCK")))
}

func TestIsRetryable_RegularError(t *testing.T) {
	assert.False(t, isRetryable(errors.New("invalid input")))
}

func TestIsRetryable_FiberError4xx(t *testing.T) {
	// 4xx errors are intentional — never retry.
	assert.False(t, isRetryable(fiber.NewError(400, "bad request")))
	assert.False(t, isRetryable(fiber.NewError(401, "unauthorized")))
	assert.False(t, isRetryable(fiber.NewError(403, "forbidden")))
	assert.False(t, isRetryable(fiber.NewError(404, "not found")))
}

func TestIsRetryable_FiberError5xxWithPattern(t *testing.T) {
	// 5xx with a retryable pattern should retry.
	assert.True(t, isRetryable(fiber.NewError(500, "Deadlock found")))
}

func TestIsRetryable_FiberError5xxWithoutPattern(t *testing.T) {
	// 5xx without a retryable pattern should NOT retry.
	assert.False(t, isRetryable(fiber.NewError(500, "null pointer")))
}

func TestIsRetryable_WrappedRetryable(t *testing.T) {
	// errors.As should unwrap through fmt.Errorf %w.
	inner := Retryable(errors.New("connection reset"))
	wrapped := fmt.Errorf("handler failed: %w", inner)
	assert.True(t, isRetryable(wrapped))
}

// --- Retryable() wrapper tests ---

func TestRetryable_Unwrap(t *testing.T) {
	inner := errors.New("original")
	wrapped := Retryable(inner)
	assert.True(t, errors.Is(wrapped, inner))
}

func TestRetryable_ErrorMessage(t *testing.T) {
	inner := errors.New("service unavailable")
	wrapped := Retryable(inner)
	assert.Equal(t, "service unavailable", wrapped.Error())
}

// --- WithRetry / WithRetryN integration tests ---

func TestRetry_SuccessNoRetry(t *testing.T) {
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "ok", body(resp))
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestRetry_NonRetryableError(t *testing.T) {
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return fiber.NewError(400, "bad request")
	})

	resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil))
	assert.Equal(t, 400, resp.StatusCode)
	// Should NOT retry a 400.
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestRetry_DeadlockThenSuccess(t *testing.T) {
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			return errors.New("Deadlock found when trying to get lock")
		}
		return c.SendString("recovered")
	}, 5)

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil), 10000)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "recovered", body(resp))
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestRetry_ExplicitRetryableThenSuccess(t *testing.T) {
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return Retryable(errors.New("third-party 503"))
		}
		return c.SendString("ok")
	}, 3)

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil), 10000)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestRetry_AllRetriesExhausted(t *testing.T) {
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("MySQL server has gone away")
	}, 3)

	resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil), 10000)
	// After 1 initial + 3 retries = 4 calls, should fail.
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, int32(4), atomic.LoadInt32(&calls))
}

func TestRetry_ResponseResetBetweenAttempts(t *testing.T) {
	// Verify the response body from a failed attempt doesn't leak into the
	// successful attempt.
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			c.SendString("LEAKED")
			return errors.New("Deadlock found")
		}
		return c.SendString("CLEAN")
	}, 3)

	resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil), 10000)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "CLEAN", body(resp))
}

func TestRetry_StatusCodeResetBetweenAttempts(t *testing.T) {
	// Verify the status code from a failed attempt doesn't leak.
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			c.Status(503)
			return Retryable(errors.New("service down"))
		}
		return c.Status(200).SendString("up")
	}, 3)

	resp, _ := app.Test(httptest.NewRequest("GET", "/test", nil), 10000)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "up", body(resp))
}

func TestRetry_DefaultMaxRetries(t *testing.T) {
	// WithRetry uses DefaultMaxRetries (5).
	var calls int32
	app := fiber.New()
	app.Get("/test", WithRetry(func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("Deadlock found")
	}))

	app.Test(httptest.NewRequest("GET", "/test", nil), 30000)
	// 1 initial + 5 retries = 6 calls.
	assert.Equal(t, int32(6), atomic.LoadInt32(&calls))
}

func TestRetry_ZeroConfigUsesDefault(t *testing.T) {
	// WithRetryN(0, h) should use DefaultMaxRetries.
	var calls int32
	app := fiber.New()
	app.Get("/test", WithRetryN(0, func(c *fiber.Ctx) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("Lock wait timeout exceeded")
	}))

	app.Test(httptest.NewRequest("GET", "/test", nil), 30000)
	assert.Equal(t, int32(DefaultMaxRetries+1), atomic.LoadInt32(&calls))
}

func TestRetry_POSTMethodRetries(t *testing.T) {
	// POST requests should also be retried — matching v1 behaviour where
	// both reads and writes get retry protection.
	var calls int32
	app := testApp(func(c *fiber.Ctx) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return errors.New("Lost connection to MySQL server")
		}
		return c.SendString("ok")
	}, 3)

	resp, _ := app.Test(httptest.NewRequest("POST", "/test", nil), 10000)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

// --- RetryGroup tests ---

func TestRetryGroup_Get(t *testing.T) {
	var calls int32
	app := fiber.New()
	rg := NewRetryGroup(app.Group("/api"))
	rg.Get("/test", func(c *fiber.Ctx) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return errors.New("Deadlock found")
		}
		return c.SendString("ok")
	})

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/test", nil), 10000)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "ok", body(resp))
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestRetryGroup_SubGroup(t *testing.T) {
	var calls int32
	app := fiber.New()
	rg := NewRetryGroup(app.Group("/api"))
	sub := rg.Group("/v2")
	sub.Get("/test", func(c *fiber.Ctx) error {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			return errors.New("MySQL server has gone away")
		}
		return c.SendString("ok")
	})

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/v2/test", nil), 10000)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}
