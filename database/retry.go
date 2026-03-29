package database

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Database-level retry for transient errors.
//
// V1 PHP equivalent: LoggedPDO::prex() retries individual SQL operations up to
// $this->tries times for connection errors (gone away, lost connection, WSREP).
// On deadlock OUTSIDE a transaction it retries the query; on deadlock INSIDE a
// transaction it throws DBException up to the API-level retry (API.php).
//
// This package provides the same two behaviours:
//   - RetryQuery / RetryExec: retry individual queries (layer 1, like prex())
//   - isRetryableDBError: pattern detection used by both layers
//
// The API-level retry middleware (handler.NewRetryMiddleware) provides layer 2.

const (
	// DBRetries matches v1's LoggedPDO::$tries default.
	DBRetries     = 10
	dbMinBackoff  = 100 // ms
	dbMaxBackoff  = 1000 // ms
)

// isConnectionError returns true for connection-level errors that should be
// retried at the query level (layer 1). These are errors where the DB
// connection is lost/broken — retrying the same query on a fresh connection
// will succeed. Mirrors v1 LoggedPDO::retryable().
//
// Deadlocks and lock wait timeouts are NOT retried here. In v1, prex() throws
// DBException on deadlock inside a transaction, letting the API-level retry
// (API.php) handle it by re-running the whole handler. We do the same: these
// bubble up to handler.WithRetry (layer 2).
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "has gone away") ||
		strings.Contains(msg, "lost connection") ||
		strings.Contains(msg, "wsrep has not yet prepared")
}

// IsDeadlockOrLockTimeout returns true for deadlock and lock-wait errors.
// These should NOT be retried at the query level (the transaction is already
// rolled back). They are retried at the API/handler level (layer 2).
func IsDeadlockOrLockTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "deadlock") ||
		strings.Contains(msg, "error 1213") ||
		strings.Contains(msg, "lock wait timeout")
}

// IsRetryableDBError returns true for any transient database error (connection
// errors + deadlocks + lock timeouts). Used by the API-level retry (layer 2)
// to decide whether to retry the entire handler.
func IsRetryableDBError(err error) bool {
	return isConnectionError(err) || IsDeadlockOrLockTimeout(err)
}

func dbBackoff() {
	sleep := time.Duration(dbMinBackoff+rand.Intn(dbMaxBackoff-dbMinBackoff)) * time.Millisecond
	time.Sleep(sleep)
}

// RetryQuery executes a SELECT-type query with retry on transient errors.
// Mirrors v1 LoggedPDO::preQuery() retry behaviour.
//
// Usage:
//
//	var results []MyType
//	err := database.RetryQuery(db, &results, "SELECT * FROM foo WHERE id = ?", id)
func RetryQuery(db *gorm.DB, dest interface{}, sql string, args ...interface{}) error {
	for attempt := 0; attempt < DBRetries; attempt++ {
		result := db.Raw(sql, args...).Scan(dest)
		if result.Error == nil {
			if attempt > 0 {
				fmt.Printf("DB RETRY query succeeded on attempt %d: %s\n", attempt+1, truncateSQL(sql))
			}
			return nil
		}

		// Only retry connection errors at this layer. Deadlocks and lock
		// timeouts bubble up to the handler-level retry (layer 2) which
		// re-runs the entire handler including fresh transactions.
		if !isConnectionError(result.Error) || attempt == DBRetries-1 {
			return result.Error
		}

		fmt.Printf("DB RETRY attempt %d/%d for %s: %v\n", attempt+1, DBRetries, truncateSQL(sql), result.Error)
		dbBackoff()
	}
	return fmt.Errorf("database: no retries configured")
}

// RetryExec executes a write query (INSERT/UPDATE/DELETE) with retry on
// connection errors. Mirrors v1 LoggedPDO::preExec() retry behaviour.
//
// Deadlocks are NOT retried here — they bubble to the handler-level retry
// (layer 2) which re-runs the whole handler with a fresh transaction.
//
// Usage:
//
//	err := database.RetryExec(db, "UPDATE foo SET bar = ? WHERE id = ?", val, id)
func RetryExec(db *gorm.DB, sql string, args ...interface{}) error {
	for attempt := 0; attempt < DBRetries; attempt++ {
		result := db.Exec(sql, args...)
		if result.Error == nil {
			if attempt > 0 {
				fmt.Printf("DB RETRY exec succeeded on attempt %d: %s\n", attempt+1, truncateSQL(sql))
			}
			return nil
		}

		if !isConnectionError(result.Error) || attempt == DBRetries-1 {
			return result.Error
		}

		fmt.Printf("DB RETRY attempt %d/%d for %s: %v\n", attempt+1, DBRetries, truncateSQL(sql), result.Error)
		dbBackoff()
	}
	return fmt.Errorf("database: no retries configured")
}

// RetryExecResult is like RetryExec but also returns the RowsAffected count.
func RetryExecResult(db *gorm.DB, sql string, args ...interface{}) (int64, error) {
	for attempt := 0; attempt < DBRetries; attempt++ {
		result := db.Exec(sql, args...)
		if result.Error == nil {
			if attempt > 0 {
				fmt.Printf("DB RETRY exec succeeded on attempt %d: %s\n", attempt+1, truncateSQL(sql))
			}
			return result.RowsAffected, nil
		}

		if !isConnectionError(result.Error) || attempt == DBRetries-1 {
			return 0, result.Error
		}

		fmt.Printf("DB RETRY attempt %d/%d for %s: %v\n", attempt+1, DBRetries, truncateSQL(sql), result.Error)
		dbBackoff()
	}
	return 0, fmt.Errorf("database: no retries configured")
}

func truncateSQL(sql string) string {
	if len(sql) > 80 {
		return sql[:80] + "..."
	}
	return sql
}
