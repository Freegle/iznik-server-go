package isochrone

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"time"
)

type Isochrones struct {
	ID          uint64    `json:"id" gorm:"primary_key"`
	Userid      uint64    `json:"userid"`
	Isochroneid uint64    `json:"isochroneid"`
	Locationid  uint64    `json:"locationid"`
	Transport   string    `json:"transport"`
	Minutes     int       `json:"minutes"`
	Timestamp   time.Time `json:"timestamp"`
	Nickname    string    `json:"nickname"`
	Polygon     string    `json:"polygon"`
}

func ListIsochrones(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	isochrones := []Isochrones{}

	db.Raw("SELECT isochrones_users.id, isochroneid, userid, timestamp, nickname, locationid, transport, minutes, ST_AsText(polygon) AS polygon FROM isochrones_users INNER JOIN isochrones ON isochrones_users.isochroneid = isochrones.id WHERE isochrones_users.userid = ?", myid).Scan(&isochrones)
	return c.JSON(isochrones)
}
