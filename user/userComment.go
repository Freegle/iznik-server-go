package user

import (
	"time"

	"github.com/freegle/iznik-server-go/database"
)

type CommentByUser struct {
	ID          uint64 `json:"id"`
	Displayname string `json:"displayname"`
}

type Comment struct {
	ID       uint64         `json:"id"`
	Userid   uint64         `json:"userid"`
	Groupid  *uint64        `json:"groupid"`
	Byuserid *uint64        `json:"byuserid"`
	Date     *time.Time     `json:"date"`
	Reviewed *time.Time     `json:"reviewed"`
	User1    *string        `json:"user1"`
	User2    *string        `json:"user2"`
	User3    *string        `json:"user3"`
	User4    *string        `json:"user4"`
	User5    *string        `json:"user5"`
	User6    *string        `json:"user6"`
	User7    *string        `json:"user7"`
	User8    *string        `json:"user8"`
	User9    *string        `json:"user9"`
	User10   *string        `json:"user10"`
	User11   *string        `json:"user11"`
	Flag     bool           `json:"flag"`
	Byuser   *CommentByUser `json:"byuser" gorm:"-"`
}

// GetComments returns comments for a list of user IDs.
// Only returns results if the requesting user is a moderator.
func GetComments(userids []uint64, myid uint64) map[uint64][]Comment {
	result := make(map[uint64][]Comment)

	if len(userids) == 0 || myid == 0 {
		return result
	}

	// Check if requesting user is a moderator.
	db := database.DBConn
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole != "Moderator" && systemrole != "Support" && systemrole != "Admin" {
		return result
	}

	var comments []Comment
	db.Raw("SELECT * FROM users_comments WHERE userid IN ? ORDER BY date DESC", userids).Scan(&comments)

	if len(comments) == 0 {
		return result
	}

	// Collect byuserids for batch fetch of display names.
	byuserids := make(map[uint64]bool)
	for _, c := range comments {
		if c.Byuserid != nil {
			byuserids[*c.Byuserid] = true
		}
	}

	byusers := make(map[uint64]CommentByUser)
	if len(byuserids) > 0 {
		ids := make([]uint64, 0, len(byuserids))
		for id := range byuserids {
			ids = append(ids, id)
		}

		type nameRow struct {
			ID       uint64 `json:"id"`
			Fullname string `json:"fullname"`
		}
		var names []nameRow
		db.Raw("SELECT id, fullname FROM users WHERE id IN ?", ids).Scan(&names)

		for _, n := range names {
			byusers[n.ID] = CommentByUser{
				ID:          n.ID,
				Displayname: n.Fullname,
			}
		}
	}

	// Assign byuser and group by userid.
	for i := range comments {
		if comments[i].Byuserid != nil {
			if bu, ok := byusers[*comments[i].Byuserid]; ok {
				comments[i].Byuser = &bu
			}
		}
		result[comments[i].Userid] = append(result[comments[i].Userid], comments[i])
	}

	return result
}
