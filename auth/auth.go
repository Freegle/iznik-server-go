// Package auth provides authentication and authorization primitives.
// It exists as a separate package from user to break a circular dependency:
// user imports location (for geo queries), and location needs auth functions.
// This package depends only on database/fiber/jwt — no user or location imports.
package auth

import (
	json2 "encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// PersistentToken represents the old-style session token from the Authorization2 header.
type PersistentToken struct {
	ID     uint64 `json:"id"`
	Series uint64 `json:"series"`
	Token  string `json:"token"`
}

// WhoAmI returns the authenticated user ID from the request, or 0 if not logged in.
// It first tries JWT (fast), then falls back to the old-style persistent token.
func WhoAmI(c *fiber.Ctx) uint64 {
	id, _, _ := GetJWTFromRequest(c)

	// If we don't manage to get a user from the JWT, which is fast, then try the old-style persistent token which
	// is stored in the session table.
	persistent := c.Get("Authorization2")

	if id == 0 && len(persistent) > 0 {
		// parse persistent token
		var persistentToken PersistentToken
		json2.Unmarshal([]byte(persistent), &persistentToken)

		if (persistentToken.ID > 0) && (persistentToken.Series > 0) && (persistentToken.Token != "") {
			// Verify token against sessions table
			db := database.DBConn

			type Userid struct {
				Userid uint64 `json:"userid"`
			}

			var userids []Userid
			db.Raw("SELECT userid FROM sessions WHERE id = ? AND series = ? AND token = ? LIMIT 1;", persistentToken.ID, persistentToken.Series, persistentToken.Token).Scan(&userids)

			if len(userids) > 0 {
				id = userids[0].Userid
			}
		}
	}

	return id
}

// GetJWTFromRequest extracts user ID, session ID, and expiry from the JWT in the request.
func GetJWTFromRequest(c *fiber.Ctx) (uint64, uint64, float64) {
	// Passing JWT via URL parameters is not a great idea, but it's useful to support that for testing.
	tokenString := c.Query("jwt")

	if tokenString == "" {
		// No URL parameter found.  Try Authorization header.
		tokenString = c.Get("Authorization")
	}

	if tokenString != "" && len(tokenString) > 2 {
		// Check if there are leading and trailing quotes.  If so, strip them.
		if tokenString[0] == '"' {
			tokenString = tokenString[1:]
		}
		if tokenString[len(tokenString)-1] == '"' {
			tokenString = tokenString[:len(tokenString)-1]
		}

		token, err := jwt.Parse(string(tokenString), func(token *jwt.Token) (interface{}, error) {
			key := os.Getenv("JWT_SECRET")
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(key), nil
		})

		if err != nil {
			fmt.Println("Failed to parse JWT", tokenString, err)
		} else if !token.Valid {
			fmt.Println("JWT invalid", tokenString)
		} else {
			if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
				// Get the expiry time.  Must be in the future otherwise the parse would have failed earlier.
				exp, _ := claims["exp"]
				idi, oki := claims["id"]
				sessionidi, oks := claims["sessionid"]

				if oki && oks {
					idStr := idi.(string)
					id, _ := strconv.ParseUint(idStr, 10, 64)
					sessionIdStr, _ := sessionidi.(string)
					sessionId, _ := strconv.ParseUint(sessionIdStr, 10, 64)

					return id, sessionId, exp.(float64)
				}
			}
		}
	}

	return 0, 0, 0
}

// IsAdminOrSupport checks if the user has Admin or Support system role.
func IsAdminOrSupport(myid uint64) bool {
	db := database.DBConn
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	return systemrole == "Support" || systemrole == "Admin"
}

// IsSystemMod checks if the user has system-level Moderator, Support, or Admin role.
func IsSystemMod(myid uint64) bool {
	db := database.DBConn
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)
	return systemrole == "Moderator" || systemrole == "Support" || systemrole == "Admin"
}

// IsModOfGroup checks if the user is a Moderator or Owner of the given group, or is Admin/Support.
func IsModOfGroup(myid uint64, groupid uint64) bool {
	if IsAdminOrSupport(myid) {
		return true
	}

	if groupid == 0 {
		return false
	}

	db := database.DBConn
	var role string
	db.Raw("SELECT role FROM memberships WHERE userid = ? AND groupid = ?", myid, groupid).Scan(&role)
	return role == "Moderator" || role == "Owner"
}

// IsModOfAnyGroup checks if the user is a Moderator or Owner of any group, or is Admin/Support.
func IsModOfAnyGroup(myid uint64) bool {
	if IsAdminOrSupport(myid) {
		return true
	}

	db := database.DBConn
	var count int64
	db.Raw("SELECT COUNT(*) FROM memberships WHERE userid = ? AND role IN ('Moderator', 'Owner')", myid).Scan(&count)
	return count > 0
}
