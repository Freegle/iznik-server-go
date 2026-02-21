package story

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

type StoryImage struct {
	ID           uint64          `json:"id"`
	Externaluid  string          `json:"externaluid"`
	Ouruid       string          `json:"ouruid"`
	Externalmods json.RawMessage `json:"externalmods"`
	Path         string          `json:"path"`
	PathThumb    string          `json:"paththumb"`
}

type Story struct {
	ID            uint64          `json:"id" gorm:"primary_key"`
	Userid        uint64          `json:"userid"`
	Date          *time.Time      `json:"date"`
	Public        bool            `json:"public"`
	Reviewed      bool            `json:"reviewed"`
	Headline      string          `json:"headline"`
	Story         string          `json:"story"`
	Imageid       uint64          `json:"imageid"`
	Imagearchived bool            `json:"-"`
	Imageuid      string          `json:"-"`
	Imagemods     json.RawMessage `json:"-"`
	Image         *StoryImage     `json:"image" gorm:"-"`
	StoryURL      string          `json:"url"`
}

func Single(c *fiber.Ctx) error {
	var s Story

	db := database.DBConn
	db.Raw("SELECT users_stories.*, users_stories_images.id AS imageid, users_stories_images.archived AS imagearchived, users_stories_images.externaluid AS imageuid, users_stories_images.externalmods AS imagemods FROM users_stories "+
		"LEFT JOIN users_stories_images ON users_stories_images.storyid = users_stories.id "+
		"WHERE users_stories.id = ?", c.Params("id")).Scan(&s)

	if s.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Not found")
	}

	if s.Imageid > 0 {
		if s.Imageuid != "" {
			s.Image = &StoryImage{
				ID:           s.Imageid,
				Ouruid:       s.Imageuid,
				Externalmods: s.Imagemods,
				Path:         misc.GetImageDeliveryUrl(s.Imageuid, string(s.Imagemods)),
				PathThumb:    misc.GetImageDeliveryUrl(s.Imageuid, string(s.Imagemods)),
			}
		} else if s.Imagearchived {
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

	reviewed := c.Query("reviewed", "1")
	public := c.Query("public", "1")

	var sql string
	var args []interface{}

	if authorityid := c.Query("authorityid"); authorityid != "" {
		// Filter stories by users whose location falls within the authority boundary.
		authorityid64, _ := strconv.ParseUint(authorityid, 10, 64)
		sql = "SELECT DISTINCT users_stories.id FROM users_stories " +
			"INNER JOIN users ON users.id = users_stories.userid " +
			"LEFT JOIN locations ON locations.id = users.lastlocation " +
			"WHERE reviewed = ? AND public = ? AND users_stories.userid IS NOT NULL AND users.deleted IS NULL " +
			"AND locations.lat IS NOT NULL " +
			"AND ST_Contains((SELECT polygon FROM authorities WHERE id = ?), ST_SRID(POINT(locations.lng, locations.lat), ?)) " +
			"ORDER BY date DESC LIMIT ?"
		args = []interface{}{reviewed, public, authorityid64, utils.SRID, limit64}
	} else {
		sql = "SELECT users_stories.id FROM users_stories " +
			"INNER JOIN users ON users.id = users_stories.userid " +
			"WHERE reviewed = ? AND public = ? AND userid IS NOT NULL AND users.deleted IS NULL"
		args = []interface{}{reviewed, public}

		if newsletterreviewed := c.Query("newsletterreviewed"); newsletterreviewed != "" {
			sql += " AND newsletterreviewed = ?"
			args = append(args, newsletterreviewed)
		}

		sql += " ORDER BY date DESC LIMIT ?"
		args = append(args, limit64)
	}

	var ids []uint64
	db.Raw(sql, args...).Pluck("id", &ids)

	return c.JSON(ids)
}

func Group(c *fiber.Ctx) error {
	db := database.DBConn

	limit := c.Query("limit", "100")
	limit64, _ := strconv.ParseUint(limit, 10, 64)
	groupid := c.Params("id", "0")
	groupid64, _ := strconv.ParseUint(groupid, 10, 64)

	reviewed := c.Query("reviewed", "1")
	public := c.Query("public", "1")

	var ids []uint64

	db.Raw("SELECT DISTINCT users_stories.id FROM users_stories "+
		"INNER JOIN memberships ON memberships.userid = users_stories.userid "+
		"INNER JOIN users ON users.id = users_stories.userid "+
		"WHERE memberships.groupid = ? "+
		"AND reviewed = ? "+
		"AND public = ? "+
		"AND users_stories.userid IS NOT NULL "+
		"AND users.deleted IS NULL "+
		"ORDER BY date DESC LIMIT ?;", groupid64, reviewed, public, limit64).Pluck("id", &ids)

	return c.JSON(ids)
}

// canModStory checks if a user can modify a story.
// They can if they're the story owner, admin/support, or a moderator on a group
// the story author is a member of.
func canModStory(myid uint64, storyID uint64) bool {
	db := database.DBConn

	var authorID uint64
	db.Raw("SELECT userid FROM users_stories WHERE id = ?", storyID).Scan(&authorID)

	if authorID == 0 {
		return false
	}

	if authorID == myid {
		return true
	}

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	// Check if moderator/owner on a group the story author is a member of.
	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships m1 "+
		"INNER JOIN memberships m2 ON m2.groupid = m1.groupid "+
		"WHERE m1.userid = ? AND m2.userid = ? "+
		"AND m1.role IN ('Moderator', 'Owner') "+
		"AND m1.collection = 'Approved' AND m2.collection = 'Approved'",
		myid, authorID).Scan(&count)

	return count > 0
}

