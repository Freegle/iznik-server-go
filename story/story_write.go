package story

import (
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// PutStoryRequest is the request body for creating a new story
type PutStoryRequest struct {
	Public   bool   `json:"public"`
	Headline string `json:"headline"`
	Story    string `json:"story"`
	Photo    uint64 `json:"photo"`
}

// PostStoryRequest is the request body for like/unlike actions
type PostStoryRequest struct {
	ID     uint64 `json:"id"`
	Action string `json:"action"`
}

// PatchStoryRequest is the request body for updating story attributes (mod-only)
type PatchStoryRequest struct {
	ID                  uint64  `json:"id"`
	Reviewed            *bool   `json:"reviewed"`
	Public              *bool   `json:"public"`
	Newsletter          *bool   `json:"newsletter"`
	Newsletterreviewed  *bool   `json:"newsletterreviewed"`
	Headline            *string `json:"headline"`
	Story               *string `json:"story"`
}

// canModStory checks if a user can moderate a story.
// Returns true if the user is Admin/Support, or is a Moderator/Owner
// on any group that the story's author belongs to.
func canModStory(myid uint64, storyid uint64) bool {
	db := database.DBConn

	// Check if user is admin/support
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	if systemrole == "Admin" || systemrole == "Support" {
		return true
	}

	// Check if mod for any group the story owner belongs to
	var count int64
	db.Raw(`SELECT COUNT(*) FROM users_stories s
		JOIN memberships m1 ON m1.userid = s.userid
		JOIN memberships m2 ON m2.groupid = m1.groupid AND m2.userid = ?
		WHERE s.id = ? AND m2.role IN ('Moderator', 'Owner')`, myid, storyid).Scan(&count)
	return count > 0
}

// PutStory creates a new story
// @Summary Create story
// @Description Creates a new user story. Requires authentication.
// @Tags story
// @Accept json
// @Produce json
// @Param body body PutStoryRequest true "Story data"
// @Security BearerAuth
// @Success 200 {object} fiber.Map "Success with story ID"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Router /story [put]
func PutStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PutStoryRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	db := database.DBConn

	publicVal := 0
	if req.Public {
		publicVal = 1
	}

	result := db.Exec("INSERT INTO users_stories (userid, public, headline, story) VALUES (?, ?, ?, ?)",
		myid, publicVal, req.Headline, req.Story)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create story")
	}

	var id uint64
	db.Raw("SELECT id FROM users_stories WHERE userid = ? ORDER BY id DESC LIMIT 1", myid).Scan(&id)

	// If photo provided, link the image to this story
	if req.Photo > 0 {
		db.Exec("UPDATE users_stories_images SET storyid = ? WHERE id = ?", id, req.Photo)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": id})
}

