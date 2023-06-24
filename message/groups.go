package message

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"time"
)

func Groups(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	msgs := []MessageSummary{}

	start := time.Now().AddDate(0, 0, -utils.OPEN_AGE).Format("2006-01-02")

	// We want to include our own messages, so that it is less obvious if a message is delayed for approval and
	// hasn't made it into messages_spatial yet.
	db.Raw("SELECT * FROM ("+
		"SELECT ST_Y(point) AS lat, "+
		"ST_X(point) AS lng, "+
		"messages_spatial.msgid AS id, "+
		"messages_spatial.successful, "+
		"messages_spatial.promised, "+
		"messages_spatial.groupid, "+
		"messages_spatial.msgtype AS type, "+
		"messages_spatial.arrival "+
		"FROM messages_spatial "+
		"INNER JOIN memberships ON memberships.groupid = messages_spatial.groupid "+
		"WHERE memberships.userid = ? "+
		"UNION "+
		"SELECT lat, lng, messages.id, "+
		"(CASE WHEN messages_outcomes.outcome IN (?, ?) THEN 1 ELSE 0 END) AS successful, "+
		"(CASE WHEN messages_promises.id IS NOT NULL THEN 1 ELSE 0 END) AS promised, "+
		"messages_groups.groupid, "+
		"type,"+
		"messages_groups.arrival "+
		"FROM messages "+
		"INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
		"LEFT JOIN messages_outcomes ON messages_outcomes.msgid = messages.id "+
		"LEFT JOIN messages_promises ON messages_promises.msgid = messages.id "+
		"WHERE fromuser = ? AND messages_groups.arrival >= ? "+
		"AND messages_outcomes.id IS NULL "+
		") t "+
		"ORDER BY arrival DESC, id DESC;",
		myid,
		utils.OUTCOME_TAKEN,
		utils.OUTCOME_RECEIVED,
		myid,
		start).Scan(&msgs)

	for ix, r := range msgs {
		// Protect anonymity of poster a bit.
		msgs[ix].Lat, msgs[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
	}

	return c.JSON(msgs)
}
