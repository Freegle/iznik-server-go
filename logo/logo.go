package logo

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"os"
	"time"
)

type Logo struct {
	ID     uint64 `json:"id" gorm:"primary_key"`
	Path   string `json:"path"`
	Date   string `json:"date"`
	Reason string `json:"reason"`
	Active bool   `json:"active"`
}

// Get returns a random active logo for today's date
func Get(c *fiber.Ctx) error {
	db := database.DBConn

	// Get current date in m-d format (same as PHP version)
	now := time.Now()
	todayDate := now.Format("01-02") // Format: month-day (mm-dd)

	var logo Logo

	// Query for active logos matching today's date, randomly ordered
	result := db.Raw("SELECT * FROM logos WHERE `date` LIKE ? AND active = 1 ORDER BY RAND() LIMIT 1;", todayDate).Scan(&logo)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}

	// If no logo found, return empty result (matching v1 behavior)
	if logo.ID == 0 {
		return c.JSON(fiber.Map{
			"logo": nil,
			"date": todayDate,
		})
	}

	// Construct full path for mobile app compatibility (same as v1)
	userSite := os.Getenv("USER_SITE")
	fullPath := "https://" + userSite + logo.Path

	// Return logo with full path
	return c.JSON(fiber.Map{
		"logo": fiber.Map{
			"id":     logo.ID,
			"path":   fullPath,
			"date":   logo.Date,
			"reason": logo.Reason,
			"active": logo.Active,
		},
		"date": todayDate,
	})
}
