package dashboard

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// GetDashboard handles GET /dashboard with component-based or legacy response.
//
// @Summary Get dashboard data
// @Description Returns dashboard components for moderator/user dashboards
// @Tags dashboard
// @Produce json
// @Param components query string false "Comma-separated component names"
// @Param group query integer false "Group ID"
// @Param systemwide query boolean false "System-wide data"
// @Param allgroups query boolean false "All moderator groups"
// @Param start query string false "Start date (default: 30 days ago)"
// @Param end query string false "End date (default: today)"
// @Success 200 {object} map[string]interface{}
// @Router /api/dashboard [get]
func GetDashboard(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	db := database.DBConn

	// Parse date range.
	startStr := c.Query("start", "30 days ago")
	endStr := c.Query("end", "today")
	startDate := parseRelativeDate(startStr)
	endDate := parseRelativeDate(endStr)
	startQ := startDate.Format("2006-01-02")
	endQ := endDate.Format("2006-01-02")

	// Determine group scope.
	groupID := c.QueryInt("group", 0)
	systemwide := c.Query("systemwide") == "true" || c.Query("systemwide") == "1"
	allgroups := c.Query("allgroups") == "true" || c.Query("allgroups") == "1"

	groupIDs := resolveGroupIDs(myid, uint64(groupID), systemwide, allgroups)

	// Check if user is a moderator (for mod-only components).
	isMod := false
	if myid > 0 && len(groupIDs) > 0 {
		var modCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner') AND groupid IN (?)",
			myid, groupIDs).Scan(&modCount)
		isMod = modCount > 0
	}

	// Component-based (new style).
	components := c.Query("components", "")
	if components != "" {
		result := make(map[string]interface{})
		for _, comp := range strings.Split(components, ",") {
			comp = strings.TrimSpace(comp)
			result[comp] = getComponent(comp, groupIDs, startQ, endQ, systemwide, isMod)
		}
		return c.JSON(fiber.Map{
			"ret":        0,
			"status":     "Success",
			"components": result,
			"start":      startStr,
			"end":        endStr,
		})
	}

	// Legacy style - return basic dashboard.
	dashboard := make(map[string]interface{})
	dashboard["newmembers"] = 0
	dashboard["newmessages"] = 0

	if len(groupIDs) > 0 {
		var msgCount int64
		db.Raw("SELECT COUNT(*) FROM messages INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
			"WHERE messages_groups.arrival >= ? AND messages_groups.arrival <= ? AND groupid IN (?)",
			startQ, endQ, groupIDs).Scan(&msgCount)
		dashboard["newmessages"] = msgCount

		var memCount int64
		db.Raw("SELECT COUNT(*) FROM memberships WHERE groupid IN (?) AND added >= ? AND added <= ?",
			groupIDs, startQ, endQ).Scan(&memCount)
		dashboard["newmembers"] = memCount
	}

	return c.JSON(fiber.Map{
		"ret":       0,
		"status":    "Success",
		"dashboard": dashboard,
		"start":     startStr,
		"end":       endStr,
	})
}

func getComponent(comp string, groupIDs []uint64, startQ, endQ string, systemwide, isMod bool) interface{} {
	switch comp {
	case "RecentCounts":
		return getRecentCounts(groupIDs, startQ, endQ)
	case "PopularPosts":
		return getPopularPosts(groupIDs, startQ, endQ)
	case "UsersPosting":
		if !isMod {
			return nil
		}
		return getUsersPosting(groupIDs, startQ, endQ)
	case "UsersReplying":
		if !isMod {
			return nil
		}
		return getUsersReplying(groupIDs, startQ, endQ)
	case "ModeratorsActive":
		if !isMod {
			return nil
		}
		return getModeratorsActive(groupIDs)
	case "Activity", "Replies", "ApprovedMessageCount", "MessageBreakdown",
		"Weight", "Outcomes", "ActiveUsers", "ApprovedMemberCount":
		modOnly := comp == "ActiveUsers" || comp == "ApprovedMemberCount"
		if modOnly && !isMod {
			return nil
		}
		return getStatsTimeSeries(comp, groupIDs, startQ, endQ)
	case "Donations":
		return getDonations(groupIDs, startQ, endQ, systemwide)
	case "Happiness":
		if !isMod {
			return nil
		}
		return getHappiness(groupIDs, startQ, endQ, systemwide)
	case "DiscourseTopics":
		if !isMod {
			return nil
		}
		return getDiscourseTopics()
	}
	return nil
}

