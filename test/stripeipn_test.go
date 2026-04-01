package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func makeChargeEvent(chargeID string, amountPence int64, metadata map[string]string, billingEmail string, description string) []byte {
	metaJSON, _ := json.Marshal(metadata)

	event := fmt.Sprintf(`{
		"id": "evt_test_%s",
		"type": "charge.succeeded",
		"data": {
			"object": {
				"id": "%s",
				"amount": %d,
				"currency": "gbp",
				"description": "%s",
				"metadata": %s,
				"payment_method_details": {"type": "card"},
				"billing_details": {"email": "%s", "name": "Test Donor"}
			}
		}
	}`, chargeID, chargeID, amountPence, description, string(metaJSON), billingEmail)

	return []byte(event)
}

func TestStripeIPN_ChargeSucceeded(t *testing.T) {
	prefix := uniquePrefix("stripein")
	db := database.DBConn

	// Create a test user with an email.
	userID := CreateTestUser(t, prefix+"_donor", "User")
	email := prefix + "_donor@test.com"

	// Send a charge.succeeded event with the user's ID in metadata.
	chargeID := "ch_test_" + prefix
	body := makeChargeEvent(chargeID, 1000, map[string]string{"uid": fmt.Sprint(userID)}, email, "One-time donation")

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify donation was recorded.
	var donationCount int64
	db.Raw("SELECT COUNT(*) FROM users_donations WHERE TransactionID = ?", chargeID).Scan(&donationCount)
	assert.Equal(t, int64(1), donationCount, "Donation should be recorded")

	// Verify amount.
	var amount float64
	db.Raw("SELECT GrossAmount FROM users_donations WHERE TransactionID = ?", chargeID).Scan(&amount)
	assert.Equal(t, 10.0, amount, "Amount should be £10.00")

	// Verify user was matched.
	var donorID *uint64
	db.Raw("SELECT userid FROM users_donations WHERE TransactionID = ?", chargeID).Scan(&donorID)
	assert.NotNil(t, donorID)
	assert.Equal(t, userID, *donorID)

	// Clean up.
	db.Exec("DELETE FROM users_donations WHERE TransactionID = ?", chargeID)
}

func TestStripeIPN_MatchByBillingEmail(t *testing.T) {
	prefix := uniquePrefix("stripemail")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_donor", "User")
	email := prefix + "_donor@test.com"

	// Send event without metadata UID — should match by billing email.
	chargeID := "ch_test_" + prefix
	body := makeChargeEvent(chargeID, 500, map[string]string{}, email, "")

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var donorID *uint64
	db.Raw("SELECT userid FROM users_donations WHERE TransactionID = ?", chargeID).Scan(&donorID)
	assert.NotNil(t, donorID)
	assert.Equal(t, userID, *donorID)

	db.Exec("DELETE FROM users_donations WHERE TransactionID = ?", chargeID)
}

func TestStripeIPN_UnmatchedUser(t *testing.T) {
	prefix := uniquePrefix("stripeunk")
	db := database.DBConn

	// Send event with unknown email and no metadata.
	chargeID := "ch_test_" + prefix
	body := makeChargeEvent(chargeID, 2000, map[string]string{}, "unknown@nowhere.com", "")

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Donation should still be recorded with null userid.
	var donationCount int64
	db.Raw("SELECT COUNT(*) FROM users_donations WHERE TransactionID = ?", chargeID).Scan(&donationCount)
	assert.Equal(t, int64(1), donationCount, "Donation should be recorded even without user match")

	var donorID *uint64
	db.Raw("SELECT userid FROM users_donations WHERE TransactionID = ?", chargeID).Scan(&donorID)
	assert.Nil(t, donorID, "userid should be NULL for unmatched donor")

	db.Exec("DELETE FROM users_donations WHERE TransactionID = ?", chargeID)
}

func TestStripeIPN_PayPalIgnored(t *testing.T) {
	db := database.DBConn

	// PayPal charges through Stripe should be ignored.
	body := []byte(`{
		"id": "evt_test_paypal",
		"type": "charge.succeeded",
		"data": {
			"object": {
				"id": "ch_test_paypal_ignore",
				"amount": 500,
				"currency": "gbp",
				"description": "",
				"metadata": {},
				"payment_method_details": {"type": "paypal"},
				"billing_details": {"email": "test@test.com", "name": "Test"}
			}
		}
	}`)

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_donations WHERE TransactionID = 'ch_test_paypal_ignore'").Scan(&count)
	assert.Equal(t, int64(0), count, "PayPal charge should not be recorded")
}

func TestStripeIPN_ZeroAmount(t *testing.T) {
	db := database.DBConn

	body := makeChargeEvent("ch_test_zero", 0, map[string]string{}, "test@test.com", "")

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var count int64
	db.Raw("SELECT COUNT(*) FROM users_donations WHERE TransactionID = 'ch_test_zero'").Scan(&count)
	assert.Equal(t, int64(0), count, "Zero amount should not be recorded")
}

func TestStripeIPN_UnknownEventType(t *testing.T) {
	body := []byte(`{"id": "evt_test_unknown", "type": "payment_intent.created", "data": {"object": {}}}`)

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestStripeIPN_InvalidPayload(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestStripeIPN_RecurringFirstDonation(t *testing.T) {
	prefix := uniquePrefix("striperecur")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_donor", "User")
	email := prefix + "_donor@test.com"

	// First recurring donation — should trigger thank-you.
	chargeID := "ch_test_" + prefix
	body := makeChargeEvent(chargeID, 500, map[string]string{"uid": fmt.Sprint(userID)}, email, "Subscription creation")

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify recorded as subscr_payment.
	var txType string
	db.Raw("SELECT TransactionType FROM users_donations WHERE TransactionID = ?", chargeID).Scan(&txType)
	assert.Equal(t, "subscr_payment", txType)

	// Verify thank-you task was queued.
	var taskCount int64
	db.Raw("SELECT COUNT(*) FROM background_tasks WHERE task_type = 'email_donate_external' AND JSON_EXTRACT(data, '$.user_id') = ? AND processed_at IS NULL",
		userID).Scan(&taskCount)
	assert.Equal(t, int64(1), taskCount, "Thank-you email should be queued for first recurring donation")

	db.Exec("DELETE FROM users_donations WHERE TransactionID = ?", chargeID)
	db.Exec("DELETE FROM background_tasks WHERE task_type = 'email_donate_external' AND data LIKE ?",
		fmt.Sprintf("%%\"user_id\":%d%%", userID))
}

func TestStripeIPN_GiftAidNotification(t *testing.T) {
	prefix := uniquePrefix("stripegift")
	db := database.DBConn

	userID := CreateTestUser(t, prefix+"_donor", "User")
	email := prefix + "_donor@test.com"

	chargeID := "ch_test_" + prefix
	body := makeChargeEvent(chargeID, 1000, map[string]string{"uid": fmt.Sprint(userID)}, email, "")

	req := httptest.NewRequest("POST", "/api/stripeipn", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify gift aid notification was created.
	var notifCount int64
	db.Raw("SELECT COUNT(*) FROM users_notifications WHERE touser = ? AND type = 'GiftAid'", userID).Scan(&notifCount)
	assert.Equal(t, int64(1), notifCount, "Gift aid notification should be created")

	db.Exec("DELETE FROM users_donations WHERE TransactionID = ?", chargeID)
	db.Exec("DELETE FROM users_notifications WHERE touser = ? AND type = 'GiftAid'", userID)
}
