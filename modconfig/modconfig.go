package modconfig

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/freegle/iznik-server-go/auth"
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

func (ModConfig) TableName() string {
	return "mod_configs"
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

func (StdMsg) TableName() string {
	return "mod_stdmsgs"
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

// configColumns is the explicit column list for mod_configs queries.
const configColumns = "id, name, createdby, fromname, ccrejectto, ccrejectaddr, ccfollowupto, ccfollowupaddr, ccrejmembto, ccrejmembaddr, ccfollmembto, ccfollmembaddr, protected, messageorder, network, coloursubj, subjreg, subjlen, `default`, chatread"

// stdMsgColumns is the explicit column list for mod_stdmsgs queries.
const stdMsgColumns = "id, configid, title, action, subjpref, subjsuff, body, rarelyused, autosend, newmodstatus, newdelstatus, edittext, `insert`"

// GetModConfig handles GET /modconfig.
// With id param: returns single config with stdmsgs.
// Without id param: returns list of configs visible to the user.
//
// @Summary Get mod config(s)
// @Tags modconfig
// @Produce json
// @Param id query integer false "Config ID"
// @Param all query boolean false "Return all configs (admin only)"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/modconfig [get]
func GetModConfig(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)

	if id == 0 {
		return listModConfigs(c)
	}

	// Auth check required for single config fetch.
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn
	var cfg ModConfig
	db.Raw("SELECT "+configColumns+" FROM mod_configs WHERE id = ?", id).Scan(&cfg)
	if cfg.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Invalid config id")
	}

	// Verify the user can see this config.
	if !canSee(myid, &cfg) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorised")
	}

	// Get standard messages.
	var stdmsgs []StdMsg
	db.Raw("SELECT "+stdMsgColumns+" FROM mod_stdmsgs WHERE configid = ?", id).Scan(&stdmsgs)
	if stdmsgs == nil {
		stdmsgs = []StdMsg{}
	}

	// Compute "cansee" - why the user can see this config.
	var cansee string
	var sharedbyid uint64
	var sharedonid uint64

	if cfg.Createdby != nil && *cfg.Createdby == myid {
		cansee = "Created"
	} else if cfg.Default == 1 {
		cansee = "Default"
	} else {
		// Shared - find who is using it on a group we both moderate.
		type SharedInfo struct {
			Userid  uint64
			Groupid uint64
		}
		var shared SharedInfo
		db.Raw("SELECT m2.userid, m2.groupid "+
			"FROM memberships m1 "+
			"INNER JOIN memberships m2 ON m1.groupid = m2.groupid "+
			"WHERE m1.userid = ? AND m1.role IN ('Moderator', 'Owner') "+
			"AND m2.configid = ? AND m2.role IN ('Moderator', 'Owner') "+
			"AND m2.userid != ? "+
			"LIMIT 1", myid, cfg.ID, myid).Scan(&shared)

		if shared.Userid > 0 {
			cansee = "Shared"
			sharedbyid = shared.Userid
			sharedonid = shared.Groupid
		}
	}

	// Compute "using" - user IDs of moderators currently using this config.
	var usingUserIDs []uint64
	db.Raw("SELECT DISTINCT m.userid "+
		"FROM memberships m "+
		"WHERE m.configid = ? AND m.role IN ('Moderator', 'Owner') "+
		"LIMIT 10", cfg.ID).Pluck("userid", &usingUserIDs)

	if usingUserIDs == nil {
		usingUserIDs = []uint64{}
	}

	resp := fiber.Map{
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
		"using":          usingUserIDs,
	}

	if cansee != "" {
		resp["cansee"] = cansee
	}
	if sharedbyid > 0 {
		resp["sharedbyid"] = sharedbyid
	}
	if sharedonid > 0 {
		resp["sharedonid"] = sharedonid
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"config": resp,
	})
}

