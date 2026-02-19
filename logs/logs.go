package logs

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
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
		return c.JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
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
	var role string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
	isAdmin := role == "Admin" || role == "Support"

	if !isAdmin && groupid > 0 {
		var memRole string
		db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", myid, groupid).Scan(&memRole)
		if memRole != "Moderator" && memRole != "Owner" {
			return c.JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
		}
	} else if !isAdmin && groupid == 0 && userid == 0 {
		// Need a group or user filter for non-admins.
		return c.JSON(fiber.Map{"ret": 2, "status": "Not moderator"})
	}

	// Build query based on logtype.
	var types []string
	var subtypes []string

	switch logtype {
	case "messages":
		types = []string{"Message"}
		if logsubtype != "" {
			subtypes = []string{logsubtype}
		} else {
			subtypes = []string{"Received", "Approved", "Rejected", "Deleted", "Autoreposted", "Autoapproved", "Outcome"}
		}
	case "memberships":
		types = []string{"Group", "User"}
		if logsubtype != "" {
			subtypes = []string{logsubtype}
		} else {
			subtypes = []string{"Joined", "Rejected", "Approved", "Applied", "Autoapproved", "Left"}
		}
	default:
		// General logs - just filter by group/user.
		types = nil
		subtypes = nil
	}

	// Build WHERE clauses.
	where := []string{"1=1"}
	args := []interface{}{}

	if groupid > 0 {
		where = append(where, "logs.groupid = ?")
		args = append(args, groupid)
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

		// Enrich user.
		if r.User != nil && *r.User > 0 {
			var displayname string
			db.Raw("SELECT COALESCE(fullname, CONCAT(COALESCE(firstname,''), ' ', COALESCE(lastname,'')), 'Unknown') FROM users WHERE id = ?", *r.User).Scan(&displayname)
			entry["user"] = map[string]interface{}{
				"id":          *r.User,
				"displayname": strings.TrimSpace(displayname),
			}
		}

		if r.Byuser != nil && *r.Byuser > 0 {
			var displayname string
			db.Raw("SELECT COALESCE(fullname, CONCAT(COALESCE(firstname,''), ' ', COALESCE(lastname,'')), 'Unknown') FROM users WHERE id = ?", *r.Byuser).Scan(&displayname)
			entry["byuser"] = map[string]interface{}{
				"id":          *r.Byuser,
				"displayname": strings.TrimSpace(displayname),
			}
		}

		// Enrich message.
		if r.Msgid != nil && *r.Msgid > 0 {
			var msg struct {
				Subject string
			}
			db.Raw("SELECT subject FROM messages WHERE id = ?", *r.Msgid).Scan(&msg)
			entry["message"] = map[string]interface{}{
				"id":      *r.Msgid,
				"subject": msg.Subject,
			}
		}

		result[i] = entry
	}

	// Build context for pagination.
	ctx := map[string]interface{}{}
	if len(rows) > 0 {
		ctx["id"] = rows[len(rows)-1].ID
	}

	return c.JSON(fiber.Map{
		"ret":     0,
		"status":  "Success",
		"logs":    result,
		"context": ctx,
	})
}
