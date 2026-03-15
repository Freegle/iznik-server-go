package user

import (
	"strconv"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

// All endpoints in this file are mod-only: the caller must be a moderator of
// a group the target user belongs to (or Admin/Support).  Each returns a flat
// array — no nested enrichment.

func requireModOfUser(c *fiber.Ctx) (myid, targetid uint64, err error) {
	myid = WhoAmI(c)
	if myid == 0 {
		return 0, 0, fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}
	targetid, parseErr := strconv.ParseUint(c.Params("id"), 10, 64)
	if parseErr != nil || targetid == 0 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "Invalid user ID")
	}
	if !IsModOfUser(myid, targetid) {
		return 0, 0, fiber.NewError(fiber.StatusForbidden, "Not a moderator for this user")
	}
	return myid, targetid, nil
}

// GetUserChatrooms returns chat rooms for a target user.
//
// @Summary Get chat rooms for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/chatrooms [get]
func GetUserChatrooms(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type ChatroomRow struct {
		ID       uint64     `json:"id"`
		Chattype string     `json:"chattype"`
		User1    uint64     `json:"user1"`
		User2    uint64     `json:"user2"`
		Groupid  uint64     `json:"groupid"`
		Lastdate *time.Time `json:"lastdate"`
	}

	var rooms []ChatroomRow
	db.Raw("SELECT id, chattype, user1, user2, COALESCE(groupid, 0) AS groupid, latestmessage AS lastdate "+
		"FROM chat_rooms WHERE (user1 = ? OR user2 = ?) "+
		"ORDER BY latestmessage DESC LIMIT 100",
		targetid, targetid).Scan(&rooms)

	if rooms == nil {
		rooms = []ChatroomRow{}
	}

	return c.JSON(rooms)
}

// GetUserEmailHistory returns recent emails sent to a user.
//
// @Summary Get email history for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/emailhistory [get]
func GetUserEmailHistory(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type EmailHistoryRow struct {
		ID        uint64     `json:"id"`
		Timestamp *time.Time `json:"timestamp"`
		Eximid    *string    `json:"eximid"`
		From      *string    `json:"from"`
		To        *string    `json:"to"`
		Subject   *string    `json:"subject"`
		Status    *string    `json:"status"`
	}

	var emails []EmailHistoryRow
	db.Raw("SELECT id, timestamp, eximid, `from`, `to`, subject, status "+
		"FROM logs_emails WHERE userid = ? ORDER BY id DESC LIMIT 100",
		targetid).Scan(&emails)

	if emails == nil {
		emails = []EmailHistoryRow{}
	}

	return c.JSON(emails)
}

// GetUserBans returns ban records for a user.
//
// @Summary Get bans for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/bans [get]
func GetUserBans(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type BanRow struct {
		Groupid uint64     `json:"groupid"`
		Group   string     `json:"group"`
		Date    *time.Time `json:"date"`
		Byuser  *uint64    `json:"byuser"`
		Byemail *string    `json:"byemail"`
	}

	var bans []BanRow
	db.Raw("SELECT ub.groupid, "+
		"COALESCE(g.namefull, g.nameshort) AS `group`, "+
		"ub.date, ub.byuser, "+
		"(SELECT ue.email FROM users_emails ue WHERE ue.userid = ub.byuser AND ue.preferred = 1 LIMIT 1) AS byemail "+
		"FROM users_banned ub "+
		"LEFT JOIN `groups` g ON g.id = ub.groupid "+
		"WHERE ub.userid = ? ORDER BY ub.date DESC",
		targetid).Scan(&bans)

	if bans == nil {
		bans = []BanRow{}
	}

	return c.JSON(bans)
}

// GetUserNewsfeed returns ChitChat posts by a user.
//
// @Summary Get newsfeed posts for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/newsfeed [get]
func GetUserNewsfeed(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type NewsfeedRow struct {
		ID        uint64     `json:"id"`
		Timestamp *time.Time `json:"timestamp"`
		Message   *string    `json:"message"`
		Hidden    *time.Time `json:"hidden"`
		Hiddenby  *uint64    `json:"hiddenby"`
		Deleted   *time.Time `json:"deleted"`
		Deletedby *uint64    `json:"deletedby"`
	}

	var posts []NewsfeedRow
	db.Raw("SELECT id, timestamp, message, hidden, hiddenby, deleted, deletedby "+
		"FROM newsfeed WHERE userid = ? AND replyto IS NULL "+
		"ORDER BY id DESC LIMIT 100",
		targetid).Scan(&posts)

	if posts == nil {
		posts = []NewsfeedRow{}
	}

	return c.JSON(posts)
}

// GetUserApplied returns recent group applications (last 31 days).
//
// @Summary Get recent group applications for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/applied [get]
func GetUserApplied(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type AppliedRow struct {
		Groupid   uint64     `json:"groupid"`
		Nameshort string     `json:"nameshort"`
		Added     *time.Time `json:"added"`
	}

	var applied []AppliedRow
	db.Raw("SELECT mh.groupid, COALESCE(g.namefull, g.nameshort) AS nameshort, mh.added "+
		"FROM memberships_history mh "+
		"INNER JOIN `groups` g ON g.id = mh.groupid "+
		"WHERE mh.userid = ? AND DATEDIFF(NOW(), mh.added) <= 31 "+
		"AND g.publish = 1 AND g.onmap = 1 "+
		"ORDER BY mh.added DESC",
		targetid).Scan(&applied)

	if applied == nil {
		applied = []AppliedRow{}
	}

	return c.JSON(applied)
}

// GetUserMembershipHistory returns full membership history.
//
// @Summary Get membership history for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/membershiphistory [get]
func GetUserMembershipHistory(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type MembershipHistoryRow struct {
		Groupid    uint64     `json:"groupid"`
		Nameshort  string     `json:"nameshort"`
		Added      *time.Time `json:"added"`
		Collection string     `json:"collection"`
	}

	limit := c.QueryInt("limit", 100)
	if limit > 500 {
		limit = 500
	}

	var history []MembershipHistoryRow
	db.Raw("SELECT mh.groupid, COALESCE(g.namefull, g.nameshort) AS nameshort, "+
		"mh.added, mh.collection "+
		"FROM memberships_history mh "+
		"INNER JOIN `groups` g ON g.id = mh.groupid "+
		"WHERE mh.userid = ? "+
		"ORDER BY mh.added DESC LIMIT ?",
		targetid, limit).Scan(&history)

	if history == nil {
		history = []MembershipHistoryRow{}
	}

	return c.JSON(history)
}


// GetUserLogins returns login history for a user.
//
// @Summary Get login history for a user (mod-only)
// @Tags user
// @Router /api/user/{id}/logins [get]
func GetUserLogins(c *fiber.Ctx) error {
	_, targetid, err := requireModOfUser(c)
	if err != nil {
		return err
	}

	db := database.DBConn

	type LoginRow struct {
		ID        uint64     `json:"id"`
		Userid    uint64     `json:"userid"`
		Type      string     `json:"type"`
		Added     *time.Time `json:"added"`
		Lastaccess *time.Time `json:"lastaccess"`
	}

	var logins []LoginRow
	db.Raw("SELECT id, userid, type, added, lastaccess FROM users_logins "+
		"WHERE userid = ? ORDER BY lastaccess DESC LIMIT 50",
		targetid).Scan(&logins)

	if logins == nil {
		logins = []LoginRow{}
	}

	return c.JSON(logins)
}

