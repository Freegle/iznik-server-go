package address

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"strconv"
)

type CreateRequest struct {
	PafID        uint64  `json:"pafid"`
	Instructions string  `json:"instructions"`
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
}

type UpdateRequest struct {
	ID           uint64   `json:"id"`
	PafID        *uint64  `json:"pafid,omitempty"`
	Instructions *string  `json:"instructions,omitempty"`
	Lat          *float64 `json:"lat,omitempty"`
	Lng          *float64 `json:"lng,omitempty"`
}

func Create(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.PafID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "pafid is required")
	}

	db := database.DBConn

	// Use REPLACE INTO to match PHP behavior - if (userid, pafid) already exists, it replaces.
	result := db.Exec("REPLACE INTO users_addresses (userid, pafid, instructions, lat, lng) VALUES (?, ?, ?, ?, ?)",
		myid, req.PafID, req.Instructions, req.Lat, req.Lng)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create address")
	}

	// Get the ID of the inserted/replaced row
	var id uint64
	db.Raw("SELECT id FROM users_addresses WHERE userid = ? AND pafid = ?", myid, req.PafID).Scan(&id)

	return c.JSON(fiber.Map{"id": id})
}

func Update(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	// Check ownership
	var ownerID uint64
	db.Raw("SELECT userid FROM users_addresses WHERE id = ?", req.ID).Scan(&ownerID)

	if ownerID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Address not found")
	}

	if ownerID != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your address")
	}

	// Update settable attributes
	if req.Instructions != nil {
		db.Exec("UPDATE users_addresses SET instructions = ? WHERE id = ?", *req.Instructions, req.ID)
	}
	if req.Lat != nil {
		db.Exec("UPDATE users_addresses SET lat = ? WHERE id = ?", *req.Lat, req.ID)
	}
	if req.Lng != nil {
		db.Exec("UPDATE users_addresses SET lng = ? WHERE id = ?", *req.Lng, req.ID)
	}
	if req.PafID != nil {
		db.Exec("UPDATE users_addresses SET pafid = ? WHERE id = ?", *req.PafID, req.ID)
	}

	return c.JSON(fiber.Map{"success": true})
}

func Delete(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid ID")
	}

	db := database.DBConn

	// Check ownership
	var ownerID uint64
	db.Raw("SELECT userid FROM users_addresses WHERE id = ?", id).Scan(&ownerID)

	if ownerID == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Address not found")
	}

	if ownerID != myid {
		return fiber.NewError(fiber.StatusForbidden, "Not your address")
	}

	db.Exec("DELETE FROM users_addresses WHERE id = ?", id)

	return c.JSON(fiber.Map{"success": true})
}

type Address struct {
	ID                              uint64  `json:"id" gorm:"primary_key"`
	Userid                          uint64  `json:"userid"`
	Instructions                    string  `json:"instructions"`
	Lat                             float64 `json:"lat"`
	Lng                             float64 `json:"lng"`
	Postcode                        string  `json:"postcode"`
	Posttown                        string  `json:"posttown"`
	Dependentlocality               string  `json:"dependentlocality"`
	Doubledependentlocality         string  `json:"doubledependentlocality"`
	Thoroughfaredescriptor          string  `json:"thoroughfaredescriptor"`
	Dependentthoroughfaredescriptor string  `json:"dependentthoroughfaredescriptor"`
	Buildingname                    string  `json:"buildingname"`
	Subbuildingname                 string  `json:"subbuildingname"`
	Pobox                           string  `json:"pobox"`
	Departmentname                  string  `json:"departmentname"`
	Organisationname                string  `json:"organisationname"`
}

