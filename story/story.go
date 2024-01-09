package story

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"os"
	"strconv"
	"time"
)

type StoryImage struct {
	ID        uint64 `json:"id"`
	Path      string `json:"path"`
	PathThumb string `json:"paththumb"`
}

type Story struct {
	ID            uint64      `json:"id" gorm:"primary_key"`
	Userid        uint64      `json:"userid"`
	Date          *time.Time  `json:"date"`
	Headline      string      `json:"headline"`
	Story         string      `json:"story"`
	Imageid       uint64      `json:"imageid"`
	Imagearchived bool        `json:"-"`
	Image         *StoryImage `json:"image" gorm:"-"`
	StoryURL      string      `json:"url"`
}

func Single(c *fiber.Ctx) error {
	var s Story

	db := database.DBConn
	db.Raw("SELECT users_stories.*, users_stories_images.id AS imageid, users_stories_images.archived AS imagearchived FROM users_stories "+
		"LEFT JOIN users_stories_images ON users_stories_images.storyid = users_stories.id "+
		"WHERE users_stories.id = ? AND public = 1", c.Params("id")).Scan(&s)

	if s.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Not found")
	}

	if s.Imageid > 0 {
		if s.Imagearchived {
			s.Image = &StoryImage{
				ID:        s.Imageid,
				Path:      "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/simg_" + strconv.FormatUint(s.Imageid, 10) + ".jpg",
				PathThumb: "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tsimg_" + strconv.FormatUint(s.Imageid, 10) + ".jpg",
			}
		} else {
			s.Image = &StoryImage{
				ID:        s.Imageid,
				Path:      "https://" + os.Getenv("IMAGE_DOMAIN") + "/simg_" + strconv.FormatUint(s.Imageid, 10) + ".jpg",
				PathThumb: "https://" + os.Getenv("IMAGE_DOMAIN") + "/tsimg_" + strconv.FormatUint(s.Imageid, 10) + ".jpg",
			}
		}
	}

	s.StoryURL = "https://" + os.Getenv("USER_SITE") + "/story/" + strconv.FormatUint(s.ID, 10)

	return c.JSON(s)
}

func List(c *fiber.Ctx) error {
	db := database.DBConn

	limit := c.Query("limit", "100")
	limit64, _ := strconv.ParseUint(limit, 10, 64)

	var ids []uint64

	db.Raw("SELECT users_stories.id FROM users_stories "+
		"INNER JOIN users ON users.id = users_stories.userid "+
		"WHERE reviewed = 1 AND public = 1 AND userid IS NOT NULL AND users.deleted IS NULL ORDER BY date DESC LIMIT ?;", limit64).Pluck("id", &ids)

	return c.JSON(ids)
}

func Group(c *fiber.Ctx) error {
	db := database.DBConn

	limit := c.Query("limit", "100")
	limit64, _ := strconv.ParseUint(limit, 10, 64)
	groupid := c.Params("id", "0")
	groupid64, _ := strconv.ParseUint(groupid, 10, 64)

	var ids []uint64

	db.Raw("SELECT DISTINCT users_stories.id FROM users_stories "+
		"INNER JOIN memberships ON memberships.userid = users_stories.userid "+
		"INNER JOIN users ON users.id = users_stories.userid "+
		"WHERE memberships.groupid = ? "+
		"AND reviewed = 1 "+
		"AND public = 1 "+
		"AND users_stories.userid IS NOT NULL "+
		"AND users.deleted IS NULL "+
		"ORDER BY date DESC LIMIT ?;", groupid64, limit64).Pluck("id", &ids)

	return c.JSON(ids)
}
