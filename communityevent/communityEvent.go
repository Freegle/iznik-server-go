package communityevent

import (
	"errors"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"os"
	"strconv"
	"sync"
	"time"
)

func (CommunityEvent) TableName() string {
	return "communityevents"
}

type CommunityEvent struct {
	ID             uint64               `json:"id" gorm:"primary_key"`
	Userid         uint64               `json:"userid"`
	Title          string               `json:"title"`
	Location       string               `json:"location"`
	Contactname    string               `json:"contactname"`
	Contactphone   string               `json:"contactphone"`
	Contactemail   string               `json:"contactemail"`
	Contacturl     string               `json:"contacturl"`
	Description    string               `json:"description"`
	Timecommitment string               `json:"timecommitment"`
	Added          time.Time            `json:"added"`
	Groups         []uint64             `json:"groups" gorm:"-"`
	Image          *CommunityEventImage `json:"image" gorm:"-"`
	Dates          []CommunityEventDate `json:"dates" gorm:"-"`
}

func List(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	memberships := user.GetMemberships(myid)
	var groupids []uint64

	for _, membership := range memberships {
		groupids = append(groupids, membership.Groupid)
	}

	var ids []uint64

	start := time.Now().Format("2006-01-02")

	db.Raw("SELECT DISTINCT communityevents.id FROM communityevents "+
		"LEFT JOIN communityevents_groups ON communityevents.id = communityevents_groups.eventid "+
		"LEFT JOIN communityevents_dates ON communityevents.id = communityevents_dates.eventid "+
		"LEFT JOIN users ON communityevents.userid = users.id "+
		"WHERE (groupid IS NULL OR groupid IN (?)) AND "+
		"end IS NOT NULL AND end >= ? AND communityevents.deleted = 0 AND (pending = 0 OR communityevents.userid = ?) "+
		"AND users.deleted IS NULL "+
		"ORDER BY end ASC", groupids, start, myid).Pluck("eventid", &ids)

	if len(ids) > 0 {
		return c.JSON(ids)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}

func ListGroup(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid groupid")
	}

	db := database.DBConn

	var ids []uint64

	start := time.Now().Format("2006-01-02")

	db.Raw("SELECT DISTINCT communityevents.id FROM communityevents "+
		"LEFT JOIN communityevents_groups ON communityevents.id = communityevents_groups.eventid "+
		"LEFT JOIN communityevents_dates ON communityevents.id = communityevents_dates.eventid "+
		"LEFT JOIN users ON communityevents.userid = users.id "+
		"WHERE groupid = ? AND "+
		"end IS NOT NULL AND end >= ? AND communityevents.deleted = 0 AND pending = 0 "+
		"AND users.deleted IS NULL "+
		"ORDER BY end ASC", id, start).Pluck("eventid", &ids)

	if len(ids) > 0 {
		return c.JSON(ids)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}

func Single(c *fiber.Ctx) error {
	var wg sync.WaitGroup
	var communityevent CommunityEvent
	var image CommunityEventImage
	var found bool
	var groups []uint64
	var dates []CommunityEventDate
	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	imageDomain := os.Getenv("IMAGE_DOMAIN")

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err == nil {

		db := database.DBConn

		wg.Add(1)

		go func() {
			defer wg.Done()

			// Can always fetch a single one if we know the id, even if it's pending.
			err := db.Where("id = ? AND deleted = 0 AND heldby IS NULL", id).First(&communityevent).Error
			found = !errors.Is(err, gorm.ErrRecordNotFound)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT id, archived, externaluid, externalmods FROM communityevents_images WHERE eventid = ? ORDER BY id DESC LIMIT 1", id).Scan(&image)

			if image.ID > 0 {
				if image.Externaluid != "" {
					image.Externalmods = image.Externalmods
					image.Ouruid = image.Externaluid
					image.Path = misc.GetImageDeliveryUrl(image.Externaluid, string(image.Externalmods))
					image.Paththumb = misc.GetImageDeliveryUrl(image.Externaluid, string(image.Externalmods))
					image.Externaluid = ""
				} else if image.Archived > 0 {
					image.Path = "https://" + archiveDomain + "/cimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + archiveDomain + "/tcimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				} else {
					image.Path = "https://" + imageDomain + "/cimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + imageDomain + "/tcimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				}
			}
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT groupid FROM communityevents_groups WHERE eventid = ?", id).Pluck("groupid", &groups)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT * FROM communityevents_dates WHERE eventid = ?", id).Scan(&dates)
		}()

		wg.Wait()

		if found {
			if image.ID > 0 {
				communityevent.Image = &image
			}

			communityevent.Groups = groups
			communityevent.Dates = dates

			return c.JSON(communityevent)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Not found")
}

func canModify(myid uint64, eventID uint64) bool {
	db := database.DBConn

	var ownerID uint64
	db.Raw("SELECT userid FROM communityevents WHERE id = ?", eventID).Scan(&ownerID)

	if ownerID == myid {
		return true
	}

	// Check if user is admin/support
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	// Check if user is moderator/owner of any linked group
	var groupIDs []uint64
	db.Raw("SELECT groupid FROM communityevents_groups WHERE eventid = ?", eventID).Pluck("groupid", &groupIDs)

	for _, gid := range groupIDs {
		var role string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'", myid, gid).Scan(&role)

		if role == "Moderator" || role == "Owner" {
			return true
		}
	}

	return false
}

func isModerator(myid uint64, eventID uint64) bool {
	db := database.DBConn

	// Check if user is admin/support
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	// Check if user is moderator/owner of any linked group
	var groupIDs []uint64
	db.Raw("SELECT groupid FROM communityevents_groups WHERE eventid = ?", eventID).Pluck("groupid", &groupIDs)

	for _, gid := range groupIDs {
		var role string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'", myid, gid).Scan(&role)

		if role == "Moderator" || role == "Owner" {
			return true
		}
	}

	return false
}

type CreateRequest struct {
	Title        string `json:"title"`
	Location     string `json:"location"`
	Contactname  string `json:"contactname"`
	Contactphone string `json:"contactphone"`
	Contactemail string `json:"contactemail"`
	Contacturl   string `json:"contacturl"`
	Description  string `json:"description"`
	GroupID      uint64 `json:"groupid"`
}

func Create(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Title == "" || req.Location == "" || req.Description == "" {
		return fiber.NewError(fiber.StatusBadRequest, "title, location and description are required")
	}

	db := database.DBConn

	result := db.Exec("INSERT INTO communityevents (userid, pending, title, location, contactname, contactphone, contactemail, contacturl, description) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?)",
		myid, req.Title, req.Location, req.Contactname, req.Contactphone, req.Contactemail, req.Contacturl, req.Description)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create community event")
	}

	var id uint64
	db.Raw("SELECT id FROM communityevents WHERE userid = ? ORDER BY id DESC LIMIT 1", myid).Scan(&id)

	if id > 0 && req.GroupID > 0 {
		db.Exec("INSERT IGNORE INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", id, req.GroupID)
	}

	return c.JSON(fiber.Map{"id": id})
}

type PatchRequest struct {
	ID           uint64  `json:"id"`
	Action       string  `json:"action"`
	Title        *string `json:"title,omitempty"`
	Location     *string `json:"location,omitempty"`
	Pending      *int    `json:"pending,omitempty"`
	Contactname  *string `json:"contactname,omitempty"`
	Contactphone *string `json:"contactphone,omitempty"`
	Contactemail *string `json:"contactemail,omitempty"`
	Contacturl   *string `json:"contacturl,omitempty"`
	Description  *string `json:"description,omitempty"`
	GroupID      uint64  `json:"groupid"`
	DateID       uint64  `json:"dateid"`
	PhotoID      uint64  `json:"photoid"`
	Start        string  `json:"start"`
	End          string  `json:"end"`
}

func Update(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn
	var exists uint64
	db.Raw("SELECT id FROM communityevents WHERE id = ?", req.ID).Scan(&exists)
	if exists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Community event not found")
	}

	if !canModify(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized to modify this community event")
	}

	// Update settable attributes
	if req.Title != nil {
		db.Exec("UPDATE communityevents SET title = ? WHERE id = ?", *req.Title, req.ID)
	}
	if req.Location != nil {
		db.Exec("UPDATE communityevents SET location = ? WHERE id = ?", *req.Location, req.ID)
	}
	if req.Pending != nil {
		db.Exec("UPDATE communityevents SET pending = ? WHERE id = ?", *req.Pending, req.ID)
	}
	if req.Contactname != nil {
		db.Exec("UPDATE communityevents SET contactname = ? WHERE id = ?", *req.Contactname, req.ID)
	}
	if req.Contactphone != nil {
		db.Exec("UPDATE communityevents SET contactphone = ? WHERE id = ?", *req.Contactphone, req.ID)
	}
	if req.Contactemail != nil {
		db.Exec("UPDATE communityevents SET contactemail = ? WHERE id = ?", *req.Contactemail, req.ID)
	}
	if req.Contacturl != nil {
		db.Exec("UPDATE communityevents SET contacturl = ? WHERE id = ?", *req.Contacturl, req.ID)
	}
	if req.Description != nil {
		db.Exec("UPDATE communityevents SET description = ? WHERE id = ?", *req.Description, req.ID)
	}

	// Process action
	switch req.Action {
	case "AddGroup":
		if req.GroupID > 0 {
			db.Exec("INSERT IGNORE INTO communityevents_groups (eventid, groupid) VALUES (?, ?)", req.ID, req.GroupID)
		}
	case "RemoveGroup":
		if req.GroupID > 0 {
			db.Exec("DELETE FROM communityevents_groups WHERE eventid = ? AND groupid = ?", req.ID, req.GroupID)
		}
	case "AddDate":
		db.Exec("INSERT INTO communityevents_dates (eventid, start, end) VALUES (?, ?, ?)",
			req.ID, nilIfEmpty(req.Start), nilIfEmpty(req.End))
	case "RemoveDate":
		if req.DateID > 0 {
			db.Exec("DELETE FROM communityevents_dates WHERE id = ?", req.DateID)
		}
	case "SetPhoto":
		if req.PhotoID > 0 {
			db.Exec("UPDATE communityevents_images SET eventid = ? WHERE id = ?", req.ID, req.PhotoID)
		}
	case "Hold":
		if isModerator(myid, req.ID) {
			db.Exec("UPDATE communityevents SET heldby = ? WHERE id = ?", myid, req.ID)
		}
	case "Release":
		if isModerator(myid, req.ID) {
			db.Exec("UPDATE communityevents SET heldby = NULL WHERE id = ?", req.ID)
		}
	}

	return c.JSON(fiber.Map{"success": true})
}

func Delete(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
	}

	db := database.DBConn
	var exists uint64
	db.Raw("SELECT id FROM communityevents WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Community event not found")
	}

	if !canModify(myid, id) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized to delete this community event")
	}

	// Soft delete - matches PHP behavior
	db.Exec("UPDATE communityevents SET deleted = 1 WHERE id = ?", id)

	return c.JSON(fiber.Map{"success": true})
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
