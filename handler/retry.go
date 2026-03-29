package handler

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

// API-level retry for transient errors.
//
// V1 PHP equivalent: API.php wraps every request in a do/while loop that catches
// DBException and retries the entire API call up to API_RETRIES (5) times. This
// handles MySQL deadlocks, lost connections, Percona cluster conflicts, and any
// other transient error that bubbles up as a DBException.
//
// This package provides the same for Go:
//   - WithRetry(h): wraps a single handler with retry logic
//   - NewRetryGroup(r): wraps a fiber.Router so all registered handlers get retry
//   - Retryable(err): marks any error as retryable (for third-party failures etc.)
//
// On a retryable error, the response buffer is reset and the handler is called
// again. Because Fiber buffers the response in memory until the handler returns,
// the client never sees the failed attempt.
//
// Database-level retry for individual queries lives in database/retry.go (layer 1,
// equivalent to LoggedPDO::prex()). This is layer 2 (equivalent to API.php).

const (
	DefaultMaxRetries = 5
	minBackoffMs      = 50
	maxBackoffMs      = 200
)

// retryableError wraps an error to explicitly mark it as retryable.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// Retryable wraps an error to signal that the request should be retried.
// Use this in handlers when a third-party service returns a transient failure
// (e.g. 503, timeout) that is not automatically detected by pattern matching.
func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{err: err}
}

// isRetryable returns true if the error should trigger a request retry.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for explicit marker.
	var re *retryableError
	if errors.As(err, &re) {
		return true
	}

	// Don't retry intentional HTTP errors (4xx).
	var fe *fiber.Error
	if errors.As(err, &fe) && fe.Code < 500 {
		return false
	}

	// Check for known transient patterns — delegates to the database package
	// which mirrors v1 LoggedPDO::retryable() and the deadlock check.
	return database.IsRetryableDBError(err)
}

// WithRetry wraps a Fiber handler with API-level retry logic.
// On a retryable error the response is reset and the handler is called again,
// up to DefaultMaxRetries additional times with random backoff.
func WithRetry(h fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var lastErr error

		for attempt := 0; attempt <= DefaultMaxRetries; attempt++ {
			if attempt > 0 {
				// Backoff with jitter before retry.
				sleep := time.Duration(minBackoffMs+rand.Intn(maxBackoffMs-minBackoffMs)) * time.Millisecond
				time.Sleep(sleep)

				// Reset the response so the retried handler starts clean.
				c.Response().Reset()

				fmt.Printf("RETRY attempt %d/%d %s %s (previous error: %v)\n",
					attempt, DefaultMaxRetries, c.Method(), c.OriginalURL(), lastErr)
			}

			lastErr = h(c)

			if lastErr == nil {
				if attempt > 0 {
					fmt.Printf("RETRY succeeded on attempt %d %s %s\n",
						attempt+1, c.Method(), c.OriginalURL())
				}
				return nil
			}

			if !isRetryable(lastErr) {
				return lastErr
			}
		}

		// All retries exhausted.
		fmt.Printf("RETRY exhausted %d attempts %s %s: %v\n",
			DefaultMaxRetries+1, c.Method(), c.OriginalURL(), lastErr)
		return lastErr
	}
}

// RetryGroup wraps a fiber.Router so that every route handler registered on it
// is automatically wrapped with WithRetry. This is the equivalent of v1's
// API.php retry loop — applied once at route setup, protecting all endpoints.
//
// Usage in routes.go:
//
//	rg := handler.NewRetryGroup(app.Group("/api"))
//	rg.Get("/isochrone", isochrone.GetIsochrones)  // automatically retried
type RetryGroup struct {
	inner fiber.Router
}

// NewRetryGroup wraps a fiber.Router with automatic retry on all registered
// route handlers.
func NewRetryGroup(router fiber.Router) *RetryGroup {
	return &RetryGroup{inner: router}
}

func wrapAll(handlers []fiber.Handler) []fiber.Handler {
	wrapped := make([]fiber.Handler, len(handlers))
	for i, h := range handlers {
		wrapped[i] = WithRetry(h)
	}
	return wrapped
}

func (rg *RetryGroup) Get(path string, handlers ...fiber.Handler) fiber.Router {
	return rg.inner.Get(path, wrapAll(handlers)...)
}

func (rg *RetryGroup) Post(path string, handlers ...fiber.Handler) fiber.Router {
	return rg.inner.Post(path, wrapAll(handlers)...)
}

func (rg *RetryGroup) Put(path string, handlers ...fiber.Handler) fiber.Router {
	return rg.inner.Put(path, wrapAll(handlers)...)
}

func (rg *RetryGroup) Patch(path string, handlers ...fiber.Handler) fiber.Router {
	return rg.inner.Patch(path, wrapAll(handlers)...)
}

func (rg *RetryGroup) Delete(path string, handlers ...fiber.Handler) fiber.Router {
	return rg.inner.Delete(path, wrapAll(handlers)...)
}

// Group creates a sub-group that also has retry wrapping.
func (rg *RetryGroup) Group(prefix string, handlers ...fiber.Handler) *RetryGroup {
	return &RetryGroup{inner: rg.inner.Group(prefix, handlers...)}
}

// WithRetryN is like WithRetry but with a configurable retry count.
func WithRetryN(maxRetries int, h fiber.Handler) fiber.Handler {
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}
	return func(c *fiber.Ctx) error {
		var lastErr error

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				sleep := time.Duration(minBackoffMs+rand.Intn(maxBackoffMs-minBackoffMs)) * time.Millisecond
				time.Sleep(sleep)

				c.Response().Reset()

				fmt.Printf("RETRY attempt %d/%d %s %s (previous error: %v)\n",
					attempt, maxRetries, c.Method(), c.OriginalURL(), lastErr)
			}

			lastErr = h(c)

			if lastErr == nil {
				if attempt > 0 {
					fmt.Printf("RETRY succeeded on attempt %d %s %s\n",
						attempt+1, c.Method(), c.OriginalURL())
				}
				return nil
			}

			if !isRetryable(lastErr) {
				return lastErr
			}
		}

		fmt.Printf("RETRY exhausted %d attempts %s %s: %v\n",
			maxRetries+1, c.Method(), c.OriginalURL(), lastErr)
		return lastErr
	}
}
