package donations

import (
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// GiftAidListItem represents a gift aid record in the admin review list
type GiftAidListItem struct {
	GiftAid
	Email     *string `json:"email" gorm:"-"`
	Donations float64 `json:"donations" gorm:"column:donations"`
}

// SetGiftAidRequest is the request body for creating/updating a gift aid declaration
type SetGiftAidRequest struct {
	Period      string `json:"period"`
	Fullname    string `json:"fullname"`
	Homeaddress string `json:"homeaddress"`
}

// EditGiftAidRequest is the request body for admin editing of a gift aid record
type EditGiftAidRequest struct {
	ID                uint64 `json:"id"`
	Period            string `json:"period"`
	Fullname          string `json:"fullname"`
	Homeaddress       string `json:"homeaddress"`
	Postcode          string `json:"postcode"`
	Housenameornumber string `json:"housenameornumber"`
	Reviewed          *bool  `json:"reviewed"`
	Deleted           *bool  `json:"deleted"`
}

// isGiftAidAdmin checks if a user has admin/support role or PERM_GIFTAID permission
func isGiftAidAdmin(myid uint64) bool {
	db := database.DBConn

	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole == "Support" || systemrole == "Admin" {
		return true
	}

	var permissions *string
	db.Raw("SELECT permissions FROM users WHERE id = ?", myid).Scan(&permissions)

	if permissions != nil && strings.Contains(strings.ToLower(*permissions), "giftaid") {
		return true
	}

	return false
}

// ListGiftAid returns gift aid records needing review (admin only)
// Called when GET /giftaid has all=true query param
func ListGiftAid(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isGiftAidAdmin(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	db := database.DBConn

	var giftaids []GiftAidListItem
	db.Raw(`SELECT giftaid.*, SUM(users_donations.GrossAmount) AS donations
		FROM giftaid
		LEFT JOIN users_donations ON users_donations.userid = giftaid.userid
		WHERE giftaid.reviewed IS NULL AND giftaid.deleted IS NULL AND giftaid.period != 'Declined'
		GROUP BY giftaid.userid
		ORDER BY giftaid.timestamp DESC`).Scan(&giftaids)

	if giftaids == nil {
		giftaids = make([]GiftAidListItem, 0)
	}

	// Fetch emails for each user
	for i := range giftaids {
		var email *string
		db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", giftaids[i].UserID).Scan(&email)
		giftaids[i].Email = email
	}

	return c.JSON(fiber.Map{"giftaids": giftaids})
}

// SearchGiftAid searches gift aid records by name or address (admin only)
// Called when GET /giftaid has search=xxx query param
func SearchGiftAid(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isGiftAidAdmin(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	search := c.Query("search")
	if search == "" {
		return fiber.NewError(fiber.StatusBadRequest, "search is required")
	}

	db := database.DBConn
	searchPattern := "%" + search + "%"

	var giftaids []GiftAidListItem
	db.Raw("SELECT * FROM giftaid WHERE fullname LIKE ? OR homeaddress LIKE ? OR id LIKE ?",
		searchPattern, searchPattern, searchPattern).Scan(&giftaids)

	if giftaids == nil {
		giftaids = make([]GiftAidListItem, 0)
	}

	// Fetch emails for each user
	for i := range giftaids {
		var email *string
		db.Raw("SELECT email FROM users_emails WHERE userid = ? ORDER BY preferred DESC LIMIT 1", giftaids[i].UserID).Scan(&email)
		giftaids[i].Email = email
	}

	return c.JSON(fiber.Map{"giftaids": giftaids})
}

// SetGiftAid creates or updates the logged-in user's gift aid declaration
// @Summary Set Gift Aid declaration
// @Description Creates or updates the user's Gift Aid declaration
// @Tags donations
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Gift Aid ID"
// @Failure 400 {object} map[string]string "Bad parameters"
// @Failure 401 {object} map[string]string "Not logged in"
// @Router /giftaid [post]
func SetGiftAid(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req SetGiftAidRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Period == "" {
		return fiber.NewError(fiber.StatusBadRequest, "period is required")
	}

	// If not Declined, fullname and homeaddress are required
	if req.Period != "Declined" {
		if req.Fullname == "" || req.Homeaddress == "" {
			return fiber.NewError(fiber.StatusBadRequest, "fullname and homeaddress are required")
		}
	}

	db := database.DBConn

	result := db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id), period = ?, fullname = ?, homeaddress = ?, deleted = NULL`,
		myid, req.Period, req.Fullname, req.Homeaddress,
		req.Period, req.Fullname, req.Homeaddress)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to set gift aid")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return c.JSON(fiber.Map{"id": id})
}

// EditGiftAid allows an admin to edit a gift aid record
// @Summary Edit Gift Aid declaration (admin)
// @Description Admin edits a Gift Aid record
// @Tags donations
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Success"
// @Failure 401 {object} map[string]string "Not logged in"
// @Failure 403 {object} map[string]string "Not authorized"
// @Router /giftaid [patch]
func EditGiftAid(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isGiftAidAdmin(myid) {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	var req EditGiftAidRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	// Update each field individually if provided, matching PHP behavior
	if req.Period != "" {
		db.Exec("UPDATE giftaid SET period = ? WHERE id = ?", req.Period, req.ID)
	}
	if req.Fullname != "" {
		db.Exec("UPDATE giftaid SET fullname = ? WHERE id = ?", req.Fullname, req.ID)
	}
	if req.Homeaddress != "" {
		db.Exec("UPDATE giftaid SET homeaddress = ? WHERE id = ?", req.Homeaddress, req.ID)
	}
	if req.Postcode != "" {
		db.Exec("UPDATE giftaid SET postcode = ? WHERE id = ?", req.Postcode, req.ID)
	}
	if req.Housenameornumber != "" {
		db.Exec("UPDATE giftaid SET housenameornumber = ? WHERE id = ?", req.Housenameornumber, req.ID)
	}
	if req.Reviewed != nil && *req.Reviewed {
		db.Exec("UPDATE giftaid SET reviewed = NOW() WHERE id = ?", req.ID)
	}
	if req.Deleted != nil && *req.Deleted {
		db.Exec("UPDATE giftaid SET deleted = NOW() WHERE id = ?", req.ID)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}

// DeleteGiftAid soft-deletes the logged-in user's gift aid declaration
// @Summary Delete Gift Aid declaration
// @Description Soft-deletes the user's Gift Aid declaration by setting period to Declined and deleted to NOW()
// @Tags donations
// @Produce json
// @Success 200 {object} map[string]interface{} "Success"
// @Failure 401 {object} map[string]string "Not logged in"
// @Router /giftaid [delete]
func DeleteGiftAid(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn

	// Get user's name for the insert if record doesn't exist
	var fullname string
	db.Raw("SELECT COALESCE(fullname, '') FROM users WHERE id = ?", myid).Scan(&fullname)

	// Match PHP: INSERT ... ON DUPLICATE KEY UPDATE period = 'Declined', deleted = NOW()
	db.Exec(`INSERT INTO giftaid (userid, period, fullname, homeaddress)
		VALUES (?, 'Declined', ?, '')
		ON DUPLICATE KEY UPDATE period = 'Declined', deleted = NOW()`,
		myid, fullname)

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
