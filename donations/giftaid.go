package donations

import (
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// GiftAid represents a user's Gift Aid declaration
type GiftAid struct {
	ID                uint64     `json:"id" gorm:"column:id"`
	UserID            uint64     `json:"userid" gorm:"column:userid"`
	Timestamp         time.Time  `json:"timestamp" gorm:"column:timestamp"`
	Period            string     `json:"period" gorm:"column:period"`
	Fullname          string     `json:"fullname" gorm:"column:fullname"`
	Homeaddress       string     `json:"homeaddress" gorm:"column:homeaddress"`
	Deleted           *time.Time `json:"deleted" gorm:"column:deleted"`
	Reviewed          *time.Time `json:"reviewed" gorm:"column:reviewed"`
	Updated           time.Time  `json:"updated" gorm:"column:updated"`
	Postcode          *string    `json:"postcode" gorm:"column:postcode"`
	Housenameornumber *string    `json:"housenameornumber" gorm:"column:housenameornumber"`
}

// GetGiftAid returns the logged-in user's Gift Aid declaration, or dispatches
// to ListGiftAid/SearchGiftAid for admin operations.
// @Summary Get user's Gift Aid declaration
// @Description Returns the Gift Aid declaration for the logged-in user. With all=true returns admin review list. With search=xxx searches records.
// @Tags donations
// @Accept json
// @Produce json
// @Param all query boolean false "Return all records needing review (admin only)"
// @Param search query string false "Search records by name/address (admin only)"
// @Success 200 {object} GiftAid "User's Gift Aid declaration"
// @Failure 401 {object} map[string]string "Not logged in"
// @Failure 404 {object} map[string]string "No Gift Aid declaration found"
// @Router /giftaid [get]
func GetGiftAid(c *fiber.Ctx) error {
	// Dispatch to admin list/search handlers if appropriate query params are present
	if c.Query("all") == "true" {
		return ListGiftAid(c)
	}
	if c.Query("search") != "" {
		return SearchGiftAid(c)
	}

	db := database.DBConn

	// Get user ID from JWT
	userID, _, _ := user.GetJWTFromRequest(c)
	if userID == 0 {
		return c.Status(401).JSON(fiber.Map{
			"error": "Not logged in",
		})
	}

	// Query for user's gift aid record (exclude deleted records)
	var giftaid GiftAid
	result := db.Raw(`
		SELECT id, userid, timestamp, period, fullname, homeaddress,
		       deleted, reviewed, updated, postcode, housenameornumber
		FROM giftaid
		WHERE userid = ? AND deleted IS NULL
		LIMIT 1
	`, userID).Scan(&giftaid)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch Gift Aid declaration",
		})
	}

	// If no record found (ID will be 0), return 404
	if giftaid.ID == 0 {
		return c.Status(404).JSON(fiber.Map{
			"error": "No Gift Aid declaration found",
		})
	}

	// Return the gift aid data at top level (v2 format)
	return c.JSON(giftaid)
}
