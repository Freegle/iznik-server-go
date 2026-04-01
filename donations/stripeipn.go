package donations

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/gofiber/fiber/v2"
	stripe "github.com/stripe/stripe-go/v82"
	stripecustomer "github.com/stripe/stripe-go/v82/customer"
)

// MANUAL_THANKS is the minimum one-off donation amount (GBP) that triggers a thank-you request.
const MANUAL_THANKS = 10.0

// StripeIPN handles Stripe webhook notifications (charge.succeeded).
// This is the Go equivalent of iznik-server/http/stripeipn.php.
//
// Stripe sends a POST with a JSON event body. We parse the event, record the
// donation, handle gift aid notifications, and queue thank-you emails.
//
// @Summary Handle Stripe webhook
// @Tags donations
// @Accept json
// @Produce json
// @Router /stripeipn [post]
func StripeIPN(c *fiber.Ctx) error {
	body := c.Body()
	log.Printf("[StripeIPN] Received webhook, body length %d", len(body))

	// Parse the event JSON.
	var event stripe.Event
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("[StripeIPN] Invalid payload: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid payload"})
	}

	log.Printf("[StripeIPN] Event type: %s, ID: %s", event.Type, event.ID)

	switch event.Type {
	case "charge.succeeded":
		return handleChargeSucceeded(c, &event)
	default:
		log.Printf("[StripeIPN] Ignoring event type: %s", event.Type)
		return c.SendStatus(fiber.StatusOK)
	}
}

