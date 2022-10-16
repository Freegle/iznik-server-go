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
	"sync"
	"time"
)

type ChatRoomListEntry struct {
	ID            uint64     `json:"id" gorm:"primary_key"`
	Chattype      string     `json:"chattype"`
	User1         uint64     `json:"-"`
	User2         uint64     `json:"-"`
	Otheruid      uint64     `json:"otheruid"`
	Supporter     bool       `json:"supporter"`
	Icon          string     `json:"icon"`
	Lastdate      *time.Time `json:"lastdate"`
	Lastmsg       uint64     `json:"lastmsg"`
	Lastmsgseen   uint64     `json:"lastmsgseen"`
	Name          string     `json:"name"`
	Nameshort     string     `json:"-"`
	Namefull      string     `json:"-"`
	Firstname     string     `json:"-"`
	Lastname      string     `json:"-"`
	Fullname      string     `json:"-"`
	Replyexpected uint64     `json:"replyexpected"`
	Snippet       string     `json:"snippet"`
	Unseen        uint64     `json:"unseen"`
	Chatmsg       string     `json:"-"`
	Chatmsgtype   string     `json:"-"`
	Refmsgtype    string     `json:"-"`
	Gimageid      uint64     `json:"-"`
	U1imageid     uint64     `json:"-"`
	U2imageid     uint64     `json:"-"`
	U1useprofile  bool       `json:"-"`
	U2useprofile  bool       `json:"-"`

	Search bool `json:"-"`
}

func ListForUser(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	since := c.Query("since")

	var start string

	if since != "" {
		t, err := time.Parse(time.RFC3339, since)

		if err == nil {
			start = t.Format("2006-01-02")
		}
	} else {
		start = time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")
	}

	search := c.Query("search")

	r := listChats(myid, start, search, 0)

	return c.JSON(r)
}

