package alert

import (
	"strconv"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type Alert struct {
	ID        uint64  `json:"id" gorm:"primary_key"`
	Createdby *uint64 `json:"createdby"`
	Groupid   *uint64 `json:"groupid"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Subject   string  `json:"subject"`
	Text      string  `json:"text"`
	Html      string  `json:"html"`
	Askclick  int     `json:"askclick"`
	Tryhard   int     `json:"tryhard"`
	Complete  string  `json:"complete"`
	Created   string  `json:"created"`
}

func (Alert) TableName() string {
	return "alerts"
}

type AlertStats struct {
	Responses []AlertResponseStat `json:"responses"`
	Reached   int64               `json:"reached"`
	Shown     int64               `json:"shown"`
	Clicked   int64               `json:"clicked"`
}

type AlertResponseStat struct {
	Response string `json:"response"`
	Count    int64  `json:"count"`
}

func isAdminOrSupport(myid uint64) bool {
	var role string
	database.DBConn.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
	return role == "Admin" || role == "Support"
}

// GetAlert handles GET /alert/:id - public access.
//
// @Summary Get alert by ID
// @Description Returns a single alert by ID. Admin/Support users also get tracking stats.
// @Tags alert
// @Produce json
// @Param id path integer true "Alert ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} fiber.Error "Alert not found"
// @Router /api/alert/{id} [get]
func GetAlert(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	db := database.DBConn

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
	}

	var a Alert
	db.Raw("SELECT id, createdby, groupid, `from`, `to`, subject, text, html, askclick, tryhard, complete, created FROM alerts WHERE id = ?", id).Scan(&a)

	if a.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Alert not found")
	}

	response := fiber.Map{
		"ret":    0,
		"status": "Success",
		"alert":  a,
	}

	// If the caller is admin/support, include tracking stats.
	if myid > 0 && isAdminOrSupport(myid) {
		var stats AlertStats

		// Get response counts.
		var responseCounts []AlertResponseStat
		db.Raw("SELECT response, COUNT(*) AS count FROM alerts_tracking WHERE alertid = ? AND response IS NOT NULL GROUP BY response", id).Scan(&responseCounts)
		if responseCounts == nil {
			responseCounts = make([]AlertResponseStat, 0)
		}
		stats.Responses = responseCounts

		// Get reached (total tracking entries).
		db.Raw("SELECT COUNT(*) FROM alerts_tracking WHERE alertid = ?", id).Scan(&stats.Reached)

		// Get shown count.
		db.Raw("SELECT COALESCE(SUM(shown), 0) FROM alerts_tracking WHERE alertid = ?", id).Scan(&stats.Shown)

		// Get clicked count.
		db.Raw("SELECT COALESCE(SUM(clicked), 0) FROM alerts_tracking WHERE alertid = ?", id).Scan(&stats.Clicked)

		// Merge stats into the alert map in the response.
		alertMap := response["alert"].(Alert)
		response["alert"] = fiber.Map{
			"id":        alertMap.ID,
			"createdby": alertMap.Createdby,
			"groupid":   alertMap.Groupid,
			"from":      alertMap.From,
			"to":        alertMap.To,
			"subject":   alertMap.Subject,
			"text":      alertMap.Text,
			"html":      alertMap.Html,
			"askclick":  alertMap.Askclick,
			"tryhard":   alertMap.Tryhard,
			"complete":  alertMap.Complete,
			"created":   alertMap.Created,
			"stats":     stats,
		}
	}

	return c.JSON(response)
}

// ListAlerts handles GET /alert - admin only.
//
// @Summary List all alerts
// @Description Returns all alerts ordered by creation date descending. Requires Admin or Support role.
// @Tags alert
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} fiber.Error "Not authorized"
// @Router /api/alert [get]
func ListAlerts(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isAdminOrSupport(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	db := database.DBConn

	var alerts []Alert
	db.Raw("SELECT id, createdby, groupid, `from`, `to`, subject, text, html, askclick, tryhard, complete, created FROM alerts ORDER BY created DESC").Scan(&alerts)

	if alerts == nil {
		alerts = make([]Alert, 0)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"alerts": alerts,
	})
}

// CreateAlert handles PUT /alert - admin only.
//
// @Summary Create a new alert
// @Description Creates a new alert. Requires Admin or Support role.
// @Tags alert
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} fiber.Error "Not logged in"
// @Failure 403 {object} fiber.Error "Not authorized"
// @Router /api/alert [put]
func CreateAlert(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isAdminOrSupport(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	type CreateRequest struct {
		From     string  `json:"from"`
		To       string  `json:"to"`
		Subject  string  `json:"subject"`
		Text     string  `json:"text"`
		Html     string  `json:"html"`
		Groupid  *uint64 `json:"groupid"`
		Askclick *int    `json:"askclick"`
		Tryhard  *int    `json:"tryhard"`
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Defaults.
	if req.To == "" {
		req.To = "Mods"
	}
	if req.Html == "" {
		req.Html = strings.ReplaceAll(req.Text, "\n", "<br>")
	}

	askclick := 1
	if req.Askclick != nil {
		askclick = *req.Askclick
	}
	tryhard := 1
	if req.Tryhard != nil {
		tryhard = *req.Tryhard
	}

	db := database.DBConn

	result := db.Exec("INSERT INTO alerts (createdby, groupid, `from`, `to`, subject, text, html, askclick, tryhard, created) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())",
		myid, req.Groupid, req.From, req.To, req.Subject, req.Text, req.Html, askclick, tryhard)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create alert")
	}

	var alertID uint64
	db.Raw("SELECT id FROM alerts WHERE createdby = ? ORDER BY id DESC LIMIT 1", myid).Scan(&alertID)

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"id":     alertID,
	})
}

// RecordAlert handles POST /alert - public access for tracking.
//
// @Summary Record alert click
// @Description Records a click on an alert tracking entry.
// @Tags alert
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/alert [post]
func RecordAlert(c *fiber.Ctx) error {
	type RecordRequest struct {
		Action  string `json:"action"`
		Trackid uint64 `json:"trackid"`
	}

	var req RecordRequest
	c.BodyParser(&req)

	if req.Action == "clicked" && req.Trackid > 0 {
		db := database.DBConn
		db.Exec("UPDATE alerts_tracking SET clicked = clicked + 1 WHERE id = ?", req.Trackid)
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}
