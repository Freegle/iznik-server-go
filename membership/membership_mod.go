package membership

import (
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/queue"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// isModOfGroup checks if the caller is a Moderator or Owner of the given group,
// or has Admin/Support system role.
func isModOfGroup(myid uint64, groupid uint64) bool {
	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	if systemrole == utils.SYSTEMROLE_SUPPORT || systemrole == utils.SYSTEMROLE_ADMIN {
		return true
	}

	if groupid == 0 {
		return false
	}

	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Approved'",
		myid, groupid).Scan(&role)
	return role == utils.ROLE_MODERATOR || role == utils.ROLE_OWNER
}

// PostMembershipsRequest is the body for POST /memberships (moderator actions).
type PostMembershipsRequest struct {
	Userid    uint64  `json:"userid"`
	Groupid   uint64  `json:"groupid"`
	Action    string  `json:"action"`
	Subject   *string `json:"subject"`
	Body      *string `json:"body"`
	Stdmsgid  *uint64 `json:"stdmsgid"`
	Ban       *bool   `json:"ban"`
	Happiness *string `json:"happiness"`
}

// PostMemberships handles POST /memberships - moderator actions on memberships.
// Actions: Hold, Release, Approve, Leave Approved Member, Reject,
// Delete Approved Member, Ban, Unban, ReviewHold, ReviewRelease, HappinessReviewed.
func PostMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostMembershipsRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	if req.Userid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "userid is required")
	}

	if req.Action == "" {
		return fiber.NewError(fiber.StatusBadRequest, "action is required")
	}

	// Permission check: caller must be mod/owner of group or admin/support.
	if !isModOfGroup(myid, req.Groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	db := database.DBConn

	switch req.Action {
	case "Hold":
		db.Exec("UPDATE memberships SET heldby = ? WHERE userid = ? AND groupid = ?",
			myid, req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Release":
		db.Exec("UPDATE memberships SET heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Approve", "Leave Approved Member":
		db.Exec("UPDATE memberships SET collection = 'Approved', heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)

		// Queue welcome/approval email.
		taskData := map[string]interface{}{
			"userid":  req.Userid,
			"groupid": req.Groupid,
			"byuser":  myid,
		}
		if req.Subject != nil {
			taskData["subject"] = *req.Subject
		}
		if req.Body != nil {
			taskData["body"] = *req.Body
		}
		queue.QueueTask(TaskEmailMembershipApproved, taskData)

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Reject", "Delete Approved Member":
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection IN ('Pending', 'Approved')",
			req.Userid, req.Groupid)

		// Queue rejection notification.
		taskData := map[string]interface{}{
			"userid":  req.Userid,
			"groupid": req.Groupid,
			"byuser":  myid,
		}
		if req.Subject != nil {
			taskData["subject"] = *req.Subject
		}
		if req.Body != nil {
			taskData["body"] = *req.Body
		}
		if req.Stdmsgid != nil {
			taskData["stdmsgid"] = *req.Stdmsgid
		}
		queue.QueueTask(TaskEmailMembershipRejected, taskData)

		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Ban":
		// Delete existing membership.
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection IN ('Pending', 'Approved')",
			req.Userid, req.Groupid)
		// Add banned record.
		db.Exec("INSERT INTO memberships (userid, groupid, role, collection) VALUES (?, ?, 'Member', 'Banned')",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "Unban":
		db.Exec("DELETE FROM memberships WHERE userid = ? AND groupid = ? AND collection = 'Banned'",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "ReviewHold":
		// ReviewHold is used in the chat review context - sets heldby on the membership.
		db.Exec("UPDATE memberships SET heldby = ? WHERE userid = ? AND groupid = ?",
			myid, req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "ReviewRelease":
		// ReviewRelease clears the heldby on the membership (chat review context).
		db.Exec("UPDATE memberships SET heldby = NULL WHERE userid = ? AND groupid = ?",
			req.Userid, req.Groupid)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	case "HappinessReviewed":
		if req.Happiness == nil {
			return fiber.NewError(fiber.StatusBadRequest, "happiness is required for HappinessReviewed")
		}
		happinessID, err := strconv.ParseUint(*req.Happiness, 10, 64)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "happiness must be a valid ID")
		}
		db.Exec("UPDATE messages_outcomes SET reviewed = 1 WHERE id = ?", happinessID)
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})

	default:
		return fiber.NewError(fiber.StatusBadRequest, "Unknown action: "+req.Action)
	}
}

// Task type constants for membership moderation emails.
const (
	TaskEmailMembershipApproved = "email_membership_approved"
	TaskEmailMembershipRejected = "email_membership_rejected"
)

// GetMembershipsMember is the response struct for individual members in GetMemberships.
type GetMembershipsMember struct {
	Userid     uint64  `json:"userid"`
	Role       string  `json:"role"`
	Collection string  `json:"collection"`
	Added      *string `json:"added"`
	Heldby     *uint64 `json:"heldby"`
	Fullname   *string `json:"fullname"`
	Firstname  *string `json:"firstname"`
	Lastname   *string `json:"lastname"`
}

// GetMemberships handles GET /memberships - list group members (moderator use).
// Query params: groupid (required), collection (default "Approved"), limit (default 100), search (optional).
func GetMemberships(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	groupid := uint64(c.QueryInt("groupid", 0))
	if groupid == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "groupid is required")
	}

	if !isModOfGroup(myid, groupid) {
		return fiber.NewError(fiber.StatusForbidden, "Not a moderator of this group")
	}

	collection := c.Query("collection", "Approved")
	limit := c.QueryInt("limit", 100)
	search := c.Query("search", "")

	db := database.DBConn

	var members []GetMembershipsMember

	if search != "" {
		searchPattern := "%" + search + "%"
		db.Raw("SELECT m.userid, m.role, m.collection, m.added, m.heldby, "+
			"u.fullname, u.firstname, u.lastname "+
			"FROM memberships m "+
			"JOIN users u ON u.id = m.userid "+
			"WHERE m.groupid = ? AND m.collection = ? "+
			"AND (u.fullname LIKE ? OR EXISTS (SELECT 1 FROM users_emails WHERE userid = m.userid AND email LIKE ?)) "+
			"ORDER BY m.added DESC LIMIT ?",
			groupid, collection, searchPattern, searchPattern, limit).Scan(&members)
	} else {
		db.Raw("SELECT m.userid, m.role, m.collection, m.added, m.heldby, "+
			"u.fullname, u.firstname, u.lastname "+
			"FROM memberships m "+
			"JOIN users u ON u.id = m.userid "+
			"WHERE m.groupid = ? AND m.collection = ? "+
			"ORDER BY m.added DESC LIMIT ?",
			groupid, collection, limit).Scan(&members)
	}

	if members == nil {
		members = make([]GetMembershipsMember, 0)
	}

	return c.JSON(members)
}
