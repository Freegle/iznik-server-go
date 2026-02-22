package user

import (
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"iznik-server-go/location"
)

// WhoAmI returns the authenticated user ID from the request, or 0 if not logged in.
// This delegates to auth.WhoAmI to avoid circular imports (user -> location -> auth).
func WhoAmI(c *fiber.Ctx) uint64 {
	return auth.WhoAmI(c)
}

// GetJWTFromRequest extracts user ID, session ID, and expiry from the JWT in the request.
func GetJWTFromRequest(c *fiber.Ctx) (uint64, uint64, float64) {
	return auth.GetJWTFromRequest(c)
}

func GetLoveJunkUser(ljuserid uint64, partnerkey string, firstname *string, lastname *string, postcodeprefix *string, profileurl *string) (*fiber.Error, uint64) {
	var myid uint64
	myid = 0

	db := database.DBConn

	if ljuserid > 0 {
		// This is ostensibly LoveJunk calling.  We need to check the partner key to validate.
		if partnerkey != "" {
			// Find in partners_keys table
			var partnername string
			db.Raw("SELECT partner FROM partners_keys WHERE `key`= ?", partnerkey).Scan(&partnername)

			// Change partnername to lower case
			partnername = strings.ToLower(partnername)

			// Check if partner name contains lovejunk.  The "contains" part allows us to run tests.
			if strings.Contains(partnername, "lovejunk") {
				// We have a valid partner key.  See if we have a user with this ljuserid.
				var ljuser User
				db.Raw("SELECT * FROM users WHERE ljuserid = ?", ljuserid).Scan(&ljuser)

				if ljuser.ID > 0 {
					// We do.
					myid = ljuser.ID
				} else {
					// We don't, so we need to create one.  Get the firstname, last name and profile url.
					ljuser.Firstname = firstname
					ljuser.Lastname = lastname
					ljuser.Fullname = nil
					ljuser.Ljuserid = &ljuserid
					ljuser.Lastaccess = time.Now()
					ljuser.Added = time.Now()
					ljuser.Systemrole = "User"
					db.Omit("Spammer").Create(&ljuser)

					if ljuser.ID == 0 {
						return fiber.NewError(fiber.StatusInternalServerError, "Error creating new user"), 0
					}

					myid = ljuser.ID

					// Create avatar from LoveJunk profile URL if provided.
					if profileurl != nil && *profileurl != "" {
						db.Exec("INSERT INTO users_images (userid, url, `default`) VALUES (?, ?, 0)", myid, *profileurl)
					}
				}

				if postcodeprefix != nil {
					// We have an approximate location.  This should be the first part of the postcode.
					// Update the user's location if needed.
					var locations []location.Location
					db.Raw("SELECT id FROM locations WHERE name LIKE ? AND type = ? LIMIT 1;", *postcodeprefix+"%", location.TYPE_POSTCODE).Scan(&locations)

					if len(locations) > 0 && locations[0].ID > 0 && (ljuser.Lastlocation == nil || locations[0].ID != *ljuser.Lastlocation) {
						// We have a location.
						// Update user table with location.
						db.Exec("UPDATE users SET lastlocation = ? WHERE id = ?", locations[0].ID, myid)
					}
				}
			} else {
				return fiber.NewError(fiber.StatusUnauthorized, "Invalid partner key"), 0
			}
		}
	}

	return fiber.NewError(fiber.StatusOK, "OK"), myid
}