// handleChargeSucceeded processes a successful Stripe charge.
func handleChargeSucceeded(c *fiber.Ctx, event *stripe.Event) error {
	gdb := database.DBConn

	// Parse the charge object from the event data.
	var charge stripe.Charge
	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		log.Printf("[StripeIPN] Failed to parse charge data: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse charge"})
	}

	// Amount is in pence — convert to pounds.
	amount := float64(charge.Amount) / 100.0

	// Exclude PayPal charges (we get a separate IPN for those).
	var paymentMethod string
	if charge.PaymentMethodDetails != nil {
		paymentMethod = string(charge.PaymentMethodDetails.Type)
	}

	log.Printf("[StripeIPN] Charge succeeded: £%.2f, method=%s, charge_id=%s",
		amount, paymentMethod, charge.ID)

	if amount == 0 {
		log.Printf("[StripeIPN] Zero amount, ignoring")
		return c.SendStatus(fiber.StatusOK)
	}

	if paymentMethod == "paypal" {
		log.Printf("[StripeIPN] PayPal payment, ignoring (handled by PayPal IPN)")
		return c.SendStatus(fiber.StatusOK)
	}

	// Try to identify the user.
	userID, userName, userEmail := matchDonorUser(&charge)

	// Determine if this is a recurring payment.
	recurring := charge.Description == "Subscription creation"

	// Check if this is the user's first recurring donation.
	firstRecurring := false
	if userID > 0 && recurring {
		var previousCount int64
		gdb.Raw("SELECT COUNT(*) FROM users_donations WHERE userid = ? AND TransactionType IN ('subscr_payment', 'recurring_payment')", userID).Scan(&previousCount)
		firstRecurring = previousCount == 0
		log.Printf("[StripeIPN] User %d previous recurring donations: %d, first=%v", userID, previousCount, firstRecurring)
	}

	// Record the donation.
	var transactionType *string
	if recurring {
		tt := "subscr_payment"
		transactionType = &tt
	}

	var userIDPtr *uint64
	if userID > 0 {
		userIDPtr = &userID
	}

	result := gdb.Exec(
		"INSERT INTO users_donations (userid, Payer, PayerDisplayName, timestamp, TransactionID, GrossAmount, source, TransactionType, type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		userIDPtr, userEmail, userName, time.Now().Format("2006-01-02 15:04:05"),
		charge.ID, amount, TYPE_STRIPE, transactionType, TYPE_STRIPE,
	)

	if result.Error != nil {
		log.Printf("[StripeIPN] Failed to record donation: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to record donation"})
	}

	var donationID uint64
	gdb.Raw("SELECT id FROM users_donations WHERE TransactionID = ? ORDER BY id DESC LIMIT 1", charge.ID).Scan(&donationID)
	log.Printf("[StripeIPN] Recorded donation id=%d for user=%d amount=£%.2f", donationID, userID, amount)

	// Handle gift aid notification.
	if userID > 0 {
		handleGiftAidNotification(userID)
	}

	// Queue thank-you email for significant donations.
	if userID > 0 && ((recurring && firstRecurring) || (!recurring && amount >= MANUAL_THANKS)) {
		log.Printf("[StripeIPN] Queuing thank-you for user %d, amount £%.2f, recurring=%v", userID, amount, recurring)

		if err := queue.QueueTask(queue.TaskEmailDonateExternal, map[string]interface{}{
			"user_name":  userName,
			"user_id":    userID,
			"user_email": userEmail,
			"amount":     amount,
		}); err != nil {
			log.Printf("[StripeIPN] Failed to queue thank-you email: %v", err)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

// matchDonorUser tries to identify the Freegle user who made the donation.
// Returns (userID, userName, userEmail). userID may be 0 if not matched.
func matchDonorUser(charge *stripe.Charge) (uint64, string, string) {
	gdb := database.DBConn
	var userID uint64
	var userName, userEmail string

	// 1. Try metadata UID from the charge.
	if charge.Metadata != nil {
		if uidStr, ok := charge.Metadata["uid"]; ok && uidStr != "" {
			uid, err := strconv.ParseUint(uidStr, 10, 64)
			if err == nil && uid > 0 {
				var exists uint64
				gdb.Raw("SELECT id FROM users WHERE id = ?", uid).Scan(&exists)
				if exists > 0 {
					userID = uid
					log.Printf("[StripeIPN] Matched user %d from charge metadata", userID)
				}
			}
		}
	}

	// 2. Try the Stripe customer if no metadata match.
	if userID == 0 && charge.Customer != nil && charge.Customer.ID != "" {
		log.Printf("[StripeIPN] Looking up customer %s", charge.Customer.ID)

		key := getStripeKey(false)
		if key != "" {
			stripeMu.Lock()
			stripe.Key = key
			cust, err := stripecustomer.Get(charge.Customer.ID, nil)
			stripeMu.Unlock()

			if err == nil && cust != nil {
				// Try customer metadata UID.
				if uidStr, ok := cust.Metadata["uid"]; ok && uidStr != "" {
					uid, err := strconv.ParseUint(uidStr, 10, 64)
					if err == nil && uid > 0 {
						var exists uint64
						gdb.Raw("SELECT id FROM users WHERE id = ?", uid).Scan(&exists)
						if exists > 0 {
							userID = uid
							log.Printf("[StripeIPN] Matched user %d from customer metadata", userID)
						}
					}
				}

				// Try customer email.
				if userID == 0 && cust.Email != "" {
					gdb.Raw("SELECT userid FROM users_emails WHERE email = ? AND userid IS NOT NULL LIMIT 1", cust.Email).Scan(&userID)
					if userID > 0 {
						log.Printf("[StripeIPN] Matched user %d from customer email %s", userID, cust.Email)
					}
				}
			}
		}
	}

	// 3. Try billing_details.email from the charge.
	if userID == 0 && charge.BillingDetails != nil && charge.BillingDetails.Email != "" {
		billingEmail := charge.BillingDetails.Email
		gdb.Raw("SELECT userid FROM users_emails WHERE email = ? AND userid IS NOT NULL LIMIT 1", billingEmail).Scan(&userID)
		if userID > 0 {
			log.Printf("[StripeIPN] Matched user %d from billing email %s", userID, billingEmail)
		}
	}

	// Get user name and email for the matched user.
	if userID > 0 {
		gdb.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", userID).Scan(&userEmail)
		gdb.Raw("SELECT fullname FROM users WHERE id = ?", userID).Scan(&userName)
		log.Printf("[StripeIPN] User %d: name=%s email=%s", userID, userName, userEmail)
	} else {
		// Use billing details as fallback.
		if charge.BillingDetails != nil {
			userEmail = charge.BillingDetails.Email
			userName = charge.BillingDetails.Name
		}
		log.Printf("[StripeIPN] No user matched, using billing details: name=%s email=%s", userName, userEmail)
	}

	return userID, userName, userEmail
}

// handleGiftAidNotification checks if the user needs a gift aid notification.
func handleGiftAidNotification(userID uint64) {
	gdb := database.DBConn

	type GiftAidRecord struct {
		Period string
	}

	var giftaid GiftAidRecord
	gdb.Raw("SELECT period FROM giftaid WHERE userid = ? ORDER BY id DESC LIMIT 1", userID).Scan(&giftaid)

	if giftaid.Period == "" || giftaid.Period == PERIOD_THIS {
		// No gift aid declaration or only a temporary one — prompt them.
		gdb.Exec("INSERT IGNORE INTO users_notifications (fromuser, touser, type, timestamp) VALUES (NULL, ?, 'GiftAid', NOW())", userID)
		log.Printf("[StripeIPN] Created gift aid notification for user %d (period=%s)", userID, giftaid.Period)
	}
}
