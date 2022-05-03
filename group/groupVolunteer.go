package group

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
)

func (GroupVolunteer) TableName() string {
	return "memberships"
}

type GroupVolunteer struct {
	ID          uint64           `json:"id" gorm:"primary_key"`
	Userid      uint64           `json:"userid"`
	Firstname   string           `json:"firstname"`
	Lastname    string           `json:"lastname"`
	Fullname    string           `json:"fullname"`
	Displayname string           `json:"displayname"`
	Profileid   uint64           `json:"-"`
	Url         string           `json:"-"`
	Archived    int              `json:"-"`
	Profile     user.UserProfile `json:"profile"`
}

func GetGroupVolunteers(id uint64) []GroupVolunteer {
	var ret []GroupVolunteer

	db := database.DBConn

	// Get most recent profile.
	// TODO showmod setting
	db.Raw("SELECT memberships.userid AS id, ui.id AS profileid, ui.url AS url, ui.archived, "+
		"CASE WHEN users.fullname IS NOT NULL THEN users.fullname ELSE CONCAT(users.firstname, ' ', users.lastname) END AS displayname "+
		"FROM memberships "+
		"LEFT JOIN users_images ui ON ui.id = ("+
		"	SELECT MAX(ui2.id) minid FROM users_images ui2 WHERE ui2.userid = memberships.userid "+
		")  "+
		"INNER JOIN users ON users.id = memberships.userid WHERE groupid = ? AND role IN (?, ?)", id, MODERATOR, OWNER).Scan(&ret)

	for ix, r := range ret {
		user.ProfileSetPath(r.Profileid, r.Url, r.Archived, &ret[ix].Profile)
	}

	return ret
}
