package user

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
	"sync"
)

type Config struct{}

func NewAuthMiddleware(config Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var userIdInDB uint64
		var sessionIdInDB uint64
		userIdInJWT, sessionIdInJWT, _ := getJWTFromRequest(c)

		var wg sync.WaitGroup

		if userIdInJWT > 0 {
			// We have a valid JWT with a user id in it.  But is the user id still in our DB?  And do they still
			// have an active session?
			wg.Add(1)
			db := database.DBConn

			go func() {
				defer wg.Done()

				// We have a uid.  Check if the user is still present in the DB.
				db.Raw("SELECT id FROM users WHERE id = ? LIMIT 1;", userIdInJWT).Scan(&userIdInDB)
			}()

			wg.Add(1)

			go func() {
				// Check if the session is still present in the DB.  If we have explicitly logged out, then it
				// will have been removed and therefore our JWT is invalid.
				defer wg.Done()

				db.Raw("SELECT id FROM sessions WHERE id = ? LIMIT 1;", sessionIdInJWT).Scan(&sessionIdInDB)
			}()
		}

		ret := c.Next()
		wg.Wait()

		if userIdInJWT > 0 && (userIdInDB != userIdInJWT || sessionIdInDB != sessionIdInJWT) {
			// We were passed a user ID in the JWT, but it's not present in the DB.  This means that the user has
			// sent an invalid JWT.  Return an error.
			fmt.Println("Invalid user in JWT", userIdInJWT, userIdInDB)
			ret = fiber.NewError(fiber.StatusUnauthorized, "JWT for invalid user")
		}

		return ret
	}
}
