package simulation

import (
	"encoding/json"
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)


// GetSimulation handles GET /simulation and dispatches based on the 'action' query param.
//
// @Summary Get simulation data
// @Tags simulation
// @Produce json
// @Param action query string false "Action: listruns, getrun, or empty for message"
// @Param runid query integer false "Run ID (for getrun and default)"
// @Param index query integer false "Message index (for default action)"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/simulation [get]
func GetSimulation(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	if !user.IsModOfAnyGroup(myid) {
		return c.JSON(fiber.Map{"ret": 2, "status": "Permission denied"})
	}

	action := c.Query("action", "")

	switch action {
	case "listruns":
		return listRuns(c)
	case "getrun":
		return getRun(c)
	default:
		return getMessage(c)
	}
}

// listRuns returns completed simulation runs.
func listRuns(c *fiber.Ctx) error {
	db := database.DBConn

	type RunRow struct {
		ID           uint64 `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Created      string `json:"created"`
		Completed    string `json:"completed"`
		Parameters   string `json:"parameters"`
		Filters      string `json:"filters"`
		MessageCount int    `json:"message_count"`
		Metrics      string `json:"metrics"`
		Status       string `json:"status"`
	}

	var runs []RunRow
	db.Raw("SELECT id, name, description, created, completed, parameters, filters, message_count, metrics, status " +
		"FROM simulation_message_isochrones_runs WHERE status = 'completed' ORDER BY created DESC LIMIT 100").Scan(&runs)

	result := make([]map[string]interface{}, len(runs))
	for i, r := range runs {
		entry := map[string]interface{}{
			"id":            r.ID,
			"name":          r.Name,
			"description":   r.Description,
			"created":       r.Created,
			"completed":     r.Completed,
			"message_count": r.MessageCount,
			"status":        r.Status,
		}

		// Parse JSON columns.
		if r.Parameters != "" {
			var params interface{}
			if json.Unmarshal([]byte(r.Parameters), &params) == nil {
				entry["parameters"] = params
			}
		}
		if r.Filters != "" {
			var filters interface{}
			if json.Unmarshal([]byte(r.Filters), &filters) == nil {
				entry["filters"] = filters
			}
		}
		if r.Metrics != "" {
			var metrics interface{}
			if json.Unmarshal([]byte(r.Metrics), &metrics) == nil {
				entry["metrics"] = metrics
			}
		}

		result[i] = entry
	}

	if result == nil {
		result = make([]map[string]interface{}, 0)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"runs":   result,
	})
}

// getRun returns a single simulation run by ID.
func getRun(c *fiber.Ctx) error {
	runID, _ := strconv.ParseUint(c.Query("runid", "0"), 10, 64)
	if runID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing runid"})
	}

	db := database.DBConn

	type RunRow struct {
		ID           uint64 `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		Created      string `json:"created"`
		Completed    string `json:"completed"`
		Parameters   string `json:"parameters"`
		Filters      string `json:"filters"`
		MessageCount int    `json:"message_count"`
		Metrics      string `json:"metrics"`
		Status       string `json:"status"`
	}

	var r RunRow
	db.Raw("SELECT id, name, description, created, completed, parameters, filters, message_count, metrics, status "+
		"FROM simulation_message_isochrones_runs WHERE id = ?", runID).Scan(&r)

	if r.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Not found"})
	}

	entry := map[string]interface{}{
		"id":            r.ID,
		"name":          r.Name,
		"description":   r.Description,
		"created":       r.Created,
		"completed":     r.Completed,
		"message_count": r.MessageCount,
		"status":        r.Status,
	}

	if r.Parameters != "" {
		var params interface{}
		if json.Unmarshal([]byte(r.Parameters), &params) == nil {
			entry["parameters"] = params
		}
	}
	if r.Filters != "" {
		var filters interface{}
		if json.Unmarshal([]byte(r.Filters), &filters) == nil {
			entry["filters"] = filters
		}
	}
	if r.Metrics != "" {
		var metrics interface{}
		if json.Unmarshal([]byte(r.Metrics), &metrics) == nil {
			entry["metrics"] = metrics
		}
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"run":    entry,
	})
}

// getMessage returns a simulation message at a specific index in a run.
func getMessage(c *fiber.Ctx) error {
	runID, _ := strconv.ParseUint(c.Query("runid", "0"), 10, 64)
	index, _ := strconv.ParseUint(c.Query("index", "0"), 10, 64)

	if runID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Missing runid"})
	}

	db := database.DBConn

	// Get total messages in the run.
	var total int64
	db.Raw("SELECT COUNT(*) FROM simulation_message_isochrones_messages WHERE runid = ?", runID).Scan(&total)

	// Get the message at this sequence.
	type MessageRow struct {
		ID       uint64  `json:"id"`
		RunID    uint64  `json:"runid"`
		Sequence uint64  `json:"sequence"`
		MsgID    *uint64 `json:"msgid"`
		Subject  string  `json:"subject"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
		Groupid  *uint64 `json:"groupid"`
	}

	var msg MessageRow
	db.Raw("SELECT id, runid, sequence, msgid, subject, lat, lng, groupid "+
		"FROM simulation_message_isochrones_messages WHERE runid = ? AND sequence = ?",
		runID, index).Scan(&msg)

	if msg.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Message not found"})
	}

	// Get expansions for this message.
	type ExpansionRow struct {
		ID       uint64  `json:"id"`
		SimMsgID uint64  `json:"sim_msgid"`
		Sequence uint64  `json:"sequence"`
		Minutes  int     `json:"minutes"`
		Users    int     `json:"users"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
	}

	var expansions []ExpansionRow
	db.Raw("SELECT id, sim_msgid, sequence, minutes, users, lat, lng "+
		"FROM simulation_message_isochrones_expansions WHERE sim_msgid = ? ORDER BY sequence ASC",
		msg.ID).Scan(&expansions)

	if expansions == nil {
		expansions = make([]ExpansionRow, 0)
	}

	// Get users for this message.
	type UserRow struct {
		ID       uint64  `json:"id"`
		SimMsgID uint64  `json:"sim_msgid"`
		Userid   uint64  `json:"userid"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
	}

	var users []UserRow
	db.Raw("SELECT id, sim_msgid, userid, lat, lng "+
		"FROM simulation_message_isochrones_users WHERE sim_msgid = ?",
		msg.ID).Scan(&users)

	if users == nil {
		users = make([]UserRow, 0)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"message": fiber.Map{
			"id":       msg.ID,
			"runid":    msg.RunID,
			"sequence": msg.Sequence,
			"msgid":    msg.MsgID,
			"subject":  msg.Subject,
			"lat":      msg.Lat,
			"lng":      msg.Lng,
			"groupid":  msg.Groupid,
		},
		"expansions": expansions,
		"users":      users,
		"navigation": fiber.Map{
			"current_index": index,
			"total":         total,
			"has_next":      int64(index+1) < total,
			"has_prev":      index > 0,
		},
	})
}