func getRecentCounts(groupIDs []uint64, startQ, endQ string) map[string]int64 {
	db := database.DBConn
	result := map[string]int64{"newmembers": 0, "newmessages": 0}
	if len(groupIDs) == 0 {
		return result
	}

	db.Raw("SELECT COUNT(*) FROM messages INNER JOIN messages_groups ON messages_groups.msgid = messages.id "+
		"WHERE messages_groups.arrival >= ? AND messages_groups.arrival <= ? AND groupid IN (?) "+
		"AND messages.arrival >= ? AND messages.arrival <= ?",
		startQ, endQ, groupIDs, startQ, endQ).Scan(&result["newmessages"])

	db.Raw("SELECT COUNT(*) FROM memberships WHERE groupid IN (?) AND added >= ? AND added <= ?",
		groupIDs, startQ, endQ).Scan(&result["newmembers"])

	return result
}

func getPopularPosts(groupIDs []uint64, startQ, endQ string) []map[string]interface{} {
	db := database.DBConn
	if len(groupIDs) == 0 {
		return []map[string]interface{}{}
	}

	type PostRow struct {
		Views   int
		ID      uint64
		Subject string
	}

	var posts []PostRow
	db.Raw("SELECT COUNT(*) AS views, messages.id, messages.subject "+
		"FROM messages "+
		"INNER JOIN messages_likes ON messages_likes.msgid = messages.id "+
		"INNER JOIN messages_groups ON messages_groups.msgid = messages.id AND messages_groups.collection = 'Approved' "+
		"WHERE messages_groups.arrival >= ? AND messages_groups.arrival <= ? AND groupid IN (?) "+
		"AND messages_likes.type = 'View' "+
		"GROUP BY messages.id HAVING views > 0 ORDER BY views DESC LIMIT 5",
		startQ, endQ, groupIDs).Scan(&posts)

	userSite := os.Getenv("USER_SITE")
	if userSite == "" {
		userSite = "www.ilovefreegle.org"
	}

	result := make([]map[string]interface{}, len(posts))
	for i, p := range posts {
		// Get reply count.
		var replies int
		db.Raw("SELECT COUNT(*) FROM chat_messages WHERE refmsgid = ?", p.ID).Scan(&replies)

		result[i] = map[string]interface{}{
			"views":   p.Views,
			"id":      p.ID,
			"subject": p.Subject,
			"replies": replies,
			"url":     fmt.Sprintf("https://%s/message/%d", userSite, p.ID),
		}
	}
	return result
}

func getUsersPosting(groupIDs []uint64, startQ, endQ string) []map[string]interface{} {
	db := database.DBConn
	if len(groupIDs) == 0 {
		return []map[string]interface{}{}
	}

	type UserCount struct {
		Count    int
		Fromuser uint64
	}

	var users []UserCount
	db.Raw("SELECT COUNT(*) AS count, messages.fromuser "+
		"FROM messages WHERE id IN (SELECT msgid FROM messages_groups "+
		"WHERE messages_groups.arrival >= ? AND messages_groups.arrival <= ? AND groupid IN (?)) "+
		"AND messages.arrival >= ? AND messages.arrival <= ? "+
		"GROUP BY messages.fromuser ORDER BY count DESC LIMIT 5",
		startQ, endQ, groupIDs, startQ, endQ).Scan(&users)

	result := make([]map[string]interface{}, len(users))
	for i, u := range users {
		var displayname string
		db.Raw("SELECT COALESCE(fullname, firstname, lastname, 'Unknown') FROM users WHERE id = ?", u.Fromuser).Scan(&displayname)
		result[i] = map[string]interface{}{
			"id":          u.Fromuser,
			"displayname": displayname,
			"posts":       u.Count,
		}
	}
	return result
}

