package chat

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ChatRoomListEntry struct {
	ID            uint64    `json:"id" gorm:"primary_key"`
	Chattype      string    `json:"chattype"`
	User1         uint64    `json:"-"`
	User2         uint64    `json:"-"`
	Otheruid      uint64    `json:"otheruid"`
	Icon          string    `json:"icon"`
	Lastdate      time.Time `json:"lastdate"`
	Lastmsg       uint64    `json:"lastmsg"`
	Lastmsgseen   uint64    `json:"lastmsgsee"`
	Name          string    `json:"name"`
	Nameshort     string    `json:"-"`
	Namefull      string    `json:"-"`
	Firstname     string    `json:"-"`
	Lastname      string    `json:"-"`
	Fullname      string    `json:"-"`
	Replyexpected bool      `json:"replyexpected"`
	Snippet       string    `json:"snippet"`
	Supporter     bool      `json:"supporter"`
	Unseen        uint64    `json:"unseen"`
	Chatmsg       string    `json:"-"`
	Chatmsgtype   string    `json:"-"`
	Refmsgtype    string    `json:"-"`
	Gimageid      uint64    `json:"-"`
	U1imageid     uint64    `json:"-"`
	U2imageid     uint64    `json:"-"`
	U1useprofile  bool      `json:"-"`
	U2useprofile  bool      `json:"-"`
}

