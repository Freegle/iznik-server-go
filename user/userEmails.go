package user

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"time"
)

type UserEmail struct {
	ID        uint64     `json:"id" gorm:"primary_key"`
	Added     time.Time  `json:"added"`
	Bounced   *time.Time `json:"bounced"`
	Ourdomain int        `json:"ourdomain"`
	Preferred int        `json:"preferred"`
	Email     string     `json:"email"`
}

func getEmails(id uint64) []UserEmail {
	db := database.DBConn

	var emails []UserEmail

	db.Raw("SELECT id, added, bounced, preferred, email FROM users_emails WHERE userid = ? ORDER BY preferred DESC, email ASC;", id).Scan(&emails)

	for ix, e := range emails {
		emails[ix].Ourdomain = utils.OurDomain(e.Email)
	}

	return emails
}
