package user

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v2"
	"sync"
)

type Config struct{}

func NewAuthMiddleware(config Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var userIdInDB uint64
		userIdInJWT, sessionIdInJWT, _ := getJWTFromRequest(c)

		var wg sync.WaitGroup

		if userIdInJWT > 0 {
			// Flag our session for Sentry.
			sentry.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetUser(sentry.User{ID: fmt.Sprint(userIdInJWT)})
			})

			// We have a valid JWT with a user id in it.  But is the user id still in our DB?  And do they still
			// have the same active session active session?
			wg.Add(1)
			db := database.DBConn

			go func() {
				defer wg.Done()

				// We have a uid.  Check if the user is still present in the DB.
				db.Raw("SELECT users.id FROM sessions INNER JOIN users ON users.id = sessions.userid WHERE sessions.id = ? AND users.id = ? LIMIT 1;", sessionIdInJWT, userIdInJWT).Scan(&userIdInDB)
			}()
		}

		ret := c.Next()
		wg.Wait()

		if userIdInJWT > 0 && (userIdInDB != userIdInJWT) {
			// We were passed a user ID in the JWT, but it's not present in the DB.  This means that the user has
			// sent an invalid JWT.  Return an error.
			fmt.Println("Invalid user in JWT", userIdInJWT, userIdInDB, sessionIdInJWT)
			ret = fiber.NewError(fiber.StatusUnauthorized, "JWT for invalid user")
		}

		return ret
	}
}
