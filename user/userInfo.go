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

type UserInfo struct {
	Replies       uint64  `json:"replies"`
	Taken         uint64  `json:"taken"`
	Reneged       uint64  `json:"reneged"`
	Collected     uint64  `json:"collected"`
	Offers        uint64  `json:"offers"`
	Wanteds       uint64  `json:"wanteds"`
	Openoffers    uint64  `json:"openoffers"`
	Openwanteds   uint64  `json:"openwanteds"`
	Expectedreply uint64  `json:"expectedreply"`
	Openage       uint64  `json:"openage"`
	Replytime     uint64  `json:"replytime"`
	Ratings       Ratings `json:"ratings"`
}

func GetUserUinfo(id uint64, myid uint64) UserInfo {
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
		// No need to check on the chat room type as we can only get messages of type Interested in a User2User chat.
		res := db.Raw("SELECT COUNT(DISTINCT refmsgid) AS replies FROM chat_messages WHERE userid = ? AND date > ? AND refmsgid IS NOT NULL AND type = ?", id, start, utils.CHAT_MESSAGE_INTERESTED)
		mu.Lock()
		defer mu.Unlock()
		res.Scan(&info)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		res := db.Raw("SELECT COUNT(DISTINCT(messages_reneged.msgid)) AS reneged FROM messages_reneged WHERE userid = ? AND timestamp > ?", id, start)
		mu.Lock()
		defer mu.Unlock()
		res.Scan(&info)
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
		mu.Lock()
		defer mu.Unlock()
		res.Scan(&info)
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
		mu.Lock()
		defer mu.Unlock()
		res.Scan(&info)
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
		mu.Lock()
		defer mu.Unlock()
		res.Scan(&info)
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
			" AND timestamp >= ?"+
			" AND (tn_rating_id IS NOT NULL OR rater = ? OR visible = 1) GROUP BY rating;", id, start, id)
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
