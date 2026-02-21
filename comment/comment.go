package comment

import (
	"strconv"
	"sync"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// CommentItem is a flat comment representation. Client fetches user details separately via /user/:id.
type CommentItem struct {
	ID       uint64     `json:"id"`
	Userid   uint64     `json:"userid"`
	Groupid  *uint64    `json:"groupid"`
	Byuserid *uint64    `json:"byuserid"`
	Date     *time.Time `json:"date"`
	Reviewed *time.Time `json:"reviewed"`
	User1    *string    `json:"user1"`
	User2    *string    `json:"user2"`
	User3    *string    `json:"user3"`
	User4    *string    `json:"user4"`
	User5    *string    `json:"user5"`
	User6    *string    `json:"user6"`
	User7    *string    `json:"user7"`
	User8    *string    `json:"user8"`
	User9    *string    `json:"user9"`
	User10   *string    `json:"user10"`
	User11   *string    `json:"user11"`
	Flag     bool       `json:"flag"`
	Flagged  bool       `json:"flagged"`
}

// Get handles GET /api/comment
// Returns flat comment objects with userid/byuserid as IDs. Client fetches user details separately.
func Get(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)

	if id > 0 {
		return getSingle(c, myid, id)
	}

	// List comments for moderated groups.
	groupid, _ := strconv.ParseUint(c.Query("groupid", "0"), 10, 64)
	contextReviewed := c.Query("context[reviewed]", "")

	// Get groups where user is moderator + system role in parallel.
	var wg sync.WaitGroup
	var modGroupIDs []uint64
	var systemrole string

	wg.Add(2)
	go func() {
		defer wg.Done()
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'", myid).Pluck("groupid", &modGroupIDs)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	}()
	wg.Wait()

	if len(modGroupIDs) == 0 && systemrole != "Support" && systemrole != "Admin" {
		return c.JSON(fiber.Map{
			"comments": make([]CommentItem, 0),
			"context":  nil,
		})
	}

	// Build query.
	query := "SELECT * FROM users_comments WHERE "
	var args []interface{}

	if groupid > 0 {
		query += "groupid = ? AND "
		args = append(args, groupid)
	}

	if contextReviewed != "" {
		query += "users_comments.reviewed < ? AND "
		args = append(args, contextReviewed)
	}

	if systemrole == "Support" || systemrole == "Admin" {
		// Admin/support can see all comments.
	} else {
		query += "(groupid IN (?) OR users_comments.byuserid = ?) AND "
		args = append(args, modGroupIDs, myid)
	}

	query += "1=1 ORDER BY reviewed DESC LIMIT 10"

	var rows []CommentItem
	db.Raw(query, args...).Scan(&rows)

	if len(rows) == 0 {
		return c.JSON(fiber.Map{
			"comments": make([]CommentItem, 0),
			"context":  nil,
		})
	}

	// Set Flagged from Flag for each row.
	for i := range rows {
		rows[i].Flagged = rows[i].Flag
	}

	var ctx interface{}
	lastReviewed := rows[len(rows)-1].Reviewed
	if lastReviewed != nil {
		ctx = fiber.Map{"reviewed": lastReviewed.Format(time.RFC3339)}
	}

	return c.JSON(fiber.Map{
		"comments": rows,
		"context":  ctx,
	})
}

func getSingle(c *fiber.Ctx, myid uint64, id uint64) error {
	db := database.DBConn

	// Get moderator group IDs and system role in parallel.
	var wg sync.WaitGroup
	var modGroupIDs []uint64
	var systemrole string

	wg.Add(2)
	go func() {
		defer wg.Done()
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND collection = 'Approved'", myid).Pluck("groupid", &modGroupIDs)
	}()
	go func() {
		defer wg.Done()
		db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	}()
	wg.Wait()

	var row CommentItem

	if systemrole == "Support" || systemrole == "Admin" {
		db.Raw("SELECT * FROM users_comments WHERE id = ?", id).Scan(&row)
	} else if len(modGroupIDs) > 0 {
		db.Raw("SELECT * FROM users_comments WHERE id = ? AND groupid IN (?)", id, modGroupIDs).Scan(&row)
	}

	if row.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Comment not found")
	}

	row.Flagged = row.Flag

	return c.JSON(row)
}

