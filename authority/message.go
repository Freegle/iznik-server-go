package authority

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

func Messages(c *fiber.Ctx) error {
	id := c.Params("id", "0")
	db := database.DBConn

	myid := user.WhoAmI(c)

	msgs := []message.MessageSummary{}

	db.Raw("SELECT ST_Y(point) AS lat, "+
		"ST_X(point) AS lng, "+
		"messages_spatial.msgid AS id, "+
		"messages_spatial.successful, "+
		"messages_spatial.promised, "+
		"messages_spatial.groupid, "+
		"messages_spatial.msgtype AS type, "+
		"messages_spatial.arrival, "+
		"CASE WHEN messages_likes.msgid IS NULL THEN 1 ELSE 0 END AS unseen "+
		"FROM messages_spatial "+
		"INNER JOIN authorities ON ST_Contains(authorities.polygon, point) "+
		"INNER JOIN `groups` ON groups.id = messages_spatial.groupid "+
		"LEFT JOIN messages_likes ON messages_likes.msgid = messages_spatial.msgid AND messages_likes.userid = ? AND messages_likes.type = 'View' "+
		"WHERE authorities.id = ? AND messages_spatial.msgid > 0 "+
		"ORDER BY unseen DESC, messages_spatial.arrival DESC, messages_spatial.msgid DESC;", myid, id).Scan(&msgs)

	for ix, r := range msgs {
		// Protect anonymity of poster a bit.
		msgs[ix].Lat, msgs[ix].Lng = utils.Blur(r.Lat, r.Lng, utils.BLUR_USER)
	}

	return c.JSON(msgs)
}
