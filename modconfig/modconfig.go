package modconfig

import (
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type ModConfig struct {
	ID             uint64  `json:"id" gorm:"primary_key"`
	Name           string  `json:"name"`
	Createdby      *uint64 `json:"createdby"`
	Fromname       string  `json:"fromname"`
	Ccrejectto     string  `json:"ccrejectto"`
	Ccrejectaddr   string  `json:"ccrejectaddr"`
	Ccfollowupto   string  `json:"ccfollowupto"`
	Ccfollowupaddr string  `json:"ccfollowupaddr"`
	Ccrejmembto    string  `json:"ccrejmembto"`
	Ccrejmembaddr  string  `json:"ccrejmembaddr"`
	Ccfollmembto   string  `json:"ccfollmembto"`
	Ccfollmembaddr string  `json:"ccfollmembaddr"`
	Protected      int     `json:"protected"`
	Messageorder   *string `json:"messageorder"`
	Network        string  `json:"network"`
	Coloursubj     int     `json:"coloursubj"`
	Subjreg        string  `json:"subjreg"`
	Subjlen        int     `json:"subjlen"`
	Default        int     `json:"default"`
	Chatread       int     `json:"chatread"`
}

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

// canModify checks if the user can modify a config.
func canModify(myid uint64, cfg *ModConfig) bool {
	var role string
	database.DBConn.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
	if role == "Admin" || role == "Support" {
		return true
	}
	// Moderator can modify if they created it or it's not protected.
	if cfg.Createdby != nil && *cfg.Createdby == myid {
		return true
	}
	if cfg.Protected == 0 {
		// Check if they can see it.
		return canSee(myid, cfg)
	}
	return false
}

// canSee checks if a moderator can see this config.
func canSee(myid uint64, cfg *ModConfig) bool {
	// Created by them.
	if cfg.Createdby != nil && *cfg.Createdby == myid {
		return true
	}
	// Default configs visible to all.
	if cfg.Default == 1 {
		return true
	}
	// Used by mods on groups they moderate.
	var count int64
	database.DBConn.Raw("SELECT COUNT(*) FROM memberships m1 "+
		"INNER JOIN memberships m2 ON m1.groupid = m2.groupid "+
		"WHERE m1.userid = ? AND m1.role IN ('Moderator', 'Owner') "+
		"AND m2.configid = ? AND m2.role IN ('Moderator', 'Owner')",
		myid, cfg.ID).Scan(&count)
	return count > 0
}

// isModerator checks if user has moderator system role or is mod on any group.
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

// GetModConfig handles GET /modconfig - single config with stdmsgs.
//
// @Summary Get mod config
// @Tags modconfig
// @Produce json
// @Param id query integer true "Config ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/modconfig [get]
func GetModConfig(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)

	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid config id"})
	}

	db := database.DBConn
	var cfg ModConfig
	db.Raw("SELECT * FROM mod_configs WHERE id = ?", id).Scan(&cfg)
	if cfg.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid config id"})
	}

	// Get standard messages.
	var stdmsgs []StdMsg
	db.Raw("SELECT * FROM mod_stdmsgs WHERE configid = ?", id).Scan(&stdmsgs)
	if stdmsgs == nil {
		stdmsgs = []StdMsg{}
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"config": fiber.Map{
			"id":             cfg.ID,
			"name":           cfg.Name,
			"createdby":      cfg.Createdby,
			"fromname":       cfg.Fromname,
			"ccrejectto":     cfg.Ccrejectto,
			"ccrejectaddr":   cfg.Ccrejectaddr,
			"ccfollowupto":   cfg.Ccfollowupto,
			"ccfollowupaddr": cfg.Ccfollowupaddr,
			"ccrejmembto":    cfg.Ccrejmembto,
			"ccrejmembaddr":  cfg.Ccrejmembaddr,
			"ccfollmembto":   cfg.Ccfollmembto,
			"ccfollmembaddr": cfg.Ccfollmembaddr,
			"protected":      cfg.Protected,
			"messageorder":   cfg.Messageorder,
			"network":        cfg.Network,
			"coloursubj":     cfg.Coloursubj,
			"subjreg":        cfg.Subjreg,
			"subjlen":        cfg.Subjlen,
			"default":        cfg.Default,
			"chatread":       cfg.Chatread,
			"stdmsgs":        stdmsgs,
		},
	})
}