type CreateRequest struct {
	Userid  uint64  `json:"userid"`
	Groupid *uint64 `json:"groupid"`
	User1   *string `json:"user1"`
	User2   *string `json:"user2"`
	User3   *string `json:"user3"`
	User4   *string `json:"user4"`
	User5   *string `json:"user5"`
	User6   *string `json:"user6"`
	User7   *string `json:"user7"`
	User8   *string `json:"user8"`
	User9   *string `json:"user9"`
	User10  *string `json:"user10"`
	User11  *string `json:"user11"`
	Flag    bool    `json:"flag"`
}

type PatchRequest struct {
	ID     uint64  `json:"id"`
	User1  *string `json:"user1"`
	User2  *string `json:"user2"`
	User3  *string `json:"user3"`
	User4  *string `json:"user4"`
	User5  *string `json:"user5"`
	User6  *string `json:"user6"`
	User7  *string `json:"user7"`
	User8  *string `json:"user8"`
	User9  *string `json:"user9"`
	User10 *string `json:"user10"`
	User11 *string `json:"user11"`
	Flag   *bool   `json:"flag"`
}

// canModerate checks if the user is a moderator/owner of the group, or admin/support.
func canModerate(myid uint64, groupid *uint64) bool {
	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	if groupid == nil || *groupid == 0 {
		return false
	}

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'", myid, *groupid).Scan(&role)

	return role == "Moderator" || role == "Owner"
}

// canModerateComment checks if the user can modify a specific existing comment.
func canModerateComment(myid uint64, commentID uint64) bool {
	db := database.DBConn

	var groupid *uint64
	db.Raw("SELECT groupid FROM users_comments WHERE id = ?", commentID).Scan(&groupid)

	return canModerate(myid, groupid)
}

// flagOthers flags a user for review in all their groups except the given group.
// This replicates the PHP User::flagOthers() + User::memberReview() behavior.
func flagOthers(userid uint64, groupid uint64) {
	db := database.DBConn

	var otherGroupIDs []uint64
	db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND groupid != ?", userid, groupid).Pluck("groupid", &otherGroupIDs)

	now := time.Now().Format("2006-01-02 15:04")
	reason := "Note flagged to other groups"

	for _, gid := range otherGroupIDs {
		db.Exec("UPDATE memberships SET reviewreason = ?, reviewrequestedat = ? WHERE groupid = ? AND userid = ?",
			reason, now, gid, userid)
	}
}

// Create handles POST /api/comment
func Create(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Userid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "userid is required")
	}

	if !canModerate(myid, req.Groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn

	var flag int
	if req.Flag {
		flag = 1
	}

	result := db.Exec(
		"INSERT INTO users_comments (userid, groupid, byuserid, user1, user2, user3, user4, user5, user6, user7, user8, user9, user10, user11, flag) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		req.Userid, req.Groupid, myid,
		req.User1, req.User2, req.User3, req.User4, req.User5,
		req.User6, req.User7, req.User8, req.User9, req.User10,
		req.User11, flag,
	)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create comment")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	// Flag user in other groups if flag is set
	if id > 0 && req.Flag && req.Groupid != nil && *req.Groupid > 0 {
		flagOthers(req.Userid, *req.Groupid)
	}

	return c.JSON(fiber.Map{
		"id": id,
	})
}

// Edit handles PATCH /api/comment
func Edit(c *fiber.Ctx) error {
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

	if !canModerateComment(myid, req.ID) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn

	db.Exec(
		"UPDATE users_comments SET user1 = ?, user2 = ?, user3 = ?, user4 = ?, user5 = ?, user6 = ?, user7 = ?, user8 = ?, user9 = ?, user10 = ?, user11 = ?, flag = COALESCE(?, flag), byuserid = ?, reviewed = NOW() WHERE id = ?",
		req.User1, req.User2, req.User3, req.User4, req.User5,
		req.User6, req.User7, req.User8, req.User9, req.User10,
		req.User11, req.Flag, myid, req.ID,
	)

	// Flag user in other groups if flag is set to true
	if req.Flag != nil && *req.Flag {
		var commentUserid uint64
		var commentGroupid uint64
		db.Raw("SELECT userid, groupid FROM users_comments WHERE id = ?", req.ID).Row().Scan(&commentUserid, &commentGroupid)
		if commentUserid > 0 && commentGroupid > 0 {
			flagOthers(commentUserid, commentGroupid)
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
	})
}

// Delete handles DELETE /api/comment/:id
func Delete(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid comment ID")
	}

	if !canModerateComment(myid, id) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn
	db.Exec("DELETE FROM users_comments WHERE id = ?", id)

	return c.JSON(fiber.Map{
		"success": true,
	})
}
