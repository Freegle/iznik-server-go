package user

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"strconv"
)

type User struct {
	ID          uint64      `json:"id" gorm:"primary_key"`
	Firstname   string      `json:"firstname"`
	Lastname    string      `json:"lastname"`
	Fullname    string      `json:"fullname"`
	Displayname string      `json:"displayname"`
	Profile     UserProfile `json:"profile"`
	Info        UserInfo    `json:"info"`
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

// This provides enough information about a message to display a summary ont he browse page.
func GetUser(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err == nil {
		user := GetUserById(id)

		if user.ID == id {
			return c.JSON(user)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "User not found")
}

func GetUserById(id uint64) User {
	db := database.DBConn

	var user User

	// TODO Tidyups in user getName()

	if !db.Where("id = ?", id).Find(&user).RecordNotFound() {
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
