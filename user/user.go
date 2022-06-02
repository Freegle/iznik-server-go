package user

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"strconv"
)

type User struct {
	ID          uint64       `json:"id" gorm:"primary_key"`
	Firstname   string       `json:"firstname"`
	Lastname    string       `json:"lastname"`
	Fullname    string       `json:"fullname"`
	Displayname string       `json:"displayname"`
	Profile     UserProfile  `json:"profile"`
	Info        UserInfo     `json:"info"`
	Memberships []Membership `json:"memberships"` // Only returned for logged-in user.
}

type Tabler interface {
	TableName() string
}

func (UserProfileRecord) TableName() string {
	return "users_images"
}

type UserProfileRecord struct {
	ID        uint64 `json:"id" gorm:"primary_key"`
	Profileid uint64
	Url       string
	Archived  int
}

type Membership struct {
	ID                  uint64 `json:"id" gorm:"primary_key"`
	Groupid             uint64 `json:"groupid"`
	Emailfrequency      uint64 `json:"emailfrequency"`
	Eventsallowed       uint64 `json:"eventsallowed"`
	Volunteeringallowed uint64 `json:"volunteeringallowed"`
	Nameshort           string `json:"nameshort"`
	Namefull            string `json:"namefull"`
	Namedisplay         string `json:"namedisplay"`
}

func GetUser(c *fiber.Ctx) error {
	if c.Params("id") != "" {
		// Looking for a specific user.
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			user := GetUserById(id)

			if user.ID == id {
				return c.JSON(user)
			}
		}
	} else {
		// Looking for the currently logged-in user as authenticated by the Authorization header JWT (if present).
		id := WhoAmI(c)

		if id > 0 {
			user := GetUserById(id)

			if user.ID == id {
				// Get the groups too.
				var memberships []Membership

				db := database.DBConn
				db.Raw("SELECT memberships.id, groupid, nameshort, namefull, emailfrequency, eventsallowed, volunteeringallowed FROM memberships INNER JOIN `groups` ON groups.id = memberships.groupid WHERE userid = ? AND collection = ?", id, "Approved").Scan(&memberships)

				for _, r := range memberships {
					if len(r.Namefull) > 0 {
						r.Namedisplay = r.Namefull
					} else {
						r.Namedisplay = r.Nameshort
					}
				}

				user.Memberships = memberships

				return c.JSON(user)
			}
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "User not found")
}

func GetUserById(id uint64) User {
	db := database.DBConn

	var user User

	// This provides enough information about a message to display a summary on the browse page.
	if !db.Where("id = ?", id).Find(&user).RecordNotFound() {
		// TODO Tidyups in user getName()
		if len(user.Fullname) > 0 {
			user.Displayname = user.Fullname
		} else {
			user.Displayname = user.Firstname + " " + user.Lastname
		}
	}

	var profileRecord UserProfileRecord

	db.Raw("SELECT ui.id AS profileid, ui.url AS url, ui.archived "+
		" FROM users_images ui WHERE userid = ? ORDER BY id DESC LIMIT 1", id).Scan(&profileRecord)

	ProfileSetPath(profileRecord.Profileid, profileRecord.Url, profileRecord.Archived, &user.Profile)

	return user
}
