package group

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"strconv"
	"time"
)

func GetGroupMessages(c *fiber.Ctx) error {
	var ret []uint64

	id, _ := strconv.ParseUint(c.Params("id"), 10, 64)

	db := database.DBConn

	now := time.Now()
	then := now.AddDate(0, 0, -31)

	// We want to return messages which have no outcome or are successful (which will be shown by the client as
	// freegled) but not withdrawn mmessages.
	db.Raw("SELECT messages_groups.msgid FROM messages_groups "+
		"LEFT JOIN messages_outcomes ON messages_outcomes.msgid = messages_groups.msgid "+
		"WHERE groupid = ? AND arrival >= ? AND collection = ? AND deleted = 0 AND (messages_outcomes.id IS NULL OR messages_outcomes.outcome IN (?, ?)) "+
		"ORDER BY arrival DESC", id, then.Format(time.RFC3339), utils.COLLECTION_APPROVED, utils.OUTCOME_TAKEN, utils.OUTCOME_RECEIVED).Pluck("msgid", &ret)

	return c.JSON(ret)
}