func getUsersReplying(groupIDs []uint64, startQ, endQ string) []map[string]interface{} {
	db := database.DBConn
	if len(groupIDs) == 0 {
		return []map[string]interface{}{}
	}

	type UserCount struct {
		Count  int
		Userid uint64
	}

	var users []UserCount
	db.Raw("SELECT COUNT(*) AS count, chat_messages.userid "+
		"FROM chat_messages "+
		"INNER JOIN messages_groups ON messages_groups.msgid = chat_messages.refmsgid "+
		"WHERE messages_groups.arrival >= ? AND messages_groups.arrival <= ? AND groupid IN (?) "+
		"AND chat_messages.type = 'Interested' "+
		"GROUP BY chat_messages.userid ORDER BY count DESC LIMIT 5",
		startQ, endQ, groupIDs).Scan(&users)

	result := make([]map[string]interface{}, len(users))
	for i, u := range users {
		var displayname string
		db.Raw("SELECT COALESCE(fullname, firstname, lastname, 'Unknown') FROM users WHERE id = ?", u.Userid).Scan(&displayname)
		result[i] = map[string]interface{}{
			"id":          u.Userid,
			"displayname": displayname,
			"replies":     u.Count,
		}
	}
	return result
}

func getModeratorsActive(groupIDs []uint64) []map[string]interface{} {
	db := database.DBConn
	if len(groupIDs) == 0 {
		return []map[string]interface{}{}
	}

	type ModRow struct {
		Userid     uint64
		Lastactive *string
	}

	var mods []ModRow
	db.Raw("SELECT userid, "+
		"(SELECT messages_groups.approvedat FROM messages_groups "+
		"WHERE messages_groups.approvedby = memberships.userid AND messages_groups.groupid = memberships.groupid "+
		"ORDER BY messages_groups.approvedat DESC LIMIT 1) AS lastactive "+
		"FROM memberships WHERE groupid IN (?) AND role IN ('Moderator', 'Owner') HAVING lastactive IS NOT NULL",
		groupIDs).Scan(&mods)

	result := make([]map[string]interface{}, 0, len(mods))
	for _, m := range mods {
		var displayname string
		db.Raw("SELECT COALESCE(fullname, firstname, lastname, 'Unknown') FROM users WHERE id = ?", m.Userid).Scan(&displayname)
		entry := map[string]interface{}{
			"id":          m.Userid,
			"displayname": displayname,
		}
		if m.Lastactive != nil {
			entry["lastactive"] = *m.Lastactive
		}
		result = append(result, entry)
	}
	return result
}

// getStatsTimeSeries reads from the pre-computed stats table.
func getStatsTimeSeries(component string, groupIDs []uint64, startQ, endQ string) []map[string]interface{} {
	db := database.DBConn
	if len(groupIDs) == 0 {
		return []map[string]interface{}{}
	}

	// Map component names to stats table types.
	statsType := component
	switch component {
	case "Activity":
		statsType = "Activity"
	case "Replies":
		statsType = "Replies"
	case "ApprovedMessageCount":
		statsType = "ApprovedMessageCount"
	case "MessageBreakdown":
		statsType = "MessageBreakdown"
	case "Weight":
		statsType = "Weight"
	case "Outcomes":
		statsType = "Outcomes"
	case "ActiveUsers":
		statsType = "ActiveUsers"
	case "ApprovedMemberCount":
		statsType = "ApprovedMemberCount"
	}

	type StatsRow struct {
		Date      string
		Count     *int64
		Breakdown *string
	}

	var rows []StatsRow
	db.Raw("SELECT date, SUM(count) AS count, breakdown FROM stats "+
		"WHERE type = ? AND groupid IN (?) AND date >= ? AND date <= ? "+
		"GROUP BY date ORDER BY date ASC",
		statsType, groupIDs, startQ, endQ).Scan(&rows)

	result := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		entry := map[string]interface{}{
			"date": r.Date,
		}
		if r.Count != nil {
			entry["count"] = *r.Count
		} else {
			entry["count"] = 0
		}
		result[i] = entry
	}
	return result
}

