package logs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/log"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// GetLogs handles GET /logs for moderator log viewing.
//
// @Summary Get logs
// @Description Returns moderator logs filtered by type, group, search, with pagination
// @Tags logs
// @Produce json
// @Param logtype query string false "Log type: messages, memberships, user"
// @Param groupid query integer false "Group ID"
// @Param userid query integer false "User ID"
// @Param logsubtype query string false "Log subtype filter"
// @Param date query integer false "Days ago"
// @Param search query string false "Search term"
// @Param limit query integer false "Result limit (default 20)"
// @Param context query string false "Pagination context (last log ID)"
// @Success 200 {object} map[string]interface{}
// @Router /api/logs [get]
func GetLogs(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
	}

	db := database.DBConn

	logtype := c.Query("logtype", "")
	groupid, _ := strconv.ParseUint(c.Query("groupid", "0"), 10, 64)
	userid, _ := strconv.ParseUint(c.Query("userid", "0"), 10, 64)
	logsubtype := c.Query("logsubtype", "")
	dateStr := c.Query("date", "")
	search := c.Query("search", "")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	contextID, _ := strconv.ParseUint(c.Query("context", "0"), 10, 64)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Permission check: must be moderator/owner of the group, or admin/support.
	isAdmin := auth.IsAdminOrSupport(myid)

	// Non-admins need either a group or user filter, and can only see logs for groups they moderate.
	var modGroupIDs []uint64

	if !isAdmin {
		if groupid == 0 && userid == 0 {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
		}

		// Get all groups this user moderates.
		db.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Pluck("groupid", &modGroupIDs)

		if groupid > 0 {
			// Check they moderate the specific group requested.
			found := false
			for _, gid := range modGroupIDs {
				if gid == groupid {
					found = true
					break
				}
			}
			if !found {
				return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
			}
		}
	}

	// Build query based on logtype.
	var types []string
	var subtypes []string

	switch logtype {
	case "messages":
		types = []string{log.LOG_TYPE_MESSAGE}
		if logsubtype != "" {
			subtypes = []string{logsubtype}
		} else {
			subtypes = []string{log.LOG_SUBTYPE_RECEIVED, log.LOG_SUBTYPE_APPROVED, log.LOG_SUBTYPE_REJECTED, log.LOG_SUBTYPE_DELETED, log.LOG_SUBTYPE_AUTO_REPOSTED, log.LOG_SUBTYPE_AUTO_APPROVED, log.LOG_SUBTYPE_OUTCOME}
		}
	case "memberships":
		types = []string{log.LOG_TYPE_GROUP, log.LOG_TYPE_USER}
		if logsubtype != "" {
			subtypes = []string{logsubtype}
		} else {
			subtypes = []string{log.LOG_SUBTYPE_JOINED, log.LOG_SUBTYPE_REJECTED, log.LOG_SUBTYPE_APPROVED, log.LOG_SUBTYPE_APPLIED, log.LOG_SUBTYPE_AUTO_APPROVED, log.LOG_SUBTYPE_LEFT}
		}
	case "user":
		// User-specific logs: all actions affecting this user (V1 parity:
		// getPublicLogs returns all types except Created/Merged/YahooConfirmed).
		types = nil
		subtypes = nil
	default:
		// General logs - just filter by group/user.
		types = nil
		subtypes = nil
	}

	// Build WHERE clauses.
	where := []string{"1=1"}
	args := []interface{}{}

	// V1 parity: exclude uninteresting log subtypes for user-specific logs.
	if logtype == "user" {
		where = append(where, "NOT (logs.type = 'User' AND logs.subtype IN ('Created', 'Merged'))")
	}

	if groupid > 0 {
		where = append(where, "logs.groupid = ?")
		args = append(args, groupid)
	} else if logtype != "user" && !isAdmin && len(modGroupIDs) > 0 {
		// Non-admins can only see logs for groups they moderate.
		// Exception: user-specific logs (logtype=user) show all groups
		// (V1 parity: getPublicLogs doesn't filter by group).
		placeholders := strings.Repeat("?,", len(modGroupIDs))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, fmt.Sprintf("logs.groupid IN (%s)", placeholders))
		for _, gid := range modGroupIDs {
			args = append(args, gid)
		}
	}

	if len(types) > 0 {
		placeholders := strings.Repeat("?,", len(types))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, fmt.Sprintf("logs.type IN (%s)", placeholders))
		for _, t := range types {
			args = append(args, t)
		}
	}

	if len(subtypes) > 0 {
		placeholders := strings.Repeat("?,", len(subtypes))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, fmt.Sprintf("logs.subtype IN (%s)", placeholders))
		for _, s := range subtypes {
			args = append(args, s)
		}
	}

	if dateStr != "" {
		days, _ := strconv.Atoi(dateStr)
		if days >= 0 {
			mysqlTime := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
			where = append(where, "logs.timestamp >= ?")
			args = append(args, mysqlTime)
		}
	}

	if contextID > 0 {
		where = append(where, "logs.id < ?")
		args = append(args, contextID)
	}

	if userid > 0 {
		where = append(where, "(logs.user = ? OR logs.byuser = ?)")
		args = append(args, userid, userid)
	}

	// Build the query.
	query := "SELECT logs.* FROM logs "

	if search != "" {
		query += "LEFT JOIN users ON users.id = logs.user " +
			"LEFT JOIN messages ON messages.id = logs.msgid "

		searchLike := "%" + search + "%"
		where = append(where, "(users.firstname LIKE ? OR users.lastname LIKE ? OR users.fullname LIKE ? "+
			"OR CONCAT(users.firstname, ' ', users.lastname) LIKE ? OR messages.subject LIKE ?)")
		args = append(args, searchLike, searchLike, searchLike, searchLike, searchLike)
	}

	query += "WHERE " + strings.Join(where, " AND ") +
		" ORDER BY logs.id DESC LIMIT ?"
	args = append(args, limit)

	type LogRow struct {
		ID        uint64  `json:"id"`
		Timestamp string  `json:"timestamp"`
		Type      string  `json:"type"`
		Subtype   *string `json:"subtype"`
		Groupid   *uint64 `json:"groupid"`
		User      *uint64 `json:"user"`
		Byuser    *uint64 `json:"byuser"`
		Msgid     *uint64 `json:"msgid"`
		Configid  *uint64 `json:"configid"`
		Stdmsgid  *uint64 `json:"stdmsgid"`
		Text      *string `json:"text"`
	}

	var rows []LogRow
	db.Raw(query, args...).Scan(&rows)

	// Enrich with user and message data.
	result := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		entry := map[string]interface{}{
			"id":        r.ID,
			"timestamp": r.Timestamp,
			"type":      r.Type,
			"subtype":   r.Subtype,
			"groupid":   r.Groupid,
			"text":      r.Text,
		}

		// V2 pattern: return IDs only — frontend fetches details from stores.
		if r.User != nil && *r.User > 0 {
			entry["userid"] = *r.User
		}

		if r.Byuser != nil && *r.Byuser > 0 {
			entry["byuserid"] = *r.Byuser
		}

		if r.Msgid != nil && *r.Msgid > 0 {
			entry["msgid"] = *r.Msgid
		}

		if r.Stdmsgid != nil && *r.Stdmsgid > 0 {
			entry["stdmsgid"] = *r.Stdmsgid
		}

		if r.Configid != nil && *r.Configid > 0 {
			entry["configid"] = *r.Configid
		}

		// Outcome subtype has long text like "Taken: thanks everyone".
		// Trim to just the first word (e.g. "Taken").
		if r.Subtype != nil && *r.Subtype == log.LOG_SUBTYPE_OUTCOME && r.Text != nil && *r.Text != "" {
			firstWord := strings.SplitN(*r.Text, " ", 2)[0]
			entry["text"] = &firstWord
		}

		result[i] = entry
	}

	// Build context for pagination. Return null when no more results to prevent infinite loop.
	var ctx interface{}
	if len(rows) > 0 {
		ctx = map[string]interface{}{
			"id": rows[len(rows)-1].ID,
		}
	}

	return c.JSON(fiber.Map{
		"ret":     0,
		"status":  "Success",
		"logs":    result,
		"context": ctx,
	})
}
