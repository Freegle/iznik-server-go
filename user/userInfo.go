package user

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"sync"
	"time"
)

type Ratings struct {
	Up   uint64
	Down uint64
	Mine string
}

type Publiclocation struct {
	Display   string `json:"display"`
	Groupid   uint64 `json:"groupid"`
	Groupname string `json:"groupname"`
	Location  string `json:"location"`
}

type PrivatePosition struct {
	Lat  float32 `json:"lat"`
	Lng  float32 `json:"lng"`
	Name string  `json:"name,omitempty"`
	Loc  string  `json:"loc,omitempty"`
}

type UserInfo struct {
	Replies         uint64          `json:"replies"`
	Repliesoffer    uint64          `json:"repliesoffer"`
	Replieswanted   uint64          `json:"replieswanted"`
	Taken           uint64          `json:"taken"`
	Reneged         uint64          `json:"reneged"`
	Collected       uint64          `json:"collected"`
	Offers          uint64          `json:"offers"`
	Wanteds         uint64          `json:"wanteds"`
	Openoffers      uint64          `json:"openoffers"`
	Openwanteds     uint64          `json:"openwanteds"`
	Expectedreply   uint64          `json:"expectedreply"`
	Expectedreplies uint64          `json:"expectedreplies"`
	Openage         uint64          `json:"openage"`
	Replytime       uint64          `json:"replytime"`
	Ratings         Ratings         `json:"ratings" gorm:"-"`
	Publiclocation  *Publiclocation `json:"publiclocation,omitempty" gorm:"-"`
}

