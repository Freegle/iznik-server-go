package group

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
)

type GroupVolunteer struct {
	ID           uint64           `json:"id" gorm:"primary_key"`
	Userid       uint64           `json:"userid"`
	Firstname    string           `json:"firstname"`
	Lastname     string           `json:"lastname"`
	Fullname     string           `json:"fullname"`
	Displayname  string           `json:"displayname"`
	Profileid    uint64           `json:"-"`
	Url          string           `json:"-"`
	Archived     int              `json:"-"`
	Profile      user.UserProfile `json:"profile" gorm:"-"`
	Showmod      bool             `json:"-"`
	Externaluid  string           `json:"externaluid"`
	Externalmods json.RawMessage  `json:"externalmods"`
}

func GetGroupVolunteers(id uint64) []GroupVolunteer {
	var ret []GroupVolunteer
	var all []GroupVolunteer

	db := database.DBConn

	// Get most recent profile.
	//
	// showmod setting defaults true.
	db.Raw("SELECT memberships.userid AS id, ui.id AS profileid, ui.url AS url, ui.archived, ui.externaluid, ui.externalmods, "+
		"CASE WHEN users.fullname IS NOT NULL THEN users.fullname ELSE CONCAT(users.firstname, ' ', users.lastname) END AS displayname, "+
		"CASE WHEN JSON_EXTRACT(users.settings, '$.showmod') IS NULL THEN 1 ELSE JSON_EXTRACT(users.settings, '$.showmod') END AS showmod "+
		"FROM memberships "+
		"LEFT JOIN users_images ui ON ui.id = ("+
		"	SELECT MAX(ui2.id) minid FROM users_images ui2 WHERE ui2.userid = memberships.userid "+
		")  "+
		"INNER JOIN users ON users.id = memberships.userid WHERE groupid = ? AND role IN (?, ?)", id, MODERATOR, OWNER).Scan(&all)

	for ix, r := range all {
		if r.Showmod {
			thisone := all[ix]
			user.ProfileSetPath(r.Profileid, r.Url, r.Externaluid, r.Externalmods, r.Archived, &thisone.Profile)
			ret = append(ret, thisone)
		}
	}

	return ret
}
