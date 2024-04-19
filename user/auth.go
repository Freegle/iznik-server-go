package user

import (
	json2 "encoding/json"
	"errors"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"os"
	"strconv"
	"strings"
	"time"
)

type PersistentToken struct {
	ID     uint64 `json:"id"`
	Series uint64 `json:"series"`
	Token  string `json:"token"`
}

func WhoAmI(c *fiber.Ctx) uint64 {
	id, _, _ := getJWTFromRequest(c)

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

func getJWTFromRequest(c *fiber.Ctx) (uint64, uint64, float64) {
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

func GetLoveJunkUser(ljuserid uint64, partnerkey string, firstname *string, lastname *string) (*fiber.Error, uint64) {
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
					db.Create(&ljuser)

					if ljuser.ID == 0 {
						return fiber.NewError(fiber.StatusInternalServerError, "Error creating new user"), 0
					}

					myid = ljuser.ID

					// TODO Create avatar
					//profileurl := c.Params("profileurl")
				}
			} else {
				return fiber.NewError(fiber.StatusUnauthorized, "Invalid partner key"), 0
			}
		}
	}

	return fiber.NewError(fiber.StatusOK, "OK"), myid
}
