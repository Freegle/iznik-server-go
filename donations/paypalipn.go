package donations

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/gofiber/fiber/v2"
	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/paymentintent"
)

// PayPalIPN handles PayPal Instant Payment Notification callbacks.
// This is the Go equivalent of iznik-server/http/donateipn.php.
//
// PayPal sends a POST with form-encoded data when a donation is received.
// We record the donation, handle gift aid notifications, and queue thank-you emails.
//
// @Summary Handle PayPal IPN
// @Tags donations
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Router /donateipn [post]
func PayPalIPN(c *fiber.Ctx) error {
	mcGross := c.FormValue("mc_gross")
	payerEmail := c.FormValue("payer_email")
	firstName := c.FormValue("first_name")
	lastName := c.FormValue("last_name")
	txnID := c.FormValue("txn_id")
	txnType := c.FormValue("txn_type")
	paymentDate := c.FormValue("payment_date")
	custom := c.FormValue("custom")

	log.Printf("[PayPalIPN] Received IPN: txn_id=%s, txn_type=%s, mc_gross=%s, payer_email=%s",
		txnID, txnType, mcGross, payerEmail)

	if mcGross == "" {
		log.Printf("[PayPalIPN] No mc_gross, ignoring")
		return c.SendStatus(fiber.StatusOK)
	}

	gdb := database.DBConn

	// Try to identify the user.
	var userID uint64
	displayName := firstName + " " + lastName

	// Check if this is a PayPal-through-Stripe payment. The custom field contains the
	// Stripe PaymentIntent ID in format: acct_xxx:pi_xxx:hash.
	if custom != "" {
		re := regexp.MustCompile(`pi_[A-Za-z0-9_]+`)
		if match := re.FindString(custom); match != "" {
			log.Printf("[PayPalIPN] Found Stripe PaymentIntent ID: %s", match)

			key := getStripeKey(false)
			if key != "" {
				stripeMu.Lock()
				stripe.Key = key
				pi, err := paymentintent.Get(match, nil)
				stripeMu.Unlock()

				if err == nil && pi != nil && pi.Metadata != nil {
					if uidStr, ok := pi.Metadata["uid"]; ok && uidStr != "" {
						var uid uint64
						gdb.Raw("SELECT id FROM users WHERE id = ?", uidStr).Scan(&uid)
						if uid > 0 {
							userID = uid
							log.Printf("[PayPalIPN] Matched user %d from Stripe metadata", userID)
						}
					}
				} else if err != nil {
					log.Printf("[PayPalIPN] Failed to retrieve Stripe PaymentIntent: %v", err)
				}
			}
		}
	}

	// Fallback to email lookup.
	if userID == 0 && payerEmail != "" {
		gdb.Raw("SELECT userid FROM users_emails WHERE email = ? AND userid IS NOT NULL LIMIT 1", payerEmail).Scan(&userID)
		if userID > 0 {
			log.Printf("[PayPalIPN] Matched user %d from payer email %s", userID, payerEmail)
		}
	}

	// Check if this is the user's first donation (for thank-you logic).
	firstDonation := false
	if userID > 0 {
		var previousCount int64
		gdb.Raw("SELECT COUNT(*) FROM users_donations WHERE userid = ?", userID).Scan(&previousCount)
		firstDonation = previousCount == 0
		log.Printf("[PayPalIPN] User %d previous donations: %d, first=%v", userID, previousCount, firstDonation)
	}

	// Parse payment_date — PayPal uses format like "12:34:56 Jan 01, 2026 PST".
	var timestamp string
	if paymentDate != "" {
		parsed, err := parsePayPalDate(paymentDate)
		if err != nil {
			log.Printf("[PayPalIPN] Failed to parse payment_date '%s': %v, using now", paymentDate, err)
			timestamp = time.Now().Format("2006-01-02 15:04:05")
		} else {
			timestamp = parsed.Format("2006-01-02 15:04:05")
		}
	} else {
		timestamp = time.Now().Format("2006-01-02 15:04:05")
	}

	// Record the donation.
	var userIDPtr *uint64
	if userID > 0 {
		userIDPtr = &userID
	}

	result := gdb.Exec(
		"INSERT INTO users_donations (userid, Payer, PayerDisplayName, timestamp, TransactionID, GrossAmount, source, TransactionType, type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		userIDPtr, payerEmail, displayName, timestamp,
		txnID, mcGross, SOURCE_DONATE_WITH_PAYPAL, txnType, TYPE_PAYPAL,
	)

	if result.Error != nil {
		log.Printf("[PayPalIPN] Failed to record donation: %v", result.Error)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to record donation"})
	}

	log.Printf("[PayPalIPN] Recorded donation txn_id=%s for user=%d amount=%s", txnID, userID, mcGross)

	// Handle gift aid notification.
	if userID > 0 {
		handleGiftAidNotification(userID)
	}

	// Determine if recurring.
	recurring := txnType == "recurring_payment" || txnType == "subscr_payment"

	// Queue thank-you email for first recurring or large one-off.
	// Exclude PayPal Giving Fund addresses.
	if userID > 0 && !IsExcludedPayer(payerEmail) {
		// Parse amount for threshold check.
		var amount float64
		gdb.Raw("SELECT GrossAmount FROM users_donations WHERE TransactionID = ? ORDER BY id DESC LIMIT 1", txnID).Scan(&amount)

		if (recurring && firstDonation) || (!recurring && amount >= MANUAL_THANKS) {
			log.Printf("[PayPalIPN] Queuing thank-you for user %d, amount £%.2f, recurring=%v", userID, amount, recurring)

			// Get user name and email for the thank-you.
			var userName, userEmail string
			gdb.Raw("SELECT fullname FROM users WHERE id = ?", userID).Scan(&userName)
			gdb.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", userID).Scan(&userEmail)

			if err := queue.QueueTask(queue.TaskEmailDonateExternal, map[string]interface{}{
				"user_name":  userName,
				"user_id":    userID,
				"user_email": userEmail,
				"amount":     amount,
			}); err != nil {
				log.Printf("[PayPalIPN] Failed to queue thank-you email: %v", err)
			}
		}
	}

	return c.SendStatus(fiber.StatusOK)
}

// IsExcludedPayer checks if a payer email is in the exclusion list (e.g. PayPal Giving Fund).
func IsExcludedPayer(email string) bool {
	for _, excluded := range getExcludedPayers() {
		if strings.EqualFold(email, excluded) {
			return true
		}
	}
	return false
}

// parsePayPalDate tries to parse a PayPal date string.
// PayPal uses formats like "12:34:56 Jan 01, 2026 PST" or PHP strtotime compatible strings.
func parsePayPalDate(dateStr string) (time.Time, error) {
	// Try common PayPal formats.
	formats := []string{
		"15:04:05 Jan 02, 2006 MST",
		"15:04:05 Jan 02, 2006",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		time.RFC1123,
		time.RFC1123Z,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, &time.ParseError{Value: dateStr, Message: "no matching format"}
}
