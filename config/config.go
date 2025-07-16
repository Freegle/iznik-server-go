package config

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"strconv"
	"strings"
)

type ConfigItem struct {
	ID    uint64 `json:"id" gorm:"primary_key"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SpamKeyword struct {
	ID      uint64  `json:"id" gorm:"primary_key"`
	Word    string  `json:"word"`
	Exclude *string `json:"exclude"`
	Action  string  `json:"action"` // 'Spam', 'Review', 'Whitelist'
	Type    string  `json:"type"`   // 'Literal', 'Regex'
}

func (SpamKeyword) TableName() string {
	return "spam_keywords"
}

type WorryWord struct {
	ID        uint64 `json:"id" gorm:"primary_key"`
	Keyword   string `json:"keyword" gorm:"column:keyword"`
	Substance string `json:"substance"`
	Type      string `json:"type"`
}

func (WorryWord) TableName() string {
	return "worrywords"
}

type CreateSpamKeywordRequest struct {
	Word    string  `json:"word" validate:"required"`
	Exclude *string `json:"exclude"`
	Action  string  `json:"action" validate:"required,oneof=Spam Review Whitelist"`
	Type    string  `json:"type" validate:"required,oneof=Literal Regex"`
}

type CreateWorryWordRequest struct {
	Keyword   string `json:"keyword" validate:"required"`
	Substance string `json:"substance"`
	Type      string `json:"type"`
}

// RequireSupportOrAdminMiddleware creates middleware that checks for Support/Admin role
func RequireSupportOrAdminMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract JWT information including session ID
		userID, sessionID, _ := user.GetJWTFromRequest(c)
		if userID == 0 {
			return fiber.NewError(fiber.StatusUnauthorized, "Authentication required")
		}

		db := database.DBConn

		// Validate that the user and session are still valid in the database
		var userInfo struct {
			ID         uint64 `json:"id"`
			Systemrole string `json:"systemrole"`
		}

		db.Raw("SELECT users.id, users.systemrole FROM sessions INNER JOIN users ON users.id = sessions.userid WHERE sessions.id = ? AND users.id = ? LIMIT 1", sessionID, userID).Scan(&userInfo)

		if userInfo.ID == 0 {
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid session")
		}

		if userInfo.Systemrole != "Support" && userInfo.Systemrole != "Admin" {
			return fiber.NewError(fiber.StatusForbidden, "Support or Admin role required")
		}

		return c.Next()
	}
}

func Get(c *fiber.Ctx) error {
	key := c.Params("key")

	var items []ConfigItem

	db := database.DBConn

	db.Raw("SELECT * FROM config WHERE `key` = ?", key).Scan(&items)

	if len(items) > 0 {
		return c.JSON(items)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}

// Spam Keywords endpoints

func ListSpamKeywords(c *fiber.Ctx) error {
	var keywords []SpamKeyword
	db := database.DBConn

	db.Order("word ASC").Find(&keywords)

	return c.JSON(keywords)
}

func CreateSpamKeyword(c *fiber.Ctx) error {
	var req CreateSpamKeywordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Basic validation
	if strings.TrimSpace(req.Word) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Word is required")
	}
	if req.Action != "Spam" && req.Action != "Review" && req.Action != "Whitelist" {
		return fiber.NewError(fiber.StatusBadRequest, "Action must be 'Spam', 'Review', or 'Whitelist'")
	}
	if req.Type != "Literal" && req.Type != "Regex" {
		return fiber.NewError(fiber.StatusBadRequest, "Type must be 'Literal' or 'Regex'")
	}

	keyword := SpamKeyword{
		Word:    strings.TrimSpace(req.Word),
		Exclude: req.Exclude,
		Action:  req.Action,
		Type:    req.Type,
	}

	db := database.DBConn
	result := db.Create(&keyword)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create spam keyword")
	}

	return c.Status(fiber.StatusOK).JSON(keyword)
}

func DeleteSpamKeyword(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
	}

	db := database.DBConn
	result := db.Delete(&SpamKeyword{}, id)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete spam keyword")
	}

	if result.RowsAffected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Spam keyword not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true})
}

// Worry Words endpoints

func ListWorryWords(c *fiber.Ctx) error {
	var words []WorryWord
	db := database.DBConn

	db.Order("keyword ASC").Find(&words)

	return c.JSON(words)
}

func CreateWorryWord(c *fiber.Ctx) error {
	var req CreateWorryWordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Basic validation
	if strings.TrimSpace(req.Keyword) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Keyword is required")
	}

	word := WorryWord{
		Keyword:   strings.TrimSpace(req.Keyword),
		Substance: req.Substance,
		Type:      req.Type,
	}

	db := database.DBConn
	result := db.Create(&word)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create worry word")
	}

	return c.Status(fiber.StatusOK).JSON(word)
}

func DeleteWorryWord(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
	}

	db := database.DBConn
	result := db.Delete(&WorryWord{}, id)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete worry word")
	}

	if result.RowsAffected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Worry word not found")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true})
}
