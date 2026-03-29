package database

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- isConnectionError tests (layer 1 — query-level retry) ---

func TestIsConnectionError_Nil(t *testing.T) {
	assert.False(t, isConnectionError(nil))
}

func TestIsConnectionError_GoneAway(t *testing.T) {
	assert.True(t, isConnectionError(errors.New("MySQL server has gone away")))
}

func TestIsConnectionError_LostConnection(t *testing.T) {
	assert.True(t, isConnectionError(errors.New("Lost connection to MySQL server during query")))
}

func TestIsConnectionError_WSREP(t *testing.T) {
	assert.True(t, isConnectionError(errors.New("WSREP has not yet prepared node for application use")))
}

func TestIsConnectionError_CaseInsensitive(t *testing.T) {
	assert.True(t, isConnectionError(errors.New("mysql server HAS GONE AWAY")))
}

func TestIsConnectionError_DeadlockIsNot(t *testing.T) {
	// Deadlocks are NOT connection errors — they bubble to layer 2.
	assert.False(t, isConnectionError(errors.New("Deadlock found when trying to get lock")))
	assert.False(t, isConnectionError(errors.New("Error 1213: Deadlock")))
	assert.False(t, isConnectionError(errors.New("Lock wait timeout exceeded")))
}

func TestIsConnectionError_RegularError(t *testing.T) {
	assert.False(t, isConnectionError(errors.New("invalid syntax")))
	assert.False(t, isConnectionError(errors.New("table does not exist")))
	assert.False(t, isConnectionError(errors.New("duplicate key")))
}

// --- IsDeadlockOrLockTimeout tests ---

func TestIsDeadlockOrLockTimeout_Deadlock(t *testing.T) {
	assert.True(t, IsDeadlockOrLockTimeout(errors.New("Deadlock found when trying to get lock")))
}

func TestIsDeadlockOrLockTimeout_Error1213(t *testing.T) {
	assert.True(t, IsDeadlockOrLockTimeout(errors.New("Error 1213: Deadlock")))
}

func TestIsDeadlockOrLockTimeout_LockWaitTimeout(t *testing.T) {
	assert.True(t, IsDeadlockOrLockTimeout(errors.New("Lock wait timeout exceeded; try restarting transaction")))
}

func TestIsDeadlockOrLockTimeout_Nil(t *testing.T) {
	assert.False(t, IsDeadlockOrLockTimeout(nil))
}

func TestIsDeadlockOrLockTimeout_ConnectionErrorIsNot(t *testing.T) {
	assert.False(t, IsDeadlockOrLockTimeout(errors.New("MySQL server has gone away")))
	assert.False(t, IsDeadlockOrLockTimeout(errors.New("Lost connection")))
}

func TestIsDeadlockOrLockTimeout_NoFalsePositive1213(t *testing.T) {
	// "1213" alone should NOT match — needs "error 1213" prefix.
	assert.False(t, IsDeadlockOrLockTimeout(errors.New("table_1213 not found")))
	assert.False(t, IsDeadlockOrLockTimeout(errors.New("user 1213 does not exist")))
}

// --- IsRetryableDBError tests (combined, used by layer 2) ---

func TestIsRetryableDBError_ConnectionErrors(t *testing.T) {
	assert.True(t, IsRetryableDBError(errors.New("MySQL server has gone away")))
	assert.True(t, IsRetryableDBError(errors.New("Lost connection")))
	assert.True(t, IsRetryableDBError(errors.New("WSREP has not yet prepared")))
}

func TestIsRetryableDBError_Deadlocks(t *testing.T) {
	assert.True(t, IsRetryableDBError(errors.New("Deadlock found")))
	assert.True(t, IsRetryableDBError(errors.New("Error 1213: Deadlock")))
	assert.True(t, IsRetryableDBError(errors.New("Lock wait timeout exceeded")))
}

func TestIsRetryableDBError_NilAndRegular(t *testing.T) {
	assert.False(t, IsRetryableDBError(nil))
	assert.False(t, IsRetryableDBError(errors.New("syntax error")))
	assert.False(t, IsRetryableDBError(errors.New("duplicate key")))
}

// --- truncateSQL tests ---

func TestTruncateSQL_Short(t *testing.T) {
	assert.Equal(t, "SELECT 1", truncateSQL("SELECT 1"))
}

func TestTruncateSQL_Exact80(t *testing.T) {
	sql := "SELECT * FROM messages WHERE id = ? AND groupid = ? AND collection = ? AND type = ? ORDER BY"
	if len(sql) > 80 {
		sql = sql[:80]
	}
	assert.Equal(t, sql, truncateSQL(sql))
}

func TestTruncateSQL_Long(t *testing.T) {
	sql := "SELECT m.id, m.subject, m.textbody, m.type, m.arrival, m.fromuser, g.nameshort, g.namedisplay FROM messages m LEFT JOIN messages_groups mg ON mg.msgid = m.id LEFT JOIN groups g ON g.id = mg.groupid WHERE m.id = ?"
	result := truncateSQL(sql)
	assert.Equal(t, 83, len(result)) // 80 + "..."
	assert.True(t, len(result) <= 83)
}
