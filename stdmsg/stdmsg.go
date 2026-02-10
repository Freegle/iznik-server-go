package stdmsg

import (
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type StdMsg struct {
	ID           uint64  `json:"id" gorm:"primary_key"`
	Configid     uint64  `json:"configid"`
	Title        string  `json:"title"`
	Action       string  `json:"action"`
	Subjpref     string  `json:"subjpref"`
	Subjsuff     string  `json:"subjsuff"`
	Body         string  `json:"body"`
	Rarelyused   int     `json:"rarelyused"`
	Autosend     int     `json:"autosend"`
	Newmodstatus string  `json:"newmodstatus"`
	Newdelstatus string  `json:"newdelstatus"`
	Edittext     string  `json:"edittext"`
	Insert       *string `json:"insert"`
}

// canModifyConfig checks if user can modify the parent config.
func canModifyConfig(myid uint64, configid uint64) bool {
	var role string
	database.DBConn.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
	if role == "Admin" || role == "Support" {
		return true
	}

	var createdby *uint64
	var protected int
	database.DBConn.Raw("SELECT createdby FROM mod_configs WHERE id = ?", configid).Scan(&createdby)
	database.DBConn.Raw("SELECT protected FROM mod_configs WHERE id = ?", configid).Scan(&protected)

	if createdby != nil && *createdby == myid {
		return true
	}
	if protected == 0 {
		return true
	}
	return false
}

// isModerator checks if user has moderator permissions.
func isModerator(myid uint64) bool {
	var role string
	database.DBConn.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
	if role == "Admin" || role == "Support" || role == "Moderator" {
		return true
	}
	var count int64
	database.DBConn.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&count)
	return count > 0
}

// GetStdMsg handles GET /stdmsg.
//
// @Summary Get standard message
// @Tags stdmsg
// @Produce json
// @Param id query integer true "StdMsg ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/stdmsg [get]
func GetStdMsg(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid stdmsg id"})
	}

	db := database.DBConn
	var msg StdMsg
	db.Raw("SELECT * FROM mod_stdmsgs WHERE id = ?", id).Scan(&msg)
	if msg.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid stdmsg id"})
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"stdmsg": msg,
	})
}

