package modtools

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"strconv"
)

type ModConfig struct {
	ID             uint64  `json:"id"`
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
	Protected      bool    `json:"protected"`
	Messageorder   *string `json:"messageorder"`
	Network        string  `json:"network"`
	Coloursubj     bool    `json:"coloursubj"`
	Subjreg        string  `json:"subjreg"`
	Subjlen        int     `json:"subjlen"`
	Default        bool    `json:"default"`
	Chatread       bool    `json:"chatread"`
}

// GetModConfig handles GET /modtools/modconfig.
// When ?all=true, returns all configs visible to the user.
// When ?id=N, returns a single config with its stdmsgs.
func GetModConfig(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	idStr := c.Query("id", "0")
	id, _ := strconv.ParseUint(idStr, 10, 64)

	if id > 0 {
		// Return a single config.
		var config ModConfig
		db.Raw("SELECT * FROM mod_configs WHERE id = ?", id).Scan(&config)
		if config.ID == 0 {
			return fiber.NewError(fiber.StatusNotFound, "Config not found")
		}

		// Fetch standard messages for this config.
		var stdmsgs []StdMsg
		db.Raw("SELECT * FROM mod_stdmsgs WHERE configid = ? ORDER BY id", id).Scan(&stdmsgs)

		return c.JSON(fiber.Map{
			"ret":    0,
			"status": "Success",
			"config": fiber.Map{
				"id":             config.ID,
				"name":           config.Name,
				"createdby":      config.Createdby,
				"fromname":       config.Fromname,
				"ccrejectto":     config.Ccrejectto,
				"ccrejectaddr":   config.Ccrejectaddr,
				"ccfollowupto":   config.Ccfollowupto,
				"ccfollowupaddr": config.Ccfollowupaddr,
				"ccrejmembto":    config.Ccrejmembto,
				"ccrejmembaddr":  config.Ccrejmembaddr,
				"ccfollmembto":   config.Ccfollmembto,
				"ccfollmembaddr": config.Ccfollmembaddr,
				"protected":      config.Protected,
				"messageorder":   config.Messageorder,
				"network":        config.Network,
				"coloursubj":     config.Coloursubj,
				"subjreg":        config.Subjreg,
				"subjlen":        config.Subjlen,
				"default":        config.Default,
				"chatread":       config.Chatread,
				"stdmsgs":        stdmsgs,
			},
		})
	}

	// Return all configs visible to this user:
	// - configs they created
	// - default configs
	// - configs used in their memberships
	var configs []ModConfig
	db.Raw("SELECT DISTINCT mc.* FROM mod_configs mc "+
		"LEFT JOIN memberships m ON m.configid = mc.id AND m.userid = ? "+
		"WHERE mc.createdby = ? OR mc.`default` = 1 OR m.id IS NOT NULL "+
		"ORDER BY mc.name", myid, myid).Scan(&configs)

	return c.JSON(configs)
}

type StdMsg struct {
	ID        uint64  `json:"id"`
	Configid  uint64  `json:"configid"`
	Title     string  `json:"title"`
	Action    string  `json:"action"`
	Body      string  `json:"body"`
	Subjpref  string  `json:"subjpref"`
	Subjsuff  string  `json:"subjsuff"`
	Newsubj   *string `json:"newsubj"`
	Newdelstatus *string `json:"newdelstatus"`
	Edittext  *string `json:"edittext"`
	Rarelyused bool   `json:"rarelyused"`
	Autosend  bool    `json:"autosend"`
	Insert    string  `json:"insert"`
}