// PostModConfig handles POST /modconfig to create a new config.
//
// @Summary Create mod config
// @Tags modconfig
// @Accept json
// @Produce json
// @Router /api/modconfig [post]
func PostModConfig(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	if !isModerator(myid) {
		return c.JSON(fiber.Map{"ret": 4, "status": "Don't have rights to create configs"})
	}

	type CreateRequest struct {
		Name string `json:"name"`
		ID   uint64 `json:"id"` // Copy from existing.
	}

	var req CreateRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.Name == "" {
		req.Name = c.FormValue("name", c.Query("name", ""))
	}

	if req.Name == "" {
		return c.JSON(fiber.Map{"ret": 3, "status": "Must supply name"})
	}

	db := database.DBConn

	if req.ID > 0 {
		// Copy from existing config.
		db.Exec("INSERT INTO mod_configs (ccrejectto, ccrejectaddr, ccfollowupto, ccfollowupaddr, "+
			"ccrejmembto, ccrejmembaddr, ccfollmembto, ccfollmembaddr, network, coloursubj, subjlen) "+
			"SELECT ccrejectto, ccrejectaddr, ccfollowupto, ccfollowupaddr, "+
			"ccrejmembto, ccrejmembaddr, ccfollmembto, ccfollmembaddr, network, coloursubj, subjlen "+
			"FROM mod_configs WHERE id = ?", req.ID)

		var newID uint64
		db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)
		db.Exec("UPDATE mod_configs SET name = ?, createdby = ? WHERE id = ?", req.Name, myid, newID)

		// Copy stdmsgs.
		var srcMsgs []StdMsg
		db.Raw("SELECT * FROM mod_stdmsgs WHERE configid = ?", req.ID).Scan(&srcMsgs)
		for _, m := range srcMsgs {
			db.Exec("INSERT INTO mod_stdmsgs (configid, title, action, subjpref, subjsuff, body, "+
				"rarelyused, autosend, newmodstatus, newdelstatus, edittext, `insert`) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				newID, m.Title, m.Action, m.Subjpref, m.Subjsuff, m.Body,
				m.Rarelyused, m.Autosend, m.Newmodstatus, m.Newdelstatus, m.Edittext, m.Insert)
		}

		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
	}

	// Simple create.
	result := db.Exec("INSERT INTO mod_configs (name, createdby) VALUES (?, ?)", req.Name, myid)
	if result.Error != nil {
		return c.JSON(fiber.Map{"ret": 1, "status": "Create failed"})
	}

	var newID uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchModConfig handles PATCH /modconfig to update settable attributes.