func getDonations(groupIDs []uint64, startQ, endQ string, systemwide bool) []map[string]interface{} {
	db := database.DBConn

	type DonRow struct {
		Count float64
		Date  string
	}

	var rows []DonRow
	if systemwide {
		db.Raw("SELECT SUM(GrossAmount) AS count, DATE(timestamp) AS date "+
			"FROM users_donations WHERE timestamp >= ? AND timestamp <= ? "+
			"GROUP BY date ORDER BY date ASC", startQ, endQ).Scan(&rows)
	} else if len(groupIDs) > 0 {
		db.Raw("SELECT SUM(GrossAmount) AS count, DATE(timestamp) AS date "+
			"FROM users_donations WHERE userid IN (SELECT DISTINCT userid FROM memberships WHERE groupid IN (?)) "+
			"AND timestamp >= ? AND timestamp <= ? "+
			"GROUP BY date ORDER BY date ASC", groupIDs, startQ, endQ).Scan(&rows)
	}

	result := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		result[i] = map[string]interface{}{
			"count": r.Count,
			"date":  r.Date,
		}
	}
	return result
}

func getHappiness(groupIDs []uint64, startQ, endQ string, systemwide bool) []map[string]interface{} {
	db := database.DBConn

	type HappyRow struct {
		Count     int
		Happiness string
	}

	var rows []HappyRow
	if systemwide {
		db.Raw("SELECT COUNT(*) AS count, happiness FROM messages_outcomes "+
			"WHERE timestamp >= ? AND timestamp <= ? AND happiness IS NOT NULL "+
			"GROUP BY happiness ORDER BY count DESC",
			startQ, endQ).Scan(&rows)
	} else if len(groupIDs) > 0 {
		db.Raw("SELECT COUNT(*) AS count, happiness FROM messages_outcomes "+
			"INNER JOIN messages ON messages.id = messages_outcomes.msgid "+
			"INNER JOIN messages_groups ON messages_groups.msgid = messages_outcomes.msgid "+
			"WHERE timestamp >= ? AND timestamp <= ? AND messages_groups.groupid IN (?) "+
			"AND happiness IS NOT NULL GROUP BY happiness ORDER BY count DESC",
			startQ, endQ, groupIDs).Scan(&rows)
	}

	result := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		result[i] = map[string]interface{}{
			"count":     r.Count,
			"happiness": r.Happiness,
		}
	}
	return result
}

func getDiscourseTopics() interface{} {
	// Discourse integration requires API key configuration.
	// Return nil if not configured.
	return nil
}

// resolveGroupIDs determines which groups to query based on parameters.
func resolveGroupIDs(myid uint64, groupID uint64, systemwide, allgroups bool) []uint64 {
	var groupIDs []uint64

	if groupID > 0 {
		groupIDs = []uint64{groupID}
	} else if systemwide {
		database.DBConn.Raw("SELECT id FROM `groups` WHERE publish = 1 AND onhere = 1").Scan(&groupIDs)
	} else if allgroups && myid > 0 {
		database.DBConn.Raw("SELECT groupid FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&groupIDs)
	}
	return groupIDs
}

func parseRelativeDate(s string) time.Time {
	switch s {
	case "today":
		return time.Now()
	case "30 days ago":
		return time.Now().AddDate(0, 0, -30)
	case "7 days ago":
		return time.Now().AddDate(0, 0, -7)
	case "90 days ago":
		return time.Now().AddDate(0, 0, -90)
	case "1 year ago":
		return time.Now().AddDate(-1, 0, 0)
	default:
		// Try parsing as a date.
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			t, err = time.Parse(time.RFC3339, s)
			if err != nil {
				return time.Now().AddDate(0, 0, -30)
			}
		}
		return t
	}
}