// createStoryNewsfeedEntry creates a newsfeed entry when a story is reviewed and made public.
func createStoryNewsfeedEntry(userid uint64, storyID uint64) {
	db := database.DBConn

	var lat, lng *float64

	if userid > 0 {
		type UserLoc struct {
			Lat *float64
			Lng *float64
		}
		var ul UserLoc
		db.Raw("SELECT l.lat, l.lng FROM users u LEFT JOIN locations l ON l.id = u.lastlocation WHERE u.id = ?", userid).Scan(&ul)
		lat = ul.Lat
		lng = ul.Lng
	}

	if lat == nil || lng == nil {
		return
	}

	result := db.Exec(
		"INSERT INTO newsfeed (`type`, userid, storyid, position, hidden, deleted, reviewrequired, pinned) "+
			"VALUES ('Story', ?, ?, ST_GeomFromText(CONCAT('POINT(', ?, ' ', ?, ')'), ?), NULL, NULL, 0, 0)",
		userid, storyID, *lng, *lat, utils.SRID,
	)

	if result.Error != nil {
		log.Printf("Failed to create story newsfeed entry: %v", result.Error)
	}
}

// @Summary Create a story
// @Tags story
// @Router /story [put]
func CreateStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type CreateRequest struct {
		Public   bool   `json:"public"`
		Headline string `json:"headline"`
		Story    string `json:"story"`
		Photo    uint64 `json:"photo"`
	}
	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn
	result := db.Exec("INSERT INTO users_stories (public, userid, headline, story) VALUES (?, ?, ?, ?)",
		req.Public, myid, req.Headline, req.Story)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	if req.Photo > 0 && id > 0 {
		db.Exec("UPDATE users_stories_images SET storyid = ? WHERE id = ?", id, req.Photo)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": id})
}

// toBoolInt converts a JSON value (bool or number) to an *int for DB storage.
// Returns nil if the value is nil, meaning the field was not present in the request.
func toBoolInt(v interface{}) *int {
	if v == nil {
		return nil
	}
	var val int
	switch t := v.(type) {
	case bool:
		if t {
			val = 1
		}
	case float64:
		val = int(t)
	default:
		return nil
	}
	return &val
}