// listModConfigs returns all configs visible to the user.
func listModConfigs(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Check if admin/support requesting all configs.
	all := c.Query("all", "") == "true"

	var configs []ModConfig

	if all {
		// Admin/support can see all configs.  Non-admin users silently
		// fall through to the per-moderator query below (matching PHP behaviour).
		var role string
		db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&role)
		if role == "Admin" || role == "Support" {
			db.Raw("SELECT " + configColumns + " FROM mod_configs ORDER BY name").Scan(&configs)
		}
	}

	if configs == nil {
		// Return configs visible to this moderator:
		// 1. Created by them
		// 2. Default configs
		// 3. Used by groups they moderate
		//
		// Use UNION to avoid the expensive double LEFT JOIN on memberships
		// which caused full table scans on the 4.7M row memberships table.
		db.Raw("SELECT "+configColumns+" FROM mod_configs WHERE createdby = ? "+
			"UNION "+
			"SELECT "+configColumns+" FROM mod_configs WHERE `default` = 1 "+
			"UNION "+
			"SELECT "+configColumns+" FROM mod_configs WHERE id IN ("+
			"SELECT m1.configid FROM memberships m1 "+
			"WHERE m1.configid IS NOT NULL AND m1.role IN ('Moderator', 'Owner') "+
			"AND m1.groupid IN ("+
			"SELECT m2.groupid FROM memberships m2 "+
			"WHERE m2.userid = ? AND m2.role IN ('Moderator', 'Owner')"+
			")"+
			") "+
			"ORDER BY name", myid, myid).Scan(&configs)
	}

	if configs == nil {
		configs = []ModConfig{}
	}

	return c.JSON(configs)
}

// PostModConfig handles POST /modconfig to create a new config.
//
// @Summary Create mod config
// @Tags modconfig
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/modconfig [post]
func PostModConfig(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !auth.IsSystemMod(myid) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ret": 4, "status": "Don't have rights to create configs"})
	}

	type CreateRequest struct {
		Name string `json:"name"`
		ID   uint64 `json:"id"` // Copy from existing.
	}

	var req CreateRequest
	if c.Get("Content-Type") == "application/json" {
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
		}
	}
	if req.Name == "" {
		req.Name = c.FormValue("name", c.Query("name", ""))
	}

	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Must supply name")
	}

	db := database.DBConn

	if req.ID > 0 {
		// Verify the user can see the source config before copying.
		var srcCfg ModConfig
		db.Raw("SELECT "+configColumns+" FROM mod_configs WHERE id = ?", req.ID).Scan(&srcCfg)
		if srcCfg.ID == 0 {
			return fiber.NewError(fiber.StatusNotFound, "Source config not found")
		}
		if !canSee(myid, &srcCfg) {
			return fiber.NewError(fiber.StatusForbidden, "Not authorised to copy this config")
		}

		// Copy from existing config.
		result := db.Exec("INSERT INTO mod_configs (ccrejectto, ccrejectaddr, ccfollowupto, ccfollowupaddr, "+
			"ccrejmembto, ccrejmembaddr, ccfollmembto, ccfollmembaddr, network, coloursubj, subjlen) "+
			"SELECT ccrejectto, ccrejectaddr, ccfollowupto, ccfollowupaddr, "+
			"ccrejmembto, ccrejmembaddr, ccfollmembto, ccfollmembaddr, network, coloursubj, subjlen "+
			"FROM mod_configs WHERE id = ?", req.ID)
		if result.Error != nil {
			log.Printf("Failed to copy mod config %d: %v", req.ID, result.Error)
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to copy config")
		}

		// Use LAST_INSERT_ID() here because the INSERT ... SELECT doesn't populate
		// any unique field we can query by. This is safe as GORM reuses the same
		// connection for sequential calls.
		var newID uint64
		db.Raw("SELECT LAST_INSERT_ID()").Scan(&newID)
		if newID == 0 {
			return fiber.NewError(fiber.StatusInternalServerError, "Failed to get new config ID")
		}
		db.Exec("UPDATE mod_configs SET name = ?, createdby = ? WHERE id = ?", req.Name, myid, newID)

		// Copy stdmsgs.
		var srcMsgs []StdMsg
		db.Raw("SELECT "+stdMsgColumns+" FROM mod_stdmsgs WHERE configid = ?", req.ID).Scan(&srcMsgs)
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
		log.Printf("Failed to create mod config: %v", result.Error)
		return fiber.NewError(fiber.StatusInternalServerError, "Create failed")
	}

	var newID uint64
	db.Raw("SELECT id FROM mod_configs WHERE name = ? AND createdby = ? ORDER BY id DESC LIMIT 1", req.Name, myid).Scan(&newID)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "id": newID})
}

