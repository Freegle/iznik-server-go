package misc

import (
	"database/sql"
	"os"
	"regexp"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

// IllustrationResult is the response structure for the illustration endpoint.
type IllustrationResult struct {
	Ret          int                    `json:"ret"`
	Status       string                 `json:"status"`
	Illustration *IllustrationData      `json:"illustration,omitempty"`
}

// IllustrationData contains the AI illustration details.
type IllustrationData struct {
	ExternalUID string `json:"externaluid"`
	URL         string `json:"url"`
	Cached      bool   `json:"cached"`
}

// GetIllustration returns an AI illustration for an item name.
// This endpoint checks the cache and returns cached illustrations.
// If not cached, returns ret=3 so the frontend can fall back to PHP API.
func GetIllustration(c *fiber.Ctx) error {
	item := c.Query("item")

	if item == "" || strings.TrimSpace(item) == "" {
		return c.JSON(IllustrationResult{
			Ret:    2,
			Status: "Item name required",
		})
	}

	// Clean and normalise the item name.
	itemName := strings.TrimSpace(item)

	// Remove OFFER/WANTED prefix if present.
	prefixPattern := regexp.MustCompile(`(?i)^(OFFER|WANTED|TAKEN|RECEIVED):\s*`)
	itemName = prefixPattern.ReplaceAllString(itemName, "")

	// Remove location suffix in parentheses if present.
	suffixPattern := regexp.MustCompile(`\s*\([^)]+\)\s*$`)
	itemName = suffixPattern.ReplaceAllString(itemName, "")

	itemName = strings.TrimSpace(itemName)

	if itemName == "" {
		return c.JSON(IllustrationResult{
			Ret:    2,
			Status: "Item name required",
		})
	}

	// Check the cache.
	db := database.DBConn
	var externalUID sql.NullString

	err := db.Raw("SELECT externaluid FROM ai_images WHERE name = ?", itemName).Scan(&externalUID).Error

	if err != nil || !externalUID.Valid || externalUID.String == "" {
		// Not cached - frontend should fall back to PHP API.
		return c.JSON(IllustrationResult{
			Ret:    3,
			Status: "Not cached - use PHP API for generation",
		})
	}

	// Build the image URL.
	imagesHost := os.Getenv("IMAGES_HOST")
	if imagesHost == "" {
		imagesHost = "https://images.ilovefreegle.org"
	}

	return c.JSON(IllustrationResult{
		Ret:    0,
		Status: "Success",
		Illustration: &IllustrationData{
			ExternalUID: externalUID.String,
			URL:         imagesHost + "/" + externalUID.String,
			Cached:      true,
		},
	})
}