// PostStory handles like/unlike actions on a story
// @Summary Like or unlike a story
// @Description Performs a Like or Unlike action on a story. Requires authentication.
// @Tags story
// @Accept json
// @Produce json
// @Param body body PostStoryRequest true "Action data"
// @Security BearerAuth
// @Success 200 {object} fiber.Map "Success"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Failure 400 {object} fiber.Error "Invalid action"
// @Router /story [post]
func PostStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostStoryRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	switch req.Action {
	case "Like":
		db.Exec("INSERT INTO users_stories_likes (storyid, userid) VALUES (?, ?) ON DUPLICATE KEY UPDATE storyid = storyid",
			req.ID, myid)
	case "Unlike":
		db.Exec("DELETE FROM users_stories_likes WHERE storyid = ? AND userid = ?",
			req.ID, myid)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "Invalid action, must be Like or Unlike")
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// PatchStory updates story attributes (mod-only)
// @Summary Update story
// @Description Updates story attributes. Requires moderator permissions for the story owner's group.
// @Tags story
// @Accept json
// @Produce json
// @Param body body PatchStoryRequest true "Fields to update"
// @Security BearerAuth
// @Success 200 {object} fiber.Map "Success"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Failure 403 {object} fiber.Error "Not authorized"
// @Router /story [patch]
func PatchStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PatchStoryRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	if !canModStory(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	db := database.DBConn

	// Get current reviewed+public state before updates
	type StoryState struct {
		Reviewed      bool
		Public        bool
		Fromnewsfeed  bool
		Userid        uint64
	}
	var before StoryState
	db.Raw("SELECT reviewed, public, fromnewsfeed, userid FROM users_stories WHERE id = ?", req.ID).Scan(&before)

	// Update whichever fields are provided
	if req.Reviewed != nil {
		db.Exec("UPDATE users_stories SET reviewed = ? WHERE id = ?", *req.Reviewed, req.ID)
		if *req.Reviewed {
			db.Exec("UPDATE users_stories SET reviewedby = ? WHERE id = ?", myid, req.ID)
		}
	}
	if req.Public != nil {
		db.Exec("UPDATE users_stories SET public = ? WHERE id = ?", *req.Public, req.ID)
	}
	if req.Newsletter != nil {
		db.Exec("UPDATE users_stories SET newsletter = ? WHERE id = ?", *req.Newsletter, req.ID)
	}
	if req.Newsletterreviewed != nil {
		db.Exec("UPDATE users_stories SET newsletterreviewed = ? WHERE id = ?", *req.Newsletterreviewed, req.ID)
		if *req.Newsletterreviewed {
			db.Exec("UPDATE users_stories SET newsletterreviewedby = ? WHERE id = ?", myid, req.ID)
		}
	}
	if req.Headline != nil {
		db.Exec("UPDATE users_stories SET headline = ? WHERE id = ?", *req.Headline, req.ID)
	}
	if req.Story != nil {
		db.Exec("UPDATE users_stories SET story = ? WHERE id = ?", *req.Story, req.ID)
	}

	// Check if story has become newly reviewed+public (wasn't before)
	newsfeedBefore := before.Reviewed && before.Public

	// Re-read the state after updates
	var after StoryState
	db.Raw("SELECT reviewed, public, fromnewsfeed, userid FROM users_stories WHERE id = ?", req.ID).Scan(&after)
	newsfeedAfter := after.Reviewed && after.Public

	if !newsfeedBefore && newsfeedAfter && !after.Fromnewsfeed {
		// Story has become reviewed+public and wasn't originally from newsfeed.
		// Create a newsfeed entry so nearby users see it.
		// Get user location for the position - only create if we have a location.
		type UserLoc struct {
			Lat *float64
			Lng *float64
		}
		var ul UserLoc
		db.Raw("SELECT l.lat, l.lng FROM users u LEFT JOIN locations l ON l.id = u.lastlocation WHERE u.id = ?", after.Userid).Scan(&ul)

		if ul.Lat != nil && ul.Lng != nil {
			db.Exec("INSERT INTO newsfeed (type, userid, storyid, timestamp, position, deleted, reviewrequired, pinned) "+
				"VALUES ('Story', ?, ?, NOW(), ST_GeomFromText(CONCAT('POINT(', ?, ' ', ?, ')'), ?), NULL, 0, 0)",
				after.Userid, req.ID, *ul.Lng, *ul.Lat, utils.SRID)
		}
		// If no location, skip newsfeed entry (matches PHP behavior: only insert if lat/lng available)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteStory deletes a story (mod-only)
// @Summary Delete story
// @Description Deletes a story by ID. Requires moderator permissions for the story owner's group.
// @Tags story
// @Produce json
// @Param id path integer true "Story ID"
// @Security BearerAuth
// @Success 200 {object} fiber.Map "Success"
// @Failure 400 {object} fiber.Error "Invalid ID"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Failure 403 {object} fiber.Error "Not authorized"
// @Failure 404 {object} fiber.Error "Story not found"
// @Router /story/{id} [delete]
func DeleteStory(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	storyID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || storyID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid id")
	}

	db := database.DBConn

	// Check story exists
	var count int64
	db.Raw("SELECT COUNT(*) FROM users_stories WHERE id = ?", storyID).Scan(&count)
	if count == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Story not found")
	}

	if !canModStory(myid, storyID) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	result := db.Exec("DELETE FROM users_stories WHERE id = ?", storyID)
	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete story")
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