func ListForUser(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// The chats we can see are:
	// - a conversation between two users that we have not closed
	// - (for user2user or user2mod) active in last 31 days
	//
	// A single query that handles this would be horrific, and having tried it, is also hard to make efficient.  So
	// break it down into smaller queries that have the dual advantage of working quickly and being comprehensible.
	var chats []ChatRoomListEntry

	start := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

	//We don't want to see non-empty chats where all the messages are held for review, because they are likely to
	// be spam.
	countq := " AND (chat_rooms.msgvalid + chat_rooms.msginvalid = 0 OR chat_rooms.msgvalid > 0) "

	atts := "chat_rooms.id, chat_rooms.chattype, chat_rooms.groupid"

	sql := "SELECT 0 AS otheruid, nameshort, namefull, '' AS firstname, '' AS lastname, '' AS fullname, " + atts + " FROM chat_rooms " +
		"INNER JOIN `groups` ON groups.id = chat_rooms.groupid " +
		"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
		"WHERE user1 = ? AND chattype = ? AND latestmessage >= ? AND (status IS NULL OR status != ?) " + countq + " " +
		"UNION " +
		"SELECT user2 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, " + atts + " FROM chat_rooms " +
		"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
		"INNER JOIN users ON users.id = user2 " +
		"WHERE user1 = ? AND chattype = ? AND latestmessage >= ? AND (status IS NULL OR status NOT IN (?, ?)) " + countq +
		"UNION " +
		"SELECT user1 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, " + atts + " FROM chat_rooms " +
		"INNER JOIN users ON users.id = user1 " +
		"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
		"WHERE user2 = ? AND chattype = ? AND latestmessage >= ? AND (status IS NULL OR status NOT IN (?, ?)) " + countq

	db := database.DBConn
	db.Raw(sql,
		myid, myid, utils.CHAT_TYPE_USER2MOD, start, utils.CHAT_STATUS_CLOSED,
		myid, myid, utils.CHAT_TYPE_USER2USER, start, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED,
		myid, myid, utils.CHAT_TYPE_USER2USER, start, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED,
	).Scan(&chats)

	// We hide the "-gxxx" part of names, which will almost always be for TN members.
	tnre := regexp.MustCompile(utils.TN_REGEXP)

	for ix, chat := range chats {
		if chat.Chattype == utils.CHAT_TYPE_USER2MOD {
			if len(chat.Namefull) > 0 {
				chats[ix].Name = chat.Namefull + " Volunteers"
			} else {
				chats[ix].Name = chat.Namefull + " Volunteers"
			}

			chats[ix].Name = tnre.ReplaceAllString(chats[ix].Name, "$1")
		} else {
			if len(chat.Fullname) > 0 {
				chats[ix].Name = chat.Fullname
			} else {
				chats[ix].Name = chat.Firstname + " " + chat.Lastname
			}
		}
	}

	// Now we have the basic chat info.  We still need:
	// - the most recent chat message (if any) for a snippet
	// - the count of unread messages for the logged in user
	// - the count of reply requested from other people
	// - the last seen for this user.
	// - the profile pic and setting about whether to show it
	// This is a beast of a query,
	if len(chats) > 0 {
		ids := []string{}

		for _, chat := range chats {
			ids = append(ids, strconv.FormatUint(chat.ID, 10))
		}

		idlist := "(" + strings.Join(ids, ",") + ") "

		sql = "SELECT chat_rooms.id, chat_rooms.chattype, chat_rooms.groupid, chat_rooms.user1, chat_rooms.user2, " +
			"CASE WHEN JSON_EXTRACT(u1.settings, '$.useprofile') IS NULL THEN 1 ELSE JSON_EXTRACT(u1.settings, '$.useprofile') END AS u1useprofile, " +
			"CASE WHEN JSON_EXTRACT(u2.settings, '$.useprofile') IS NULL THEN 1 ELSE JSON_EXTRACT(u2.settings, '$.useprofile') END AS u2useprofile, " +
			"(SELECT COUNT(*) AS count FROM chat_messages WHERE id > " +
			"  COALESCE((SELECT lastmsgseen FROM chat_roster WHERE chatid = chat_rooms.id AND userid = ? " +
			"  AND status != ? AND status != ?), 0) AND chatid = chat_rooms.id AND userid != ?) AS unseen, " +
			"(SELECT COUNT(*) AS count FROM chat_messages WHERE chatid = chat_rooms.id AND replyexpected = 1 AND" +
			"  replyreceived = 0 AND userid != ? AND chat_messages.date >= ? AND chat_rooms.chattype = ?) AS replyexpected, " +
			"i1.id AS u1imageid, " +
			"i2.id AS u2imageid, " +
			"i3.id AS gimageid, " +
			"chat_messages.id AS lastmsg, chat_messages.message AS chatmsg, chat_messages.date AS lastdate, chat_messages.type AS chatmsgtype, " +
			"(SELECT chat_roster.lastmsgseen FROM chat_roster WHERE chatid = chat_rooms.id AND userid = ?) AS lastmsgseen, " +
			"messages.type AS refmsgtype " +
			"FROM chat_rooms " +
			"LEFT JOIN `groups` ON groups.id = chat_rooms.groupid " +
			"LEFT JOIN users u1 ON chat_rooms.user1 = u1.id " +
			"LEFT JOIN users u2 ON chat_rooms.user2 = u2.id " +
			"LEFT JOIN users_images i1 ON i1.userid = u1.id " +
			"LEFT JOIN users_images i2 ON i2.userid = u2.id " +
			"LEFT JOIN groups_images i3 ON i3.groupid = chat_rooms.groupid " +
			"LEFT JOIN chat_messages ON chat_messages.id = " +
			"  (SELECT id FROM chat_messages WHERE chat_messages.chatid = chat_rooms.id ORDER BY chat_messages.id DESC LIMIT 1) " +
			"LEFT JOIN messages ON messages.id = chat_messages.refmsgid " +
			"WHERE chat_rooms.id IN " + idlist

		var chats2 []ChatRoomListEntry
		res := db.Debug().Raw(sql, myid, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED, myid, myid, start, utils.CHAT_TYPE_USER2USER, myid)
		res.Scan(&chats2)

		// Combine the data.
		//
		// Scalability isn't great here.
		for ix, chat1 := range chats {
			for _, chat := range chats2 {
				if chat1.ID == chat.ID {
					chats[ix].Unseen = chat.Unseen
					chats[ix].Replyexpected = chat.Replyexpected
					chats[ix].Lastdate = chat.Lastdate
					chats[ix].Lastmsgseen = chat.Lastmsgseen

					if chat.Chattype == utils.CHAT_TYPE_USER2MOD {
						chats[ix].Icon = "https://" + os.Getenv("USER_SITE") + "/gimg_" + strconv.FormatUint(chat.Gimageid, 10) + ".jpg"
					} else {
						if chat.User1 == myid {
							if chat.U2useprofile && chat.U2imageid > 0 {
								chats[ix].Icon = "https://" + os.Getenv("USER_SITE") + "/uimg_" + strconv.FormatUint(chat.U2imageid, 10) + ".jpg"
							} else {
								chats[ix].Icon = "https://" + os.Getenv("USER_SITE") + "/defaultprofile.png"
							}
						} else {
							if chat.U1useprofile && chat.U1imageid > 0 {
								chats[ix].Icon = "https://" + os.Getenv("USER_SITE") + "/uimg_" + strconv.FormatUint(chat.U1imageid, 10) + ".jpg"
							} else {
								chats[ix].Icon = "https://" + os.Getenv("USER_SITE") + "/defaultprofile.png"
							}
						}
					}

					// TODO snippet, refmsgtype
				}
			}
		}
	}

	// TODO Search

	return c.JSON(chats)
}