func listChats(myid uint64, start string, search string, id uint64) []ChatRoomListEntry {
	var r []ChatRoomListEntry

	// The chats we can see are:
	// - a conversation between two users that we have not closed
	// - (for user2user or user2mod) active in last 31 days
	//
	// A single query that handles this would be horrific, and having tried it, is also hard to make efficient.  So
	// break it down into smaller queries that have the dual advantage of working quickly and being comprehensible.
	var chats []ChatRoomListEntry

	// We don't want to see non-empty chats where all the messages are held for review, because they are likely to
	// be spam.
	countq := " AND (chat_rooms.msgvalid + chat_rooms.msginvalid = 0 OR chat_rooms.msgvalid > 0) "

	if id > 0 {
		countq += " AND chat_rooms.id = " + strconv.FormatUint(id, 10) + " "
	}

	atts := "chat_rooms.id, chat_rooms.chattype, chat_rooms.groupid, chat_rooms.latestmessage"

	sql :=
		"SELECT * FROM (SELECT 0 AS search, 0 AS otheruid, nameshort, namefull, '' AS firstname, '' AS lastname, '' AS fullname, " + atts + " FROM chat_rooms " +
			"INNER JOIN `groups` ON groups.id = chat_rooms.groupid " +
			"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
			"WHERE user1 = ? AND chattype = ? AND latestmessage >= ? AND (status IS NULL OR status != ?) " + countq + " " +
			"UNION " +
			"SELECT 0 AS search, user2 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, " + atts + " FROM chat_rooms " +
			"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
			"INNER JOIN users ON users.id = user2 " +
			"WHERE user1 = ? AND chattype = ? AND latestmessage >= ? AND (status IS NULL OR status NOT IN (?, ?)) " + countq +
			"UNION " +
			"SELECT 0 AS search, user1 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, " + atts + " FROM chat_rooms " +
			"INNER JOIN users ON users.id = user1 " +
			"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
			"WHERE user2 = ? AND chattype = ? AND latestmessage >= ? AND (status IS NULL OR status NOT IN (?, ?)) " + countq

	params := []interface{}{myid, myid, utils.CHAT_TYPE_USER2MOD, start, utils.CHAT_STATUS_CLOSED,
		myid, myid, utils.CHAT_TYPE_USER2USER, start, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED,
		myid, myid, utils.CHAT_TYPE_USER2USER, start, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED,
	}

	if search != "" {
		// We also want to search in the messages.
		sql += "UNION " +
			"SELECT 1 AS search, user2 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, " + atts + " FROM chat_rooms " +
			"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
			"INNER JOIN users ON users.id = user2 " +
			"INNER JOIN chat_messages ON chat_messages.chatid = chat_rooms.id " +
			"LEFT JOIN messages ON messages.id = chat_messages.refmsgid " +
			"WHERE user1 = ? AND chattype = ? AND (status IS NULL OR status NOT IN (?, ?)) " + countq + " " +
			"AND (chat_messages.message LIKE ? OR messages.subject LIKE ?) " +
			"UNION " +
			"SELECT 1 AS search, user1 AS otheruid, '' AS nameshort, '' AS namefull, firstname, lastname, fullname, " + atts + " FROM chat_rooms " +
			"INNER JOIN users ON users.id = user1 " +
			"INNER JOIN chat_messages ON chat_messages.chatid = chat_rooms.id " +
			"LEFT JOIN messages ON messages.id = chat_messages.refmsgid " +
			"LEFT JOIN chat_roster ON chat_roster.userid = ? AND chat_rooms.id = chat_roster.chatid " +
			"WHERE user2 = ? AND chattype = ?  AND (status IS NULL OR status NOT IN (?, ?)) " + countq + " " +
			"AND (chat_messages.message LIKE ? OR messages.subject LIKE ? ) "

		params = append(params,
			myid, myid, utils.CHAT_TYPE_USER2USER, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED, "%"+search+"%", "%"+search+"%",
			myid, myid, utils.CHAT_TYPE_USER2USER, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED, "%"+search+"%", "%"+search+"%",
		)
	}

	sql += ") t  GROUP BY t.id ORDER BY t.latestmessage DESC"

	db := database.DBConn
	db.Raw(sql, params...).Scan(&chats)

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
	// - the supporter info for the chat users
	// This is a beast of a query,
	if len(chats) > 0 {
		var chats2 []ChatRoomListEntry

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()

			ids := []string{}

			for _, chat := range chats {
				ids = append(ids, strconv.FormatUint(chat.ID, 10))
			}

			idlist := "(" + strings.Join(ids, ",") + ") "

			sql = "SELECT DISTINCT chat_rooms.id, chat_rooms.chattype, chat_rooms.groupid, chat_rooms.user1, chat_rooms.user2, " +
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
				"(SELECT chat_roster.lastmsgseen FROM chat_roster WHERE chatid = chat_rooms.id AND userid = ?) AS lastmsgseen, " +
				"messages.type AS refmsgtype, " +
				"rcm.* " +
				"FROM chat_rooms " +
				"LEFT JOIN `groups` ON groups.id = chat_rooms.groupid " +
				"LEFT JOIN users u1 ON chat_rooms.user1 = u1.id " +
				"LEFT JOIN users u2 ON chat_rooms.user2 = u2.id " +
				"LEFT JOIN users_images i1 ON i1.userid = u1.id " +
				"LEFT JOIN users_images i2 ON i2.userid = u2.id " +
				"LEFT JOIN groups_images i3 ON i3.groupid = chat_rooms.groupid " +
				"LEFT JOIN chat_messages ON chat_messages.id = " +
				"  (SELECT id FROM chat_messages WHERE chat_messages.chatid = chat_rooms.id AND reviewrequired = 0 ORDER BY chat_messages.id DESC LIMIT 1) " +
				"LEFT JOIN messages ON messages.id = chat_messages.refmsgid " +
				"LEFT JOIN (WITH cm AS (SELECT chat_messages.id AS lastmsg, chat_messages.chatid, chat_messages.message AS chatmsg," +
				" chat_messages.date AS lastdate, chat_messages.type AS chatmsgtype, ROW_NUMBER() OVER (PARTITION BY chatid ORDER BY id DESC) AS rn " +
				" FROM chat_messages WHERE chatid IN " + idlist + ") " +
				"  SELECT * FROM cm WHERE rn = 1) rcm ON rcm.chatid = chat_rooms.id " +
				"WHERE chat_rooms.id IN " + idlist

			res := db.Raw(sql, myid, utils.CHAT_STATUS_CLOSED, utils.CHAT_STATUS_BLOCKED, myid, myid, start, utils.CHAT_TYPE_USER2USER, myid)
			res.Scan(&chats2)
		}()

		supporterMap := map[uint64]bool{}

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Get the supporter status for the other users.
			ids := []string{}

			ids = append(ids, strconv.FormatUint(myid, 10))

			for _, chat := range chats {
				if chat.Otheruid > 0 {
					ids = append(ids, strconv.FormatUint(chat.Otheruid, 10))
				}
			}

			idlist := "(" + strings.Join(ids, ",") + ") "

			start := time.Now().AddDate(0, 0, -utils.SUPPORTER_PERIOD).Format("2006-01-02")

			var supporters []user.User

			db.Raw("SELECT DISTINCT users.id, (CASE WHEN "+
				"((users.systemrole != 'User' OR "+
				"EXISTS(SELECT id FROM users_donations WHERE userid = users.id AND users_donations.timestamp >= ?) OR "+
				"EXISTS(SELECT id FROM microactions WHERE userid = users.id AND microactions.timestamp >= ?)) AND "+
				"(CASE WHEN JSON_EXTRACT(users.settings, '$.hidesupporter') IS NULL THEN 0 ELSE JSON_EXTRACT(users.settings, '$.hidesupporter') END) = 0) "+
				"THEN 1 ELSE 0 END) "+
				"AS supporter "+
				"FROM users "+
				"WHERE users.id IN "+idlist, start, start).Scan(&supporters)

			// Convert supporters into a map for easy of access below.
			for _, supporter := range supporters {
				supporterMap[supporter.ID] = supporter.Supporter
			}
		}()

		wg.Wait()

		// Combine the data.
		//
		// Scalability isn't great here.
		for ix, chat1 := range chats {
			chats[ix].Supporter = false

			if chat1.Otheruid > 0 {
				// Check if otheruid is in map
				if val, ok := supporterMap[chat1.Otheruid]; ok {
					chats[ix].Supporter = val
				}
			}

			for _, chat := range chats2 {
				if chat1.ID == chat.ID {
					chats[ix].Unseen = chat.Unseen
					chats[ix].Replyexpected = chat.Replyexpected

					if chat.Lastdate != nil {
						chats[ix].Lastdate = chat.Lastdate
						chats[ix].Lastmsg = chat.Lastmsg
						chats[ix].Lastmsgseen = chat.Lastmsgseen
					}

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

					if chats[ix].Search {
						chats[ix].Snippet = "...contains '" + search + "'"
					} else {
						chats[ix].Snippet = getSnippet(chat.Chatmsgtype, chat.Chatmsg, chat.Refmsgtype)
					}

					r = append(r, chats[ix])
					break
				}
			}
		}
	}

	return r
}

