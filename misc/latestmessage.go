package misc

import (
	"database/sql"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

type LatestMessageResult struct {
	Ret           int    `json:"ret"`
	Status        string `json:"status"`
	LatestMessage string `json:"latestmessage,omitempty"`
}

func LatestMessage(c *fiber.Ctx) error {
	var latestMessage sql.NullString

	db := database.DBConn

	err := db.Raw("SELECT MAX(arrival) FROM messages").Scan(&latestMessage).Error

	if err != nil {
		return c.JSON(LatestMessageResult{
			Ret:    1,
			Status: "Error",
		})
	}

	if !latestMessage.Valid || latestMessage.String == "" {
		return c.JSON(LatestMessageResult{
			Ret:    1,
			Status: "No messages found",
		})
	}

	return c.JSON(LatestMessageResult{
		Ret:           0,
		Status:        "Success",
		LatestMessage: latestMessage.String,
	})
}
