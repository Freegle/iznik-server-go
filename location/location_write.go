package location

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// whoAmI extracts the authenticated user ID from the JWT in the request.
// This is a local version to avoid a circular import with the user package.
func whoAmI(c *fiber.Ctx) uint64 {
	tokenString := c.Query("jwt")
	if tokenString == "" {
		tokenString = c.Get("Authorization")
	}

	if tokenString == "" || len(tokenString) < 3 {
		return 0
	}

	// Strip quotes if present.
	if tokenString[0] == '"' {
		tokenString = tokenString[1:]
	}
	if tokenString[len(tokenString)-1] == '"' {
		tokenString = tokenString[:len(tokenString)-1]
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return 0
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if idi, oki := claims["id"]; oki {
			id, _ := strconv.ParseUint(idi.(string), 10, 64)
			return id
		}
	}

	return 0
}

// isSystemMod checks if the user has system-level Moderator, Support, or Admin role.
func isSystemMod(myid uint64) bool {
	db := database.DBConn
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	return systemrole == "Moderator" || systemrole == "Support" || systemrole == "Admin"
}

// isGroupMod checks if the user is a Moderator or Owner of the given group.
func isGroupMod(myid uint64, groupid uint64) bool {
	db := database.DBConn
	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", myid, groupid).Scan(&role)
	return role == "Moderator" || role == "Owner"
}

type CreateLocationRequest struct {
	Name    string `json:"name"`
	Polygon string `json:"polygon"`
}

// CreateLocation handles PUT /locations - create a new location (system mod/admin only).
func CreateLocation(c *fiber.Ctx) error {
	myid := whoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isSystemMod(myid) {
		return fiber.NewError(fiber.StatusForbidden, "System moderator or admin role required")
	}

	var req CreateLocationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Name == "" || req.Polygon == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name and polygon are required")
	}

	canon := strings.ToLower(req.Name)

	db := database.DBConn
	result := db.Exec(
		fmt.Sprintf("INSERT INTO locations (name, type, geometry, canon, popularity) VALUES (?, 'Polygon', ST_GeomFromText(?, %d), ?, 0)", utils.SRID),
		req.Name, req.Polygon, canon,
	)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create location")
	}

	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)

	return c.JSON(fiber.Map{"id": id})
}

type UpdateLocationRequest struct {
	ID      uint64  `json:"id"`
	Name    *string `json:"name,omitempty"`
	Polygon *string `json:"polygon,omitempty"`
}

// UpdateLocation handles PATCH /locations - update a location (system mod/admin only).
func UpdateLocation(c *fiber.Ctx) error {
	myid := whoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	if !isSystemMod(myid) {
		return fiber.NewError(fiber.StatusForbidden, "System moderator or admin role required")
	}

	var req UpdateLocationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.ID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id is required")
	}

	db := database.DBConn

	if req.Polygon != nil && *req.Polygon != "" {
		db.Exec(
			fmt.Sprintf("UPDATE locations SET geometry = ST_GeomFromText(?, %d) WHERE id = ?", utils.SRID),
			*req.Polygon, req.ID,
		)
	}

	if req.Name != nil && *req.Name != "" {
		canon := strings.ToLower(*req.Name)
		db.Exec("UPDATE locations SET name = ?, canon = ? WHERE id = ?", *req.Name, canon, req.ID)
	}

	return c.JSON(fiber.Map{"success": true})
}

type ExcludeLocationRequest struct {
	ID        uint64 `json:"id"`
	GroupID   uint64 `json:"groupid"`
	Action    string `json:"action"`
	Byname    bool   `json:"byname"`
	MessageID uint64 `json:"messageid"`
}

// ExcludeLocation handles POST /locations with action=Exclude - exclude a location from a group (group mod only).
func ExcludeLocation(c *fiber.Ctx) error {
	myid := whoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req ExcludeLocationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.Action != "Exclude" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid action")
	}

	if req.ID == 0 || req.GroupID == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "id and groupid are required")
	}

	if !isGroupMod(myid, req.GroupID) {
		return fiber.NewError(fiber.StatusForbidden, "Must be a moderator or owner of the group")
	}

	db := database.DBConn

	// Exclude the specified location.
	db.Exec("INSERT IGNORE INTO locations_excluded (locationid, groupid, userid) VALUES (?, ?, ?)",
		req.ID, req.GroupID, myid)

	// If byname, also exclude all locations with the same name.
	if req.Byname {
		var name string
		db.Raw("SELECT name FROM locations WHERE id = ?", req.ID).Scan(&name)
		if name != "" {
			var otherIDs []uint64
			db.Raw("SELECT id FROM locations WHERE name = ? AND id != ?", name, req.ID).Pluck("id", &otherIDs)
			for _, otherID := range otherIDs {
				db.Exec("INSERT IGNORE INTO locations_excluded (locationid, groupid, userid) VALUES (?, ?, ?)",
					otherID, req.GroupID, myid)
			}
		}
	}

	return c.JSON(fiber.Map{"success": true})
}

// --- KML to WKT conversion ---

type ConvertKMLRequest struct {
	Action string `json:"action"`
	KML    string `json:"kml"`
}

type kmlDocument struct {
	XMLName  xml.Name      `xml:"kml"`
	Document kmlDocElement `xml:",any"`
}

type kmlDocElement struct {
	Placemarks []kmlPlacemark `xml:"Placemark"`
}

type kmlPlacemark struct {
	Polygon kmlPolygon `xml:"Polygon"`
}

type kmlPolygon struct {
	OuterBoundaryIs kmlOuterBoundary `xml:"outerBoundaryIs"`
}

type kmlOuterBoundary struct {
	LinearRing kmlLinearRing `xml:"LinearRing"`
}

type kmlLinearRing struct {
	Coordinates string `xml:"coordinates"`
}

// ConvertKML handles POST /locations/kml - converts KML XML to WKT format.
func ConvertKML(c *fiber.Ctx) error {
	myid := whoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	var req ConvertKMLRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	if req.KML == "" {
		return fiber.NewError(fiber.StatusBadRequest, "kml is required")
	}

	var kml kmlDocument
	if err := xml.Unmarshal([]byte(req.KML), &kml); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid KML XML")
	}

	var coordsStr string
	for _, pm := range kml.Document.Placemarks {
		coords := strings.TrimSpace(pm.Polygon.OuterBoundaryIs.LinearRing.Coordinates)
		if coords != "" {
			coordsStr = coords
			break
		}
	}

	if coordsStr == "" {
		return fiber.NewError(fiber.StatusBadRequest, "No polygon coordinates found in KML")
	}

	// KML coordinates are "lng,lat[,alt]" separated by whitespace.
	// WKT needs "lng lat" pairs separated by commas.
	fields := strings.Fields(coordsStr)
	wktPairs := make([]string, 0, len(fields))

	for _, field := range fields {
		parts := strings.Split(field, ",")
		if len(parts) < 2 {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid coordinate format in KML")
		}

		lngVal, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid longitude in KML coordinates")
		}
		latVal, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "Invalid latitude in KML coordinates")
		}

		wktPairs = append(wktPairs, strconv.FormatFloat(lngVal, 'f', -1, 64)+" "+strconv.FormatFloat(latVal, 'f', -1, 64))
	}

	wkt := "POLYGON((" + strings.Join(wktPairs, ",") + "))"

	return c.JSON(fiber.Map{
		"ret":    0,
		"status": "Success",
		"wkt":    wkt,
	})
}
