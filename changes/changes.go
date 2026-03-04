package changes

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"time"
)

type MessageChange struct {
	ID        uint64 `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
}

type UserChange struct {
	ID          uint64 `json:"id"`
	LastUpdated string `json:"lastupdated"`
}

type Rating struct {
	Rater     uint64 `json:"rater"`
	Ratee     uint64 `json:"ratee"`
	Rating    string `json:"rating"`
	Timestamp string `json:"timestamp"`
	Visible   int    `json:"visible"`
}

// GetChanges returns message changes, user changes, and optionally ratings since a given time.
// Requires partner key authentication via the partner query parameter.
// @Summary Get changes since a timestamp
// @Tags changes
// @Produce json
// @Param since query string false "ISO8601 timestamp (defaults to 1 hour ago)"
// @Param partner query string true "Partner API key"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} fiber.Error "Invalid partner key"
// @Router /api/changes [get]
func GetChanges(c *fiber.Ctx) error {
	// Partner authentication is required.
	partner := c.Query("partner", "")
	if partner == "" {
		return fiber.NewError(fiber.StatusForbidden, "Partner key required")
	}

	db := database.DBConn

	var partnerID uint64
	db.Raw("SELECT id FROM partners_keys WHERE `key` = ?", partner).Scan(&partnerID)

	if partnerID == 0 {
		return fiber.NewError(fiber.StatusForbidden, "Invalid partner key")
	}

	// Parse since parameter - default to 1 hour ago.
	sinceStr := c.Query("since", "")
	var since time.Time

	if sinceStr != "" {
		parsed, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			// Try MySQL-style datetime format.
			parsed, err = time.Parse("2006-01-02 15:04:05", sinceStr)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "Invalid since parameter")
			}
		}
		since = parsed
	} else {
		since = time.Now().Add(-1 * time.Hour)
	}

	mysqlTime := since.Format("2006-01-02 15:04:05")

	// Fetch message changes, user changes, and ratings.
	var messages []MessageChange
	db.Raw("SELECT id, deleted AS timestamp, 'Deleted' AS `type` FROM messages WHERE deleted > ? "+
		"UNION SELECT msgid AS id, timestamp, outcome AS `type` FROM messages_outcomes WHERE timestamp > ? "+
		"UNION SELECT messages_edits.msgid AS id, timestamp, 'Edited' AS `type` FROM messages_edits "+
		"INNER JOIN messages_groups ON messages_groups.msgid = messages_edits.msgid AND collection = ? WHERE timestamp > ? "+
		"UNION SELECT msgid AS id, promisedat AS timestamp, 'Promised' AS `type` FROM messages_promises WHERE promisedat > ? "+
		"UNION SELECT msgid AS id, timestamp, 'Reneged' AS `type` FROM messages_reneged WHERE timestamp > ? "+
		"UNION SELECT msgid AS id, arrival AS timestamp, 'ApprovedOrReposted' AS `type` FROM messages_groups "+
		"WHERE messages_groups.arrival > ? AND messages_groups.collection = ?",
		mysqlTime, mysqlTime, utils.COLLECTION_APPROVED, mysqlTime, mysqlTime, mysqlTime, mysqlTime, utils.COLLECTION_APPROVED).Scan(&messages)

	var users []UserChange
	db.Raw("SELECT id, lastupdated FROM users WHERE lastupdated >= ?", mysqlTime).Scan(&users)

	var ratings []Rating
	db.Raw("SELECT rater, ratee, rating, timestamp, visible FROM ratings WHERE timestamp >= ? AND visible = 1", mysqlTime).Scan(&ratings)

	// Format timestamps to ISO8601.
	for i := range messages {
		messages[i].Timestamp = formatISO(messages[i].Timestamp)
	}
	for i := range users {
		users[i].LastUpdated = formatISO(users[i].LastUpdated)
	}
	for i := range ratings {
		ratings[i].Timestamp = formatISO(ratings[i].Timestamp)
	}

	// Ensure empty arrays rather than null in JSON.
	if messages == nil {
		messages = make([]MessageChange, 0)
	}
	if users == nil {
		users = make([]UserChange, 0)
	}
	if ratings == nil {
		ratings = make([]Rating, 0)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"changes": fiber.Map{
			"messages": messages,
			"users":    users,
			"ratings":  ratings,
		},
	})
}

// formatISO converts a MySQL datetime string to ISO8601 format.
func formatISO(mysqlTime string) string {
	t, err := time.Parse("2006-01-02 15:04:05", mysqlTime)
	if err != nil {
		return mysqlTime
	}
	return t.Format(time.RFC3339)
}