// @Summary Update a story (mod review)
// @Tags story
// @Router /story [patch]
func UpdateStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Use interface{} for fields that may be sent as bool (true/false) or int (1/0).
	type UpdateRequest struct {
		ID                 uint64      `json:"id"`
		Public             interface{} `json:"public"`
		Headline           *string     `json:"headline"`
		Story              *string     `json:"story"`
		Reviewed           interface{} `json:"reviewed"`
		Newsletterreviewed interface{} `json:"newsletterreviewed"`
		Newsletter         interface{} `json:"newsletter"`
	}
	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing story ID")
	}

	if !canModStory(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	db := database.DBConn

	// Get current state before update (for newsfeed side effect).
	type StoryState struct {
		Reviewed     bool
		Public       bool
		Userid       uint64
		Fromnewsfeed bool
	}
	var before StoryState
	db.Raw("SELECT reviewed, public, userid, COALESCE(fromnewsfeed, 0) AS fromnewsfeed FROM users_stories WHERE id = ?", req.ID).Scan(&before)

	// Update settable attributes.
	if p := toBoolInt(req.Public); p != nil {
		db.Exec("UPDATE users_stories SET public = ? WHERE id = ?", *p, req.ID)
	}
	if req.Headline != nil {
		db.Exec("UPDATE users_stories SET headline = ? WHERE id = ?", *req.Headline, req.ID)
	}
	if req.Story != nil {
		db.Exec("UPDATE users_stories SET story = ? WHERE id = ?", *req.Story, req.ID)
	}
	if r := toBoolInt(req.Reviewed); r != nil {
		db.Exec("UPDATE users_stories SET reviewed = ?, reviewedby = ? WHERE id = ?", *r, myid, req.ID)
	}
	if nr := toBoolInt(req.Newsletterreviewed); nr != nil {
		db.Exec("UPDATE users_stories SET newsletterreviewed = ?, newsletterreviewedby = ? WHERE id = ?", *nr, myid, req.ID)
	}
	if n := toBoolInt(req.Newsletter); n != nil {
		db.Exec("UPDATE users_stories SET newsletter = ? WHERE id = ?", *n, req.ID)
	}

	// Side effect: if story just became reviewed+public and wasn't from newsfeed, create newsfeed entry.
	newsfeedBefore := before.Reviewed && before.Public

	var after StoryState
	db.Raw("SELECT reviewed, public, userid, COALESCE(fromnewsfeed, 0) AS fromnewsfeed FROM users_stories WHERE id = ?", req.ID).Scan(&after)
	newsfeedAfter := after.Reviewed && after.Public

	if !newsfeedBefore && newsfeedAfter && !before.Fromnewsfeed {
		createStoryNewsfeedEntry(before.Userid, req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// @Summary Like a story
// @Tags story
// @Router /story/like [post]
func LikeStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type LikeRequest struct {
		ID uint64 `json:"id"`
	}
	var req LikeRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing story ID")
	}

	db := database.DBConn
	db.Exec("INSERT IGNORE INTO users_stories_likes (storyid, userid) VALUES (?, ?)", req.ID, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// @Summary Unlike a story
// @Tags story
// @Router /story/unlike [post]
func UnlikeStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type UnlikeRequest struct {
		ID uint64 `json:"id"`
	}
	var req UnlikeRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing story ID")
	}

	db := database.DBConn
	db.Exec("DELETE FROM users_stories_likes WHERE storyid = ? AND userid = ?", req.ID, myid)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// @Summary Post story action (Like/Unlike)
// @Tags story
// @Router /story [post]
func PostStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	type PostRequest struct {
		ID     uint64 `json:"id"`
		Action string `json:"action"`
	}
	var req PostRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Missing story ID")
	}

	db := database.DBConn

	switch req.Action {
	case "Like":
		db.Exec("INSERT IGNORE INTO users_stories_likes (storyid, userid) VALUES (?, ?)", req.ID, myid)
	case "Unlike":
		db.Exec("DELETE FROM users_stories_likes WHERE storyid = ? AND userid = ?", req.ID, myid)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action")
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// @Summary Delete a story
// @Tags story
// @Router /story/{id} [delete]
func DeleteStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
	}

	if !canModStory(myid, id) {
		return fiber.NewError(fiber.StatusForbidden, "Permission denied")
	}

	db := database.DBConn
	db.Exec("DELETE FROM users_stories WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
