package volunteering

import (
	"errors"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/newsfeed"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"html"
	"os"
	"strconv"
	"sync"
	"time"
)

func (Volunteering) TableName() string {
	return "volunteering"
}

type Volunteering struct {
	ID             uint64             `json:"id" gorm:"primary_key"`
	Userid         uint64             `json:"userid"`
	Pending        bool               `json:"pending"`
	Heldby         *uint64            `json:"heldby"`
	Title          string             `json:"title"`
	Location       string             `json:"location"`
	Contactname    string             `json:"contactname"`
	Contactphone   string             `json:"contactphone"`
	Contactemail   string             `json:"contactemail"`
	Contacturl     string             `json:"contacturl"`
	Description    string             `json:"description"`
	Timecommitment string             `json:"timecommitment"`
	Added          time.Time          `json:"added"`
	Groups         []uint64           `json:"groups"  gorm:"-"`
	Image          *VolunteeringImage `json:"image" gorm:"-"`
	Dates          []VolunteeringDate `json:"dates" gorm:"-"`
	Expired        bool               `json:"expired"`
}

func List(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn
	pending := c.Query("pending") == "true"

	memberships := user.GetMemberships(myid)
	var groupids []uint64

	for _, membership := range memberships {
		groupids = append(groupids, membership.Groupid)
	}

	var ids []uint64

	if pending {
		// Return only pending volunteering visible to this moderator/admin.
		var modGroupIDs []uint64
		for _, m := range memberships {
			if m.Role == "Moderator" || m.Role == "Owner" {
				modGroupIDs = append(modGroupIDs, m.Groupid)
			}
		}

		var systemrole string
		db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
		isAdmin := systemrole == "Support" || systemrole == "Admin"

		if isAdmin {
			db.Raw("SELECT DISTINCT volunteering.id FROM volunteering "+
				"WHERE volunteering.deleted = 0 AND pending = 1 "+
				"ORDER BY id DESC").Pluck("id", &ids)
		} else if len(modGroupIDs) > 0 {
			db.Raw("SELECT DISTINCT volunteering.id FROM volunteering "+
				"LEFT JOIN volunteering_groups ON volunteering.id = volunteering_groups.volunteeringid "+
				"WHERE (groupid IN (?) OR groupid IS NULL) AND volunteering.deleted = 0 AND pending = 1 "+
				"ORDER BY id DESC", modGroupIDs).Pluck("id", &ids)
		}
	} else {
		start := time.Now().Format("2006-01-02")

		db.Raw("SELECT DISTINCT volunteering.id FROM volunteering "+
			"LEFT JOIN volunteering_groups ON volunteering.id = volunteering_groups.volunteeringid "+
			"LEFT JOIN volunteering_dates ON volunteering.id = volunteering_dates.volunteeringid "+
			"LEFT JOIN users ON volunteering.userid = users.id "+
			"WHERE (groupid IS NULL OR groupid IN (?)) AND "+
			"(applyby IS NULL OR applyby >= ?) AND (end IS NULL OR end >= ?) AND volunteering.deleted = 0 AND expired = 0 AND (pending = 0 OR volunteering.userid = ?) "+
			"AND users.deleted IS NULL "+
			"ORDER BY id DESC", groupids, start, start, myid).Pluck("volunteeringid", &ids)
	}

	if len(ids) > 0 {
		return c.JSON(ids)
	} else {
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

	db.Raw("SELECT DISTINCT volunteering.id FROM volunteering "+
		"LEFT JOIN volunteering_groups ON volunteering.id = volunteering_groups.volunteeringid "+
		"LEFT JOIN volunteering_dates ON volunteering.id = volunteering_dates.volunteeringid "+
		"LEFT JOIN users ON volunteering.userid = users.id "+
		"WHERE groupid = ? AND "+
		"(applyby IS NULL OR applyby >= ?) AND (end IS NULL OR end >= ?) AND volunteering.deleted = 0 AND expired = 0 AND pending = 0 "+
		"AND users.deleted IS NULL "+
		"ORDER BY id DESC", id, start, start).Pluck("volunteeringid", &ids)

	if len(ids) > 0 {
		return c.JSON(ids)
	} else {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	}
}

func Single(c *fiber.Ctx) error {
	var wg sync.WaitGroup
	var volunteering Volunteering
	var image VolunteeringImage
	var found bool
	var groups []uint64
	var dates []VolunteeringDate
	archiveDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	imageDomain := os.Getenv("IMAGE_DOMAIN")

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err == nil {
		db := database.DBConn

		wg.Add(1)

		go func() {
			defer wg.Done()

			// Can always fetch a single one if we know the id, even if it's pending or held.
			err := db.Where("id = ? AND deleted = 0", id).First(&volunteering).Error
			found = !errors.Is(err, gorm.ErrRecordNotFound)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT id, archived, externaluid, externalmods FROM volunteering_images WHERE opportunityid = ? ORDER BY id DESC LIMIT 1", id).Scan(&image)

			if image.ID > 0 {
				if image.Externaluid != "" {
					image.Ouruid = image.Externaluid
					image.Externalmods = image.Externalmods
					image.Path = misc.GetImageDeliveryUrl(image.Externaluid, string(image.Externalmods))
					image.Paththumb = misc.GetImageDeliveryUrl(image.Externaluid, string(image.Externalmods))
					image.Externaluid = ""
				} else if image.Archived > 0 {
					image.Path = "https://" + archiveDomain + "/oimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + archiveDomain + "/toimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				} else {
					image.Path = "https://" + imageDomain + "/oimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
					image.Paththumb = "https://" + imageDomain + "/toimg_" + strconv.FormatUint(image.ID, 10) + ".jpg"
				}
			}
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT groupid FROM volunteering_groups WHERE volunteeringid = ?", id).Pluck("groupid", &groups)
		}()

		wg.Add(1)

		go func() {
			defer wg.Done()

			db.Raw("SELECT * FROM volunteering_dates WHERE volunteeringid = ?", id).Scan(&dates)
		}()

		wg.Wait()

		if found {
			if image.ID > 0 {
				volunteering.Image = &image
			}

			volunteering.Groups = groups
			volunteering.Dates = dates

			// Decode HTML entities in text fields
			volunteering.Title = html.UnescapeString(volunteering.Title)
			volunteering.Description = html.UnescapeString(volunteering.Description)
			volunteering.Location = html.UnescapeString(volunteering.Location)
			volunteering.Contactname = html.UnescapeString(volunteering.Contactname)
			volunteering.Contacturl = html.UnescapeString(volunteering.Contacturl)
			volunteering.Timecommitment = html.UnescapeString(volunteering.Timecommitment)

			return c.JSON(volunteering)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Not found")
}

// canModify checks if a user can modify a volunteering opportunity.
// They can if they created it, are admin/support, or are a moderator/owner of a group
// the volunteering is linked to.
func canModify(myid uint64, volunteeringID uint64) bool {
	db := database.DBConn

	var ownerID uint64
	db.Raw("SELECT userid FROM volunteering WHERE id = ?", volunteeringID).Scan(&ownerID)

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
	db.Raw("SELECT groupid FROM volunteering_groups WHERE volunteeringid = ?", volunteeringID).Pluck("groupid", &groupIDs)

	for _, gid := range groupIDs {
		var role string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'", myid, gid).Scan(&role)

		if role == "Moderator" || role == "Owner" {
			return true
		}
	}

	return false
}

// isModerator checks if a user is a moderator who can hold/release volunteering opportunities.
func isModerator(myid uint64, volunteeringID uint64) bool {
	db := database.DBConn

	// Check if user is admin/support
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	// Check if user is moderator/owner of any linked group
	var groupIDs []uint64
	db.Raw("SELECT groupid FROM volunteering_groups WHERE volunteeringid = ?", volunteeringID).Pluck("groupid", &groupIDs)

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
	Title          string `json:"title"`
	Online         bool   `json:"online"`
	Location       string `json:"location"`
	Contactname    string `json:"contactname"`
	Contactphone   string `json:"contactphone"`
	Contactemail   string `json:"contactemail"`
	Contacturl     string `json:"contacturl"`
	Description    string `json:"description"`
	Timecommitment string `json:"timecommitment"`
	GroupID        uint64 `json:"groupid"`
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

	result := db.Exec("INSERT INTO volunteering (userid, pending, title, online, location, contactname, contactphone, contactemail, contacturl, description, timecommitment) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		myid, req.Title, req.Online, req.Location, req.Contactname, req.Contactphone, req.Contactemail, req.Contacturl, req.Description, req.Timecommitment)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create volunteering")
	}

	var id uint64
	db.Raw("SELECT id FROM volunteering WHERE userid = ? ORDER BY id DESC LIMIT 1", myid).Scan(&id)

	if id > 0 && req.GroupID > 0 {
		db.Exec("INSERT IGNORE INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", id, req.GroupID)
	}

	return c.JSON(fiber.Map{"id": id})
}

type PatchRequest struct {
	ID             uint64  `json:"id"`
	Action         string  `json:"action"`
	Title          *string `json:"title,omitempty"`
	Location       *string `json:"location,omitempty"`
	Online         *bool   `json:"online,omitempty"`
	Pending        *int    `json:"pending,omitempty"`
	Contactname    *string `json:"contactname,omitempty"`
	Contactphone   *string `json:"contactphone,omitempty"`
	Contactemail   *string `json:"contactemail,omitempty"`
	Contacturl     *string `json:"contacturl,omitempty"`
	Description    *string `json:"description,omitempty"`
	Timecommitment *string `json:"timecommitment,omitempty"`
	GroupID        uint64  `json:"groupid"`
	DateID         uint64  `json:"dateid"`
	PhotoID        uint64  `json:"photoid"`
	Start          string  `json:"start"`
	End            string  `json:"end"`
	Applyby        string  `json:"applyby"`
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

	// Check the volunteering exists
	db := database.DBConn
	var exists uint64
	db.Raw("SELECT id FROM volunteering WHERE id = ?", req.ID).Scan(&exists)
	if exists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Volunteering not found")
	}

	if !canModify(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized to modify this volunteering")
	}

	// Update settable attributes
	if req.Title != nil {
		db.Exec("UPDATE volunteering SET title = ? WHERE id = ?", *req.Title, req.ID)
	}
	if req.Location != nil {
		db.Exec("UPDATE volunteering SET location = ? WHERE id = ?", *req.Location, req.ID)
	}
	if req.Online != nil {
		db.Exec("UPDATE volunteering SET online = ? WHERE id = ?", *req.Online, req.ID)
	}
	if req.Pending != nil {
		db.Exec("UPDATE volunteering SET pending = ? WHERE id = ?", *req.Pending, req.ID)
	}
	if req.Contactname != nil {
		db.Exec("UPDATE volunteering SET contactname = ? WHERE id = ?", *req.Contactname, req.ID)
	}
	if req.Contactphone != nil {
		db.Exec("UPDATE volunteering SET contactphone = ? WHERE id = ?", *req.Contactphone, req.ID)
	}
	if req.Contactemail != nil {
		db.Exec("UPDATE volunteering SET contactemail = ? WHERE id = ?", *req.Contactemail, req.ID)
	}
	if req.Contacturl != nil {
		db.Exec("UPDATE volunteering SET contacturl = ? WHERE id = ?", *req.Contacturl, req.ID)
	}
	if req.Description != nil {
		db.Exec("UPDATE volunteering SET description = ? WHERE id = ?", *req.Description, req.ID)
	}
	if req.Timecommitment != nil {
		db.Exec("UPDATE volunteering SET timecommitment = ? WHERE id = ?", *req.Timecommitment, req.ID)
	}

	// Process action
	switch req.Action {
	case "AddGroup":
		if req.GroupID > 0 {
			db.Exec("INSERT IGNORE INTO volunteering_groups (volunteeringid, groupid) VALUES (?, ?)", req.ID, req.GroupID)

			// Side effects: create newsfeed entry and notify group moderators.
			// 1. Create newsfeed entry for this volunteering opportunity.
			var ownerID uint64
			db.Raw("SELECT userid FROM volunteering WHERE id = ?", req.ID).Scan(&ownerID)
			if ownerID > 0 {
				volID := req.ID
				newsfeed.CreateNewsfeedEntry(newsfeed.TypeVolunteerOpportunity, ownerID, req.GroupID, nil, &volID)
			}

			// 2. Notify group moderators via background task queue.
			queue.QueueTask(queue.TaskPushNotifyGroupMods, map[string]interface{}{
				"group_id": req.GroupID,
			})
		}
	case "RemoveGroup":
		if req.GroupID > 0 {
			db.Exec("DELETE FROM volunteering_groups WHERE volunteeringid = ? AND groupid = ?", req.ID, req.GroupID)
		}
	case "AddDate":
		db.Exec("INSERT INTO volunteering_dates (volunteeringid, start, end, applyby) VALUES (?, ?, ?, ?)",
			req.ID, nilIfEmpty(req.Start), nilIfEmpty(req.End), nilIfEmpty(req.Applyby))
	case "RemoveDate":
		if req.DateID > 0 {
			db.Exec("DELETE FROM volunteering_dates WHERE id = ?", req.DateID)
		}
	case "SetPhoto":
		if req.PhotoID > 0 {
			db.Exec("UPDATE volunteering_images SET opportunityid = ? WHERE id = ?", req.ID, req.PhotoID)
		}
	case "Renew":
		db.Exec("UPDATE volunteering SET renewed = NOW(), expired = 0 WHERE id = ?", req.ID)
	case "Expire":
		db.Exec("UPDATE volunteering SET expired = 1 WHERE id = ?", req.ID)
	case "Hold":
		if isModerator(myid, req.ID) {
			db.Exec("UPDATE volunteering SET heldby = ? WHERE id = ?", myid, req.ID)
		}
	case "Release":
		if isModerator(myid, req.ID) {
			db.Exec("UPDATE volunteering SET heldby = NULL WHERE id = ?", req.ID)
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
	db.Raw("SELECT id FROM volunteering WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Volunteering not found")
	}

	if !canModify(myid, id) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized to delete this volunteering")
	}

	// Soft delete.
	db.Exec("UPDATE volunteering SET deleted = 1, deletedby = ? WHERE id = ?", myid, id)

	return c.JSON(fiber.Map{"success": true})
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
