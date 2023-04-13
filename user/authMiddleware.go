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
		userIdInJWT, _ := getJWTFromRequest(c)

		var wg sync.WaitGroup

		if userIdInJWT > 0 {
			// We have a valid JWT with a user id in it.  But is the user id still in our DB?
			wg.Add(1)

			go func() {
				defer wg.Done()

				// We have a uid.  Check if the user is still present in the DB.
				// TODO If the user has forced a logout since the JWT was issued, we should reject the request.
				// This will need the grant time adding to the JWT.
				db := database.DBConn
				db.Raw("SELECT id FROM users WHERE id = ? LIMIT 1;", userIdInJWT).Scan(&userIdInDB)
			}()
		}

		ret := c.Next()
		wg.Wait()

		if userIdInJWT > 0 && userIdInDB != userIdInJWT {
			// We were passed a user ID in the JWT, but it's not present in the DB.  This means that the user has
			// sent an invalid JWT.  Return an error.
			fmt.Println("Invalid user in JWT", userIdInJWT, userIdInDB)
			ret = fiber.NewError(fiber.StatusUnauthorized, "JWT for invalid user")
		}

		return ret
	}
}