// PostStdMsg handles POST /stdmsg to create a new standard message.
//
// @Summary Create standard message
// @Tags stdmsg
// @Accept json
// @Produce json
// @Router /api/stdmsg [post]
func PostStdMsg(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	if !isModerator(myid) {
		return c.JSON(fiber.Map{"ret": 4, "status": "Don't have rights to create configs"})
	}

	type CreateRequest struct {
		Configid     uint64  `json:"configid"`
		Title        string  `json:"title"`
		Action       string  `json:"action"`
		Subjpref     string  `json:"subjpref"`
		Subjsuff     string  `json:"subjsuff"`
		Body         string  `json:"body"`
		Rarelyused   int     `json:"rarelyused"`
		Autosend     int     `json:"autosend"`
		Newmodstatus string  `json:"newmodstatus"`
		Newdelstatus string  `json:"newdelstatus"`
		Edittext     string  `json:"edittext"`
		Insert       *string `json:"insert"`
	}

	var req CreateRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.Title == "" {
		req.Title = c.FormValue("title", c.Query("title", ""))
	}
	if req.Configid == 0 {
		req.Configid, _ = strconv.ParseUint(c.FormValue("configid", c.Query("configid", "0")), 10, 64)
	}

	if req.Title == "" {
		return c.JSON(fiber.Map{"ret": 3, "status": "Must supply title"})
	}
	if req.Configid == 0 {
		return c.JSON(fiber.Map{"ret": 3, "status": "Must supply configid"})
	}

	db := database.DBConn
	result := db.Exec("INSERT INTO mod_stdmsgs (configid, title) VALUES (?, ?)", req.Configid, req.Title)
	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 1, "status": "Create failed"})
	}

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	// Apply optional attributes.
	if req.Action != "" {
		db.Exec("UPDATE mod_stdmsgs SET action = ? WHERE id = ?", req.Action, newID)
	}
	if req.Subjpref != "" {
		db.Exec("UPDATE mod_stdmsgs SET subjpref = ? WHERE id = ?", req.Subjpref, newID)
	}
	if req.Subjsuff != "" {
		db.Exec("UPDATE mod_stdmsgs SET subjsuff = ? WHERE id = ?", req.Subjsuff, newID)
	}
	if req.Body != "" {
		db.Exec("UPDATE mod_stdmsgs SET body = ? WHERE id = ?", req.Body, newID)
	}
	if req.Rarelyused != 0 {
		db.Exec("UPDATE mod_stdmsgs SET rarelyused = ? WHERE id = ?", req.Rarelyused, newID)
	}
	if req.Autosend != 0 {
		db.Exec("UPDATE mod_stdmsgs SET autosend = ? WHERE id = ?", req.Autosend, newID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchStdMsg handles PATCH /stdmsg to update attributes.
//
// @Summary Update standard message
// @Tags stdmsg
// @Accept json
// @Produce json
// @Router /api/stdmsg [patch]
func PatchStdMsg(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type PatchRequest struct {
		ID           uint64  `json:"id"`
		Title        *string `json:"title"`
		Action       *string `json:"action"`
		Subjpref     *string `json:"subjpref"`
		Subjsuff     *string `json:"subjsuff"`
		Body         *string `json:"body"`
		Rarelyused   *int    `json:"rarelyused"`
		Autosend     *int    `json:"autosend"`
		Newmodstatus *string `json:"newmodstatus"`
		Newdelstatus *string `json:"newdelstatus"`
		Edittext     *string `json:"edittext"`
		Insert       *string `json:"insert"`
	}

	var req PatchRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}

	if req.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid stdmsg id"})
	}

	db := database.DBConn

	// Get the stdmsg to find its configid.
	var configid uint64
	db.Raw("SELECT configid FROM mod_stdmsgs WHERE id = ?", req.ID).Scan(&configid)
	if configid == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid stdmsg id"})
	}

	if !canModifyConfig(myid, configid) {
		return c.JSON(fiber.Map{"ret": 4, "status": "Don't have rights to modify config"})
	}

	if req.Title != nil {
		db.Exec("UPDATE mod_stdmsgs SET title = ? WHERE id = ?", *req.Title, req.ID)
	}
	if req.Action != nil {
		db.Exec("UPDATE mod_stdmsgs SET action = ? WHERE id = ?", *req.Action, req.ID)
	}
	if req.Subjpref != nil {
		db.Exec("UPDATE mod_stdmsgs SET subjpref = ? WHERE id = ?", *req.Subjpref, req.ID)
	}
	if req.Subjsuff != nil {
		db.Exec("UPDATE mod_stdmsgs SET subjsuff = ? WHERE id = ?", *req.Subjsuff, req.ID)
	}
	if req.Body != nil {
		db.Exec("UPDATE mod_stdmsgs SET body = ? WHERE id = ?", *req.Body, req.ID)
	}
	if req.Rarelyused != nil {
		db.Exec("UPDATE mod_stdmsgs SET rarelyused = ? WHERE id = ?", *req.Rarelyused, req.ID)
	}
	if req.Autosend != nil {
		db.Exec("UPDATE mod_stdmsgs SET autosend = ? WHERE id = ?", *req.Autosend, req.ID)
	}
	if req.Newmodstatus != nil {
		db.Exec("UPDATE mod_stdmsgs SET newmodstatus = ? WHERE id = ?", *req.Newmodstatus, req.ID)
	}
	if req.Newdelstatus != nil {
		db.Exec("UPDATE mod_stdmsgs SET newdelstatus = ? WHERE id = ?", *req.Newdelstatus, req.ID)
	}
	if req.Edittext != nil {
		db.Exec("UPDATE mod_stdmsgs SET edittext = ? WHERE id = ?", *req.Edittext, req.ID)
	}
	if req.Insert != nil {
		db.Exec("UPDATE mod_stdmsgs SET `insert` = ? WHERE id = ?", *req.Insert, req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteStdMsg handles DELETE /stdmsg.
//
// @Summary Delete standard message
// @Tags stdmsg
// @Produce json
// @Param id query integer true "StdMsg ID"
// @Router /api/stdmsg [delete]
func DeleteStdMsg(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid stdmsg id"})
	}

	db := database.DBConn

	var configid uint64
	db.Raw("SELECT configid FROM mod_stdmsgs WHERE id = ?", id).Scan(&configid)
	if configid == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid stdmsg id"})
	}

	if !canModifyConfig(myid, configid) {
		return c.JSON(fiber.Map{"ret": 4, "status": "Don't have rights to modify config"})
	}

	db.Exec("DELETE FROM mod_stdmsgs WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
