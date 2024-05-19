package notification

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"time"
)

type Notification struct {
	ID             int64     `json:"id"`
	Fromuser       int64     `json:"fromuser"`
	Touser         int64     `json:"touser"`
	Timestamp      time.Time `json:"timestamp"`
	Type           string    `json:"type"`
	Newsfeedid     int64     `json:"newsfeedid"`
	Eventid        int64     `json:"eventid"`
	Volunteeringid int64     `json:"volunteeringid"`
	Url            string    `json:"url"`
	Seen           bool      `json:"seen"`
	Mailed         bool      `json:"mailed"`
	Title          string    `json:"title"`
	Text           string    `json:"text"`
}

func Count(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	start := time.Now().AddDate(0, 0, -utils.NOTIFICATION_AGE).Format("2006-01-02")

	var count []int64
	db.Raw("SELECT COUNT(*) AS count FROM users_notifications "+
		"LEFT JOIN spam_users ON spam_users.userid = users_notifications.fromuser AND collection IN ('PendingAdd', 'Spammer') "+
		"WHERE touser = ? AND timestamp >= ? AND seen = 0 AND spam_users.id IS NULL;", myid, start).Pluck("count", &count)

	if len(count) > 0 {
		return c.JSON(fiber.Map{
			"count": count[0],
		})
	} else {
		return c.JSON(fiber.Map{
			"count": 0,
		})
	}
}

func List(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	start := time.Now().AddDate(0, 0, -utils.NOTIFICATION_AGE).Format("2006-01-02")

	var notifications []Notification
	db.Raw("SELECT * FROM users_notifications "+
		"LEFT JOIN spam_users ON spam_users.userid = users_notifications.fromuser AND collection IN ('PendingAdd', 'Spammer') "+
		"WHERE touser = ? AND timestamp >= ? AND spam_users.id IS NULL ORDER BY users_notifications.id DESC", myid, start).Scan(&notifications)

	return c.JSON(notifications)
}
