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
)

type PersistentToken struct {
	ID     uint64 `json:"id"`
	Series uint64 `json:"series"`
	Token  string `json:"token"`
}

func WhoAmI(c *fiber.Ctx) uint64 {
	// Passing JWT via URL parameters is not a great idea, but it's useful to support that for testing.
	tokenString := c.Query("jwt")

	if tokenString == "" {
		// No URL parameter found.  Try Authorization header.
		tokenString = c.Get("Authorization")
	}

	var ret uint64 = 0

	if tokenString != "" {
		tokenString = tokenString[1 : len(tokenString)-1]
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
				idi, oki := claims["id"]

				if oki {
					idStr := idi.(string)
					ret, _ = strconv.ParseUint(idStr, 10, 64)
				}
			}
		}
	}

	// If we don't manage to get a user from the JWT, which is fast, then try the old-style persistent token which
	// is stored in the session table.
	persistent := c.Get("Authorization2")

	if ret == 0 && len(persistent) > 0 {
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
				ret = userids[0].Userid
			}
		}
	}

	return ret
}
