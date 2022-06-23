package user

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"sync"
	"time"
)

type UserInfo struct {
	Replies        uint64 `json: "replies"`
	Taken          uint64 `json: "taken"`
	Reneged        uint64 `json: "reneged"`
	Collected      uint64 `json: "collected"`
	Offers         uint64 `json: "offers"`
	Wanteds        uint64 `json: "wanteds"`
	Openoffers     uint64 `json: "openoffers"`
	Openwanteds    uint64 `json: "openwanteds"`
	Explectedreply uint64 `json: "expectedreply"`
	Openage        uint64 `json: "openage"`
}

func GetUserUinfo(id uint64) UserInfo {
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
		res := db.Raw("SELECT COUNT(DISTINCT(messages_reneged.msgid)) AS reneged FROM messages_reneged WHERE userid = ? AND timestamp > ?", id, start).Scan(&info)
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

	// TODO About me, replytime, nudges, ratings, expected replies
	wg.Wait()

	return info
}