// PatchModConfig handles PATCH /modconfig to update settable attributes.
//
// @Summary Update mod config
// @Tags modconfig
// @Accept json
// @Produce json
// @Security BearerAuth
// @Router /api/modconfig [patch]
func PatchModConfig(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
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
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
		}
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseUint(c.FormValue("id", c.Query("id", "0")), 10, 64)
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid config id")
	}

	db := database.DBConn
	var cfg ModConfig
	db.Raw("SELECT "+configColumns+" FROM mod_configs WHERE id = ?", req.ID).Scan(&cfg)
	if cfg.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Invalid config id")
	}

	if !canModify(myid, &cfg) {
		return fiber.NewError(fiber.StatusForbidden, "Don't have rights to modify config")
	}

	// Build a single UPDATE with all changed fields.
	setClauses := []string{}
	args := []interface{}{}

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Fromname != nil {
		setClauses = append(setClauses, "fromname = ?")
		args = append(args, *req.Fromname)
	}
	if req.Ccrejectto != nil {
		setClauses = append(setClauses, "ccrejectto = ?")
		args = append(args, *req.Ccrejectto)
	}
	if req.Ccrejectaddr != nil {
		setClauses = append(setClauses, "ccrejectaddr = ?")
		args = append(args, *req.Ccrejectaddr)
	}
	if req.Ccfollowupto != nil {
		setClauses = append(setClauses, "ccfollowupto = ?")
		args = append(args, *req.Ccfollowupto)
	}
	if req.Ccfollowupaddr != nil {
		setClauses = append(setClauses, "ccfollowupaddr = ?")
		args = append(args, *req.Ccfollowupaddr)
	}
	if req.Ccrejmembto != nil {
		setClauses = append(setClauses, "ccrejmembto = ?")
		args = append(args, *req.Ccrejmembto)
	}
	if req.Ccrejmembaddr != nil {
		setClauses = append(setClauses, "ccrejmembaddr = ?")
		args = append(args, *req.Ccrejmembaddr)
	}
	if req.Ccfollmembto != nil {
		setClauses = append(setClauses, "ccfollmembto = ?")
		args = append(args, *req.Ccfollmembto)
	}
	if req.Ccfollmembaddr != nil {
		setClauses = append(setClauses, "ccfollmembaddr = ?")
		args = append(args, *req.Ccfollmembaddr)
	}
	if req.Protected != nil {
		setClauses = append(setClauses, "protected = ?")
		args = append(args, *req.Protected)
	}
	if req.Messageorder != nil {
		setClauses = append(setClauses, "messageorder = ?")
		args = append(args, *req.Messageorder)
	}
	if req.Network != nil {
		setClauses = append(setClauses, "network = ?")
		args = append(args, *req.Network)
	}
	if req.Coloursubj != nil {
		setClauses = append(setClauses, "coloursubj = ?")
		args = append(args, *req.Coloursubj)
	}
	if req.Subjreg != nil {
		setClauses = append(setClauses, "subjreg = ?")
		args = append(args, *req.Subjreg)
	}
	if req.Subjlen != nil {
		setClauses = append(setClauses, "subjlen = ?")
		args = append(args, *req.Subjlen)
	}
	if req.Chatread != nil {
		setClauses = append(setClauses, "chatread = ?")
		args = append(args, *req.Chatread)
	}

	if len(setClauses) > 0 {
		args = append(args, req.ID)
		query := fmt.Sprintf("UPDATE mod_configs SET %s WHERE id = ?", strings.Join(setClauses, ", "))
		if result := db.Exec(query, args...); result.Error != nil {
			log.Printf("Failed to update mod config %d: %v", req.ID, result.Error)
			return fiber.NewError(fiber.StatusInternalServerError, "Update failed")
		}
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteModConfig handles DELETE /modconfig.
//
// @Summary Delete mod config
// @Tags modconfig
// @Produce json
// @Param id query integer true "Config ID"
// @Security BearerAuth
// @Router /api/modconfig [delete]
func DeleteModConfig(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, _ := strconv.ParseUint(c.Query("id", "0"), 10, 64)
	if id == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid config id")
	}

	db := database.DBConn
	var cfg ModConfig
	db.Raw("SELECT "+configColumns+" FROM mod_configs WHERE id = ?", id).Scan(&cfg)
	if cfg.ID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Invalid config id")
	}

	if !canModify(myid, &cfg) {
		return fiber.NewError(fiber.StatusForbidden, "Don't have rights to modify config")
	}

	// Check if still in use.
	var inUse int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE configid = ? AND role IN ('Moderator', 'Owner')", id).Scan(&inUse)
	if inUse > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"ret": 5, "status": "Config still in use"})
	}

	db.Exec("DELETE FROM mod_configs WHERE id = ?", id)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