//
// @Summary Update mod config
// @Tags modconfig
// @Accept json
// @Produce json
// @Router /api/modconfig [patch]
func PatchModConfig(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	type PatchRequest struct {
		ID             uint64  `json:"id"`
		Name           *string `json:"name"`
		Fromname       *string `json:"fromname"`
		Ccrejectto     *string `json:"ccrejectto"`
		Ccrejectaddr   *string `json:"ccrejectaddr"`
		Ccfollowupto   *string `json:"ccfollowupto"`
		Ccfollowupaddr *string `json:"ccfollowupaddr"`
		Ccrejmembto    *string `json:"ccrejmembto"`
		Ccrejmembaddr  *string `json:"ccrejmembaddr"`
		Ccfollmembto   *string `json:"ccfollmembto"`
		Ccfollmembaddr *string `json:"ccfollmembaddr"`
		Protected      *int    `json:"protected"`
		Messageorder   *string `json:"messageorder"`
		Network        *string `json:"network"`
		Coloursubj     *int    `json:"coloursubj"`
		Subjreg        *string `json:"subjreg"`
		Subjlen        *int    `json:"subjlen"`
		Chatread       *int    `json:"chatread"`
	}

	var req PatchRequest
	if c.Get("Content-Type") == "application/json" {
		c.BodyParser(&req)
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}

	if req.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid config id"})
	}

	db := database.DBConn
	var cfg ModConfig
	db.Raw("SELECT * FROM mod_configs WHERE id = ?", req.ID).Scan(&cfg)
	if cfg.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid config id"})
	}

	if !canModify(myid, &cfg) {
		return c.JSON(fiber.Map{"ret": 4, "status": "Don't have rights to modify config"})
	}

	// Apply settable attributes.
	if req.Name != nil {
		db.Exec("UPDATE mod_configs SET name = ? WHERE id = ?", *req.Name, req.ID)
	}
	if req.Fromname != nil {
		db.Exec("UPDATE mod_configs SET fromname = ? WHERE id = ?", *req.Fromname, req.ID)
	}
	if req.Ccrejectto != nil {
		db.Exec("UPDATE mod_configs SET ccrejectto = ? WHERE id = ?", *req.Ccrejectto, req.ID)
	}
	if req.Ccrejectaddr != nil {
		db.Exec("UPDATE mod_configs SET ccrejectaddr = ? WHERE id = ?", *req.Ccrejectaddr, req.ID)
	}
	if req.Ccfollowupto != nil {
		db.Exec("UPDATE mod_configs SET ccfollowupto = ? WHERE id = ?", *req.Ccfollowupto, req.ID)
	}
	if req.Ccfollowupaddr != nil {
		db.Exec("UPDATE mod_configs SET ccfollowupaddr = ? WHERE id = ?", *req.Ccfollowupaddr, req.ID)
	}
	if req.Ccrejmembto != nil {
		db.Exec("UPDATE mod_configs SET ccrejmembto = ? WHERE id = ?", *req.Ccrejmembto, req.ID)
	}
	if req.Ccrejmembaddr != nil {
		db.Exec("UPDATE mod_configs SET ccrejmembaddr = ? WHERE id = ?", *req.Ccrejmembaddr, req.ID)
	}
	if req.Ccfollmembto != nil {
		db.Exec("UPDATE mod_configs SET ccfollmembto = ? WHERE id = ?", *req.Ccfollmembto, req.ID)
	}
	if req.Ccfollmembaddr != nil {
		db.Exec("UPDATE mod_configs SET ccfollmembaddr = ? WHERE id = ?", *req.Ccfollmembaddr, req.ID)
	}
	if req.Protected != nil {
		db.Exec("UPDATE mod_configs SET protected = ? WHERE id = ?", *req.Protected, req.ID)
	}
	if req.Messageorder != nil {
		db.Exec("UPDATE mod_configs SET messageorder = ? WHERE id = ?", *req.Messageorder, req.ID)
	}
	if req.Network != nil {
		db.Exec("UPDATE mod_configs SET network = ? WHERE id = ?", *req.Network, req.ID)
	}
	if req.Coloursubj != nil {
		db.Exec("UPDATE mod_configs SET coloursubj = ? WHERE id = ?", *req.Coloursubj, req.ID)
	}
	if req.Subjreg != nil {
		db.Exec("UPDATE mod_configs SET subjreg = ? WHERE id = ?", *req.Subjreg, req.ID)
	}
	if req.Subjlen != nil {
		db.Exec("UPDATE mod_configs SET subjlen = ? WHERE id = ?", *req.Subjlen, req.ID)
	}
	if req.Chatread != nil {
		db.Exec("UPDATE mod_configs SET chatread = ? WHERE id = ?", *req.Chatread, req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteModConfig handles DELETE /modconfig.
//
// @Summary Delete mod config
// @Tags modconfig
// @Produce json
// @Param id query integer true "Config ID"
// @Router /api/modconfig [delete]
func DeleteModConfig(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return c.JSON(fiber.Map{"ret": 1, "status": "Not logged in"})
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid config id"})
	}

	db := database.DBConn
	var cfg ModConfig
	db.Raw("SELECT * FROM mod_configs WHERE id = ?", id).Scan(&cfg)
	if cfg.ID == 0 {
		return c.JSON(fiber.Map{"ret": 2, "status": "Invalid config id"})
	}

	if !canModify(myid, &cfg) {
		return c.JSON(fiber.Map{"ret": 4, "status": "Don't have rights to modify config"})
	}

	// Check if still in use.
	var inUse int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE configid = ? AND role IN ('Moderator', 'Owner')", id).Scan(&inUse)
	if inUse > 0 {
		return c.JSON(fiber.Map{"ret": 5, "status": "Config still in use"})
	}

	db.Exec("DELETE FROM mod_configs WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
