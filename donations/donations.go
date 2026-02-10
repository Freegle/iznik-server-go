package donations

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

const TYPE_PAYPAL = "PayPal"
const TYPE_EXTERNAL = "External"
const TYPE_OTHER = "Other"
const TYPE_STRIPE = "Stripe"

const SOURCE_BANK_TRANSFER = "BankTransfer"

const PERIOD_THIS = "This"

// Default values match current production configuration in /etc/iznik.conf
// These can be overridden via environment variables DONATION_TARGET and DONATIONS_EXCLUDE
const DEFAULT_DONATION_TARGET = 2000                                                         // Matches DONATION_TARGET in /etc/iznik.conf
const DEFAULT_DONATIONS_EXCLUDE = "ppgfukpay@paypalgivingfund.org,paypal.msb@tipalti.com" // Matches DONATIONS_EXCLUDE in /etc/iznik.conf

// getDonationTarget returns the donation target from env var or default
func getDonationTarget() int {
	if target := os.Getenv("DONATION_TARGET"); target != "" {
		if val, err := strconv.Atoi(target); err == nil {
			return val
		}
	}
	return DEFAULT_DONATION_TARGET
}

// getExcludedPayers returns list of emails to exclude from donation counts
//
// Excluded emails come from the DONATIONS_EXCLUDE environment variable (comma-separated).
// These are typically:
// - ppgfukpay@paypalgivingfund.org: PayPal Giving Fund payments (already donated through other channels)
// - paypal.msb@tipalti.com: Tipalti payment processor (internal transfers, not actual donations)
//
// The exclusion list is configurable via environment variable to handle future payment processors
// or partnership accounts that shouldn't count toward donation targets.
//
// Source: Copied from v1 PHP Donations::getExcludedPayersCondition() in iznik-server/include/misc/Donations.php
func getExcludedPayers() []string {
	exclude := os.Getenv("DONATIONS_EXCLUDE")
	if exclude == "" {
		exclude = DEFAULT_DONATIONS_EXCLUDE
	}
	emails := strings.Split(exclude, ",")
	var result []string
	for _, email := range emails {
		if trimmed := strings.TrimSpace(email); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// GetDonations returns donation target and amount raised for the current month
// @Summary Get donations summary
// @Description Returns the donation target and amount raised for the current month, optionally filtered by group
// @Tags donations
// @Accept json
// @Produce json
// @Param groupid query int false "Group ID to filter donations"
// @Success 200 {object} map[string]interface{} "Donation summary with target and raised amounts"
// @Router /donations [get]
func GetDonations(c *fiber.Ctx) error {
	db := database.DBConn

	// Get optional groupid parameter
	groupID := c.Query("groupid")

	var target int
	var raised float64

	// Get target - from group if specified, otherwise use default from env
	target = getDonationTarget()
	if groupID != "" {
		var fundingtarget *int
		db.Raw("SELECT fundingtarget FROM `groups` WHERE id = ?", groupID).Scan(&fundingtarget)
		if fundingtarget != nil && *fundingtarget > 0 {
			target = *fundingtarget
		}
	}

	// Get raised amount for current month
	// If groupid specified, only count donations from members of that group
	// Exclude certain payers (eBay partnerships, PayPal Giving Fund) from totals
	excludedPayers := getExcludedPayers()

	query := `
		SELECT COALESCE(SUM(GrossAmount), 0) AS raised
		FROM users_donations
	`

	if groupID != "" {
		query += ` INNER JOIN memberships ON users_donations.userid = memberships.userid
		           AND memberships.groupid = ?
		`
	}

	query += ` WHERE timestamp >= DATE_FORMAT(NOW(), '%Y-%m-01')`

	// Build exclusion condition dynamically
	for range excludedPayers {
		query += ` AND Payer != ?`
	}

	// Build query arguments
	var args []interface{}
	if groupID != "" {
		args = append(args, groupID)
	}
	for _, email := range excludedPayers {
		args = append(args, email)
	}

	db.Raw(query, args...).Scan(&raised)

	return c.JSON(fiber.Map{
		"target": target,
		"raised": raised,
	})
}

// AddDonation records an external bank transfer donation.
// @Summary Record an external donation
// @Description Records a donation made via bank transfer. Requires GiftAid permission for non-zero amounts.
// @Tags donations
// @Accept json
// @Produce json
// @Param body body AddDonationRequest true "Donation details"
// @Success 200 {object} map[string]interface{} "Donation recorded with id"
// @Failure 401 {object} map[string]string "Not logged in"
// @Failure 403 {object} map[string]string "Permission denied"
// @Router /donations [put]
func AddDonation(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type AddDonationRequest struct {
		UserID uint64  `json:"userid"`
		Amount float64 `json:"amount"`
		Date   string  `json:"date"`
	}

	var req AddDonationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.UserID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing userid")
	}

	db := database.DBConn

	// Permission check: need GiftAid permission for non-zero amounts.
	if req.Amount > 0 {
		var permissions *string
		db.Raw("SELECT permissions FROM users WHERE id = ?", myid).Scan(&permissions)

		hasGiftAid := false
		if permissions != nil {
			hasGiftAid = strings.Contains(strings.ToLower(*permissions), "giftaid")
		}

		if !hasGiftAid {
			return fiber.NewError(fiber.StatusForbidden, "Permission denied")
		}
	}

	// Look up the target user's name and preferred email.
	var fullname *string
	var preferredEmail string

	db.Raw("SELECT fullname FROM users WHERE id = ?", req.UserID).Scan(&fullname)
	if fullname == nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid userid")
	}

	name := *fullname

	// Get preferred email: external email first, then any email.
	db.Raw(`SELECT email FROM users_emails
		WHERE userid = ?
		ORDER BY preferred DESC, email ASC
		LIMIT 1`, req.UserID).Scan(&preferredEmail)

	if preferredEmail == "" {
		preferredEmail = "unknown"
	}

	// Build transaction ID matching PHP format.
	transactionID := fmt.Sprintf("External for #%d added at %s%s",
		req.UserID, time.Now().UTC().Format("2006-01-02 15:04:05"), SOURCE_BANK_TRANSFER)

	// Insert donation with ON DUPLICATE KEY UPDATE (TransactionID is unique).
	result := db.Exec(`INSERT INTO users_donations
		(userid, Payer, PayerDisplayName, timestamp, TransactionID, GrossAmount, type, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE userid = VALUES(userid), timestamp = VALUES(timestamp)`,
		req.UserID, preferredEmail, name, req.Date, transactionID, req.Amount, TYPE_EXTERNAL, SOURCE_BANK_TRANSFER)

	if result.Error != nil {
		log.Printf("Failed to add donation for user %d: %v", req.UserID, result.Error)
		return fiber.NewError(fiber.StatusInternalServerError, "Add failed")
	}

	// Get the inserted ID.
	var donationID uint64
	db.Raw("SELECT id FROM users_donations WHERE TransactionID = ?", transactionID).Scan(&donationID)

	if donationID == 0 {
		return fiber.NewError(fiber.StatusInternalServerError, "Add failed")
	}

	// For non-zero amounts: create Gift Aid notification and queue email.
	if req.Amount > 0 {
		// Check if user needs a Gift Aid prompt.
		var giftAidPeriod *string
		db.Raw("SELECT period FROM giftaid WHERE userid = ? AND deleted IS NULL LIMIT 1", req.UserID).Scan(&giftAidPeriod)

		if giftAidPeriod == nil || *giftAidPeriod == PERIOD_THIS {
			// Create a GiftAid notification for the user.
			db.Exec("INSERT INTO users_notifications (touser, type, timestamp, seen) VALUES (?, 'GiftAid', NOW(), 0)",
				req.UserID)
		}

		// Queue email to info@ilovefreegle.org.
		queue.QueueTask(queue.TaskEmailDonateExternal, map[string]interface{}{
			"user_id":    req.UserID,
			"user_name":  name,
			"user_email": preferredEmail,
			"amount":     req.Amount,
		})
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"id":     donationID,
	})
}