func GetChat(c *fiber.Ctx) error {
	// convert id to uint64
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid chat id")
	}

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	chat, err2 := GetChatRoom(id, myid)

	if !err2 {
		return c.JSON(chat)
	}

	return fiber.NewError(fiber.StatusNotFound, "Chat not found")
}

func GetChatRoom(id uint64, myid uint64) (ChatRoomListEntry, bool) {
	chats := listChats(myid, "2009-09-11", "", id)

	if len(chats) > 0 {
		// Found it
		return chats[0], false
	}

	var chat ChatRoomListEntry
	return chat, true
}

func getSnippet(msgtype string, chatmsg string, refmsgtype string) string {
	var ret string

	switch msgtype {
	case utils.CHAT_MESSAGE_ADDRESS:
		ret = "Address sent"
	case utils.CHAT_MESSAGE_NUDGE:
		ret = "Nudged"
	case utils.CHAT_MESSAGE_COMPLETED:
		if refmsgtype == utils.OFFER {
			ret = "Item marked as TAKEN"
		} else {
			ret = "Item marked as RECEIVED"
		}
	case utils.CHAT_MESSAGE_PROMISED:
		ret = "Item promised"
	case utils.CHAT_MESSAGE_RENEGED:
		ret = "Promise cancelled"
	case utils.CHAT_MESSAGE_IMAGE:
		ret = "Image"
	default:
		{
			// We don't want to land in the middle of an encoded emoji otherwise it will display
			//# wrongly.
			ret = splitEmoji(chatmsg)

			if len(ret) > 30 {
				ret = ret[:30]
			}
		}
	}

	return ret
}

func splitEmoji(msg string) string {
	re := regexp.MustCompile("\\\\u.*?\\\\u/")

	without := re.ReplaceAllString(msg, "")

	// If we have something other than emojis, return that.  Otherwise return the emoji(s) which will be
	// rendered in the client.
	if len(without) > 0 {
		msg = without
	}

	return msg
}
