package test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

// TestCreateIntent_RejectsUnauthenticated verifies 401 for unauthenticated requests.
func TestCreateIntent_RejectsUnauthenticated(t *testing.T) {
	body := []byte(`{"amount": 5.00}`)
	req := httptest.NewRequest("POST", "/apiv2/stripecreateintent", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

// TestCreateIntent_RejectsBelowMinimum verifies 400 when amount < 0.30.
func TestCreateIntent_RejectsBelowMinimum(t *testing.T) {
	prefix := uniquePrefix("intlow")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)
	_, jwt := CreateTestSession(t, userID)

	body := []byte(`{"amount": 0.10, "test": true}`)
	req := httptest.NewRequest("POST", "/apiv2/stripecreateintent?jwt="+jwt, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

// TestCreateIntent_RejectsAboveMaximum verifies 400 when amount > 250.
func TestCreateIntent_RejectsAboveMaximum(t *testing.T) {
	prefix := uniquePrefix("inthigh")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)
	_, jwt := CreateTestSession(t, userID)

	body := []byte(`{"amount": 300.00, "test": true}`)
	req := httptest.NewRequest("POST", "/apiv2/stripecreateintent?jwt="+jwt, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

// TestCreateIntent_AcceptsAmountAsString verifies that amount sent as a JSON string
// does NOT return 400. Vue v-model on <input type="number"> sends numbers as strings.
// Without STRIPE_SECRET_KEY_TEST the handler returns 500; we only verify it is not 400/401.
func TestCreateIntent_AcceptsAmountAsString(t *testing.T) {
	prefix := uniquePrefix("intstr")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)
	_, jwt := CreateTestSession(t, userID)

	body := []byte(`{"amount": "5.00", "test": true}`)
	req := httptest.NewRequest("POST", "/apiv2/stripecreateintent?jwt="+jwt, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.NotEqual(t, 400, resp.StatusCode, "string amount must not cause 400 parse error")
	assert.NotEqual(t, 401, resp.StatusCode)
}

// TestCreateIntent_AcceptsAmountAsNumber verifies that a plain numeric amount also works.
func TestCreateIntent_AcceptsAmountAsNumber(t *testing.T) {
	prefix := uniquePrefix("intnum")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)
	_, jwt := CreateTestSession(t, userID)

	body := []byte(`{"amount": 5.00, "test": true}`)
	req := httptest.NewRequest("POST", "/apiv2/stripecreateintent?jwt="+jwt, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.NotEqual(t, 400, resp.StatusCode, "numeric amount must not cause 400")
	assert.NotEqual(t, 401, resp.StatusCode)
}

// TestCreateSubscription_AcceptsAmountAsString verifies the subscription endpoint
// also accepts amount sent as a JSON string.
func TestCreateSubscription_AcceptsAmountAsString(t *testing.T) {
	prefix := uniquePrefix("substr")
	db := database.DBConn
	userID := CreateTestUser(t, prefix, "User")
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)
	_, jwt := CreateTestSession(t, userID)

	body := []byte(`{"amount": "3", "test": true}`)
	req := httptest.NewRequest("POST", "/apiv2/stripecreatesubscription?jwt="+jwt, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.NotEqual(t, 400, resp.StatusCode, "string amount must not cause 400 parse error")
	assert.NotEqual(t, 401, resp.StatusCode)
}