func GetUserInfo(id uint64, myid uint64) UserInfo {
	db := database.DBConn

	var info UserInfo
	var mu sync.Mutex

	info.Replies = 0
	info.Reneged = 0
	info.Collected = 0
	info.Openage = utils.OPEN_AGE

	start := time.Now().AddDate(0, 0, -utils.OPEN_AGE).Format("2006-01-02")

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Count replies, split by message type (Offer vs Wanted).
		type replyCount struct {
			Count   uint64
			Msgtype string
		}
		var counts []replyCount
		db.Raw("SELECT COUNT(DISTINCT cm.refmsgid) AS count, m.type AS msgtype "+
			"FROM chat_messages cm "+
			"INNER JOIN messages m ON m.id = cm.refmsgid "+
			"WHERE cm.userid = ? AND cm.date > ? AND cm.refmsgid IS NOT NULL AND cm.type = ? "+
			"GROUP BY m.type", id, start, utils.CHAT_MESSAGE_INTERESTED).Scan(&counts)
		mu.Lock()
		defer mu.Unlock()
		for _, c := range counts {
			info.Replies += c.Count
			switch c.Msgtype {
			case utils.OFFER:
				info.Repliesoffer = c.Count
			case utils.WANTED:
				info.Replieswanted = c.Count
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		res := db.Raw("SELECT COUNT(DISTINCT(messages_reneged.msgid)) AS reneged FROM messages_reneged WHERE userid = ? AND timestamp > ?", id, start)
		var info2 UserInfo
		res.Scan(&info2)
		mu.Lock()
		defer mu.Unlock()
		info.Reneged = info2.Reneged
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		res := db.Raw("SELECT COUNT(DISTINCT messages_by.msgid) AS collected FROM messages_by "+
			"INNER JOIN messages ON messages.id = messages_by.msgid "+
			"INNER JOIN chat_messages ON chat_messages.refmsgid = messages.id AND messages.type = ? AND chat_messages.type = ? "+
			"INNER JOIN messages_groups ON messages_groups.msgid = messages.id WHERE chat_messages.userid = ? AND messages_by.userid = ? AND messages_by.userid != messages.fromuser AND messages_groups.arrival >= ?",
			utils.OFFER,
			utils.CHAT_MESSAGE_INTERESTED,
			id,
			id,
			start)
		var info2 UserInfo
		res.Scan(&info2)
		mu.Lock()
		defer mu.Unlock()
		info.Collected = info2.Collected
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		rows, _ := db.Raw("SELECT COUNT(*) AS count, messages.type, messages_outcomes.outcome FROM messages "+
			"INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
			"LEFT JOIN messages_outcomes ON messages_outcomes.msgid = messages.id "+
			"WHERE fromuser = ? AND messages.arrival > ? AND collection = ? AND messages_groups.deleted = 0 "+
			"GROUP BY messages.type, messages_outcomes.outcome",
			id,
			start,
			utils.COLLECTION_APPROVED).Rows()

		if rows != nil {
			defer rows.Close()

			for rows.Next() {
				type countRow struct {
					Count   uint64
					Type    string
					Outcome string
				}

				var cr countRow

				db.ScanRows(rows, &cr)

				mu.Lock()

				switch cr.Type {
				case utils.OFFER:
					info.Offers += cr.Count

					if len(cr.Outcome) == 0 {
						info.Openoffers += cr.Count
					}
				case utils.WANTED:
					info.Wanteds += cr.Count

					if len(cr.Outcome) == 0 {
						info.Openwanteds += cr.Count
					}
				}

				mu.Unlock()
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// No need to check on the chat room type as we can only get messages of type Interested in a User2User chat.
		res := db.Raw("SELECT replytime FROM users_replytime WHERE userid = ?", id)
		var info2 UserInfo
		res.Scan(&info2)
		mu.Lock()
		defer mu.Unlock()
		info.Replytime = info2.Replytime
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// No need to check on the chat room type as we can only get messages of type Interested in a User2User chat.
		start := time.Now().AddDate(0, 0, -utils.CHAT_ACTIVE_LIMIT).Format("2006-01-02")

		res := db.Raw("SELECT COUNT(*) AS expectedreply FROM users_expected "+
			"INNER JOIN users ON users.id = users_expected.expectee "+
			"INNER JOIN chat_messages ON chat_messages.id = users_expected.chatmsgid "+
			"WHERE expectee = ? AND chat_messages.date >= ? AND replyexpected = 1 AND "+
			"replyreceived = 0 AND TIMESTAMPDIFF(MINUTE, chat_messages.date, users.lastaccess) >= ?", id, start, utils.CHAT_REPLY_GRACE)
		var info2 UserInfo
		res.Scan(&info2)
		mu.Lock()
		defer mu.Unlock()
		info.Expectedreply = info2.Expectedreply
		info.Expectedreplies = info2.Expectedreply
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		// We show visible ratings, ones we have made ourselves, or those from TN.
		type Count struct {
			Count  uint64
			Rating string
		}

		var counts []Count

		start := time.Now().AddDate(0, 0, -utils.RATINGS_PERIOD).Format("2006-01-02")
		res := db.Raw("SELECT COUNT(*) AS count, rating FROM ratings WHERE ratee = ?"+
			" AND timestamp >= ? AND (tn_rating_id IS NOT NULL OR rater = ? OR visible = 1) GROUP BY rating;", id, start, myid)
		res.Scan(&counts)

		mu.Lock()
		defer mu.Unlock()

		for _, count := range counts {
			if count.Rating == utils.RATING_UP {
				info.Ratings.Up = count.Count
			} else if count.Rating == utils.RATING_DOWN {
				info.Ratings.Down = count.Count
			}
		}
	}()

	if myid > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// We show visible ratings, ones we have made ourselves, or those from TN.
			type Count struct {
				Count  uint64
				Rating string
			}

			var counts []Count

			start := time.Now().AddDate(0, 0, -utils.RATINGS_PERIOD).Format("2006-01-02")
			res := db.Raw("SELECT rating FROM ratings WHERE rater = ? AND ratee = ?"+
				" AND timestamp >= ?", myid, id, start)
			res.Scan(&counts)

			mu.Lock()
			defer mu.Unlock()

			for _, count := range counts {
				info.Ratings.Mine = count.Rating
			}
		}()
	}

	wg.Wait()

	return info
}

// GetPublicLocationForUser returns the public location for a user, derived from their
// lastlocation or most recent group membership.
func GetPublicLocationForUser(userid uint64) *Publiclocation {
	db := database.DBConn

	// Try lastlocation first — just get the location name.
	var locName string
	db.Raw("SELECT l.name "+
		"FROM users u "+
		"INNER JOIN locations l ON l.id = u.lastlocation "+
		"WHERE u.id = ? AND u.lastlocation IS NOT NULL "+
		"LIMIT 1", userid).Scan(&locName)

	if locName != "" {
		return &Publiclocation{
			Display:  locName,
			Location: locName,
		}
	}

	// Fall back to most recent group membership.
	var groupLoc struct {
		Groupid   uint64
		Groupname string
	}
	db.Raw("SELECT m.groupid, COALESCE(g.namefull, g.nameshort) AS groupname "+
		"FROM memberships m "+
		"INNER JOIN `groups` g ON g.id = m.groupid "+
		"WHERE m.userid = ? AND m.collection = 'Approved' "+
		"ORDER BY m.added DESC LIMIT 1", userid).Scan(&groupLoc)

	if groupLoc.Groupid > 0 {
		return &Publiclocation{
			Display:   groupLoc.Groupname,
			Location:  groupLoc.Groupname,
			Groupid:   groupLoc.Groupid,
			Groupname: groupLoc.Groupname,
		}
	}

	return nil
}
