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

	db.Raw("SELECT msgid FROM messages_groups WHERE groupid = ? AND arrival >= ? AND collection = ? AND deleted = 0 ORDER BY arrival DESC", id, then.Format(time.RFC3339), utils.COLLECTION_APPROVED).Pluck("msgid", &ret)

	return c.JSON(ret)
}
