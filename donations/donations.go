package donations

import (
	"os"
	"strconv"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

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
