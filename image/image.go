package image

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"strconv"
)

// imageTypeConfig maps imgtype names to their database table and parent ID column.
type imageTypeConfig struct {
	Table          string
	IDColumn       string
	HasContentType bool // All image tables except messages_attachments have a NOT NULL contenttype column.
}

var typeConfigs = map[string]imageTypeConfig{
	"Message":        {Table: "messages_attachments", IDColumn: "msgid", HasContentType: false},
	"Group":          {Table: "groups_images", IDColumn: "groupid", HasContentType: true},
	"Newsletter":     {Table: "newsletters_images", IDColumn: "articleid", HasContentType: true},
	"CommunityEvent": {Table: "communityevents_images", IDColumn: "eventid", HasContentType: true},
	"Volunteering":   {Table: "volunteering_images", IDColumn: "opportunityid", HasContentType: true},
	"ChatMessage":    {Table: "chat_images", IDColumn: "chatmsgid", HasContentType: true},
	"User":           {Table: "users_images", IDColumn: "userid", HasContentType: true},
	"Newsfeed":       {Table: "newsfeed_images", IDColumn: "newsfeedid", HasContentType: true},
	"Story":          {Table: "users_stories_images", IDColumn: "storyid", HasContentType: true},
	"Noticeboard":    {Table: "noticeboards_images", IDColumn: "noticeboardid", HasContentType: true},
}

// PostRequest handles all POST /image operations.
// The operation is determined by which fields are present:
// - externaluid present → create new attachment
// - id + rotate present → rotate existing attachment
type PostRequest struct {
	// Create fields
	ExternalUID  string          `json:"externaluid"`
	ExternalMods json.RawMessage `json:"externalmods"`
	Hash         string          `json:"hash"`

	// Rotate fields
	ID     uint64 `json:"id"`
	Rotate *int   `json:"rotate"`

	// Type resolution: either imgtype string or boolean flags
	ImgType        string `json:"imgtype"`
	Type           string `json:"type"` // Alternative field name for imgtype
	MsgID          uint64 `json:"msgid"`
	GroupID        uint64 `json:"groupid"`
	CommunityEvent any    `json:"communityevent"` // Can be uint64 (parent ID) or bool (type flag)
	Volunteering   any    `json:"volunteering"`   // Can be uint64 (parent ID) or bool (type flag)
	ChatMessage    uint64 `json:"chatmessage"`
	UserID         any    `json:"user"`       // Can be uint64 (parent ID) or bool (type flag)
	Newsfeed       uint64 `json:"newsfeed"`
	Story          any    `json:"story"`      // Can be bool (type flag)
	Noticeboard    any    `json:"noticeboard"` // Can be bool (type flag)
	Newsletter     uint64 `json:"newsletter"`
}

// resolveType determines the image type from the various possible request formats.
func (req *PostRequest) resolveType() string {
	if req.ImgType != "" {
		return req.ImgType
	}
	if req.Type != "" {
		return req.Type
	}
	// Check boolean flags used by rotation calls.
	if isTruthy(req.CommunityEvent) {
		return "CommunityEvent"
	}
	if isTruthy(req.Volunteering) {
		return "Volunteering"
	}
	if isTruthy(req.UserID) {
		return "User"
	}
	if isTruthy(req.Story) {
		return "Story"
	}
	if isTruthy(req.Noticeboard) {
		return "Noticeboard"
	}
	return "Message"
}

// resolveParentID determines the parent entity ID from the request.
func (req *PostRequest) resolveParentID() uint64 {
	switch req.resolveType() {
	case "Message":
		return req.MsgID
	case "Group":
		return req.GroupID
	case "Newsletter":
		return req.Newsletter
	case "CommunityEvent":
		return toUint64(req.CommunityEvent)
	case "Volunteering":
		return toUint64(req.Volunteering)
	case "ChatMessage":
		return req.ChatMessage
	case "User":
		return toUint64(req.UserID)
	case "Newsfeed":
		return req.Newsfeed
	case "Story":
		return toUint64(req.Story)
	case "Noticeboard":
		return toUint64(req.Noticeboard)
	default:
		return req.MsgID
	}
}

// Post handles POST /image for both creating attachments and rotating images.
//
// @Summary Create or update image attachment
// @Tags Image
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/image [post]
func Post(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req PostRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Determine operation: rotate if id + rotate present, otherwise create.
	if req.ID > 0 && req.Rotate != nil {
		return doRotate(c, &req)
	}

	return doCreate(c, &req)
}

func doCreate(c *fiber.Ctx, req *PostRequest) error {
	if req.ExternalUID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "externaluid is required")
	}

	imgType := req.resolveType()

	cfg, ok := typeConfigs[imgType]
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid imgtype: "+imgType)
	}

	parentID := req.resolveParentID()

	var modsStr *string
	if len(req.ExternalMods) > 0 && string(req.ExternalMods) != "null" {
		s := string(req.ExternalMods)
		modsStr = &s
	}

	db := database.DBConn

	var result *gorm.DB
	if cfg.HasContentType {
		result = db.Exec(
			"INSERT INTO `"+cfg.Table+"` (`"+cfg.IDColumn+"`, externaluid, externalmods, hash, contenttype) VALUES (?, ?, ?, ?, 'image/jpeg')",
			parentID, req.ExternalUID, modsStr, nilIfEmpty(req.Hash),
		)
	} else {
		result = db.Exec(
			"INSERT INTO `"+cfg.Table+"` (`"+cfg.IDColumn+"`, externaluid, externalmods, hash) VALUES (?, ?, ?, ?)",
			parentID, req.ExternalUID, modsStr, nilIfEmpty(req.Hash),
		)
	}

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create image attachment")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"id":     id,
		"uid":    req.ExternalUID,
	})
}

func doRotate(c *fiber.Ctx, req *PostRequest) error {
	imgType := req.resolveType()

	cfg, ok := typeConfigs[imgType]
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid imgtype")
	}

	modsJSON := `{"rotate":` + strconv.Itoa(*req.Rotate) + `}`

	db := database.DBConn
	result := db.Exec("UPDATE `"+cfg.Table+"` SET externalmods = ? WHERE id = ?", modsJSON, req.ID)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to rotate image")
	}

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
	})
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// isTruthy checks if a value from JSON is truthy (bool true or non-zero number).
func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val != "" && val != "0" && val != "false"
	}
	return false
}

// toUint64 extracts a uint64 from a value that might be a number or bool.
func toUint64(v any) uint64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return uint64(val)
	case bool:
		return 0
	}
	return 0
}