func ListForUser(c *fiber.Ctx) error {
	var r []Address

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn
	db.Raw("SELECT "+
		"users_addresses.id, users_addresses.userid, instructions,"+
		"COALESCE(users_addresses.lat, locations.lat) AS lat, "+
		"COALESCE(users_addresses.lng, locations.lng) AS lng, "+
		"locations.name AS postcode, "+
		"posttown,dependentlocality,doubledependentlocality,thoroughfaredescriptor,dependentthoroughfaredescriptor,buildingname,subbuildingname,pobox,departmentname,organisationname "+
		"FROM users_addresses "+
		"INNER JOIN paf_addresses ON paf_addresses.id = users_addresses.pafid "+
		"INNER JOIN locations ON locations.id = paf_addresses.postcodeid "+
		"LEFT JOIN paf_posttown ON paf_posttown.id = paf_addresses.posttownid "+
		"LEFT JOIN paf_dependentlocality ON paf_dependentlocality.id = paf_addresses.dependentlocalityid "+
		"LEFT JOIN paf_doubledependentlocality ON paf_doubledependentlocality.id = paf_addresses.doubledependentlocalityid "+
		"LEFT JOIN paf_thoroughfaredescriptor ON paf_thoroughfaredescriptor.id = paf_addresses.thoroughfaredescriptorid "+
		"LEFT JOIN paf_dependentthoroughfaredescriptor ON paf_dependentthoroughfaredescriptor.id = paf_addresses.dependentthoroughfaredescriptorid "+
		"LEFT JOIN paf_buildingname ON paf_buildingname.id = paf_addresses.buildingnameid "+
		"LEFT JOIN paf_subbuildingname ON paf_subbuildingname.id = paf_addresses.subbuildingnameid "+
		"LEFT JOIN paf_pobox ON paf_pobox.id = paf_addresses.poboxid "+
		"LEFT JOIN paf_departmentname ON paf_departmentname.id = paf_addresses.departmentnameid "+
		"LEFT JOIN paf_organisationname ON paf_organisationname.id = paf_addresses.organisationnameid "+
		"WHERE users_addresses.userid = ?", myid).Scan(&r)

	if len(r) == 0 {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	} else {
		return c.JSON(r)
	}
}

func GetAddress(c *fiber.Ctx) error {
	var r []Address

	myid := user.WhoAmI(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)

	if err == nil {
		// We have to check that the address is referenced by a chat message in a chat to which we have access, or
		// which we own, or where we are a moderator of the group associated with the chat, or if we have Support/Admin rights.
		db := database.DBConn
		db.Raw("SELECT "+
			"users_addresses.id, users_addresses.userid, instructions,"+
			"COALESCE(users_addresses.lat, locations.lat) AS lat, "+
			"COALESCE(users_addresses.lng, locations.lng) AS lng, "+
			"locations.name AS postcode, "+
			"posttown,dependentlocality,doubledependentlocality,thoroughfaredescriptor,dependentthoroughfaredescriptor,buildingname,subbuildingname,pobox,departmentname,organisationname "+
			"FROM users_addresses "+
			"LEFT JOIN chat_rooms ON chat_rooms.user1 = ? OR chat_rooms.user2 = ? "+
			"LEFT JOIN chat_messages ON chat_messages.chatid = chat_rooms.id "+
			"LEFT JOIN memberships ON memberships.groupid = chat_rooms.groupid AND memberships.userid = ? AND memberships.role IN (?, ?) "+
			"LEFT JOIN users ON users.id = ? "+
			"INNER JOIN paf_addresses ON paf_addresses.id = users_addresses.pafid "+
			"INNER JOIN locations ON locations.id = paf_addresses.postcodeid "+
			"LEFT JOIN paf_posttown ON paf_posttown.id = paf_addresses.posttownid "+
			"LEFT JOIN paf_dependentlocality ON paf_dependentlocality.id = paf_addresses.dependentlocalityid "+
			"LEFT JOIN paf_doubledependentlocality ON paf_doubledependentlocality.id = paf_addresses.doubledependentlocalityid "+
			"LEFT JOIN paf_thoroughfaredescriptor ON paf_thoroughfaredescriptor.id = paf_addresses.thoroughfaredescriptorid "+
			"LEFT JOIN paf_dependentthoroughfaredescriptor ON paf_dependentthoroughfaredescriptor.id = paf_addresses.dependentthoroughfaredescriptorid "+
			"LEFT JOIN paf_buildingname ON paf_buildingname.id = paf_addresses.buildingnameid "+
			"LEFT JOIN paf_subbuildingname ON paf_subbuildingname.id = paf_addresses.subbuildingnameid "+
			"LEFT JOIN paf_pobox ON paf_pobox.id = paf_addresses.poboxid "+
			"LEFT JOIN paf_departmentname ON paf_departmentname.id = paf_addresses.departmentnameid "+
			"LEFT JOIN paf_organisationname ON paf_organisationname.id = paf_addresses.organisationnameid "+
			"WHERE users_addresses.id = ? AND (users_addresses.userid = ? OR (chat_messages.type = ? AND chat_messages.message = ?) OR memberships.id IS NOT NULL OR users.systemrole IN (?, ?)) LIMIT 1",
			myid, myid, myid, utils.ROLE_MODERATOR, utils.ROLE_OWNER, myid, id, myid, utils.CHAT_MESSAGE_ADDRESS, id, utils.SYSTEMROLE_ADMIN, utils.SYSTEMROLE_SUPPORT).Scan(&r)
	}

	if len(r) == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Address not visible")
	} else {
		return c.JSON(r[0])
	}

}
