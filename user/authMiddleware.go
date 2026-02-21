package user

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v2"
	"sync"
	"time"
)

type Config struct{}

func NewAuthMiddleware(config Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var userIdInDB struct {
			Id         uint64    `gorm:"id"`
			Lastaccess time.Time `gorm:"lastaccess"`
			Systemrole string    `gorm:"systemrole"`
		}

		userIdInJWT, sessionIdInJWT, _ := GetJWTFromRequest(c)

		var wg sync.WaitGroup

		if userIdInJWT > 0 {
			// Flag our session for Sentry.
			sentry.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetUser(sentry.User{ID: fmt.Sprint(userIdInJWT)})
			})

			// We have a valid JWT with a user id in it.  But is the user id still in our DB?  And do they still
			// have the same active session?
			wg.Add(1)
			db := database.DBConn

			go func() {
				defer wg.Done()

				// We have a uid.  Check if the user is still present in the DB.
				// Also fetch systemrole for HAProxy rate limit exemption.
				db.Raw("SELECT users.id, users.lastaccess, users.systemrole FROM sessions INNER JOIN users ON users.id = sessions.userid WHERE sessions.id = ? AND users.id = ? LIMIT 1;", sessionIdInJWT, userIdInJWT).Scan(&userIdInDB)
			}()
		}

		ret := c.Next()
		wg.Wait()

		if userIdInJWT > 0 && (userIdInDB.Id != userIdInJWT) && c.Locals("skipPostAuthCheck") == nil {
			// We were passed a user ID in the JWT, but it's not present in the DB.  This means that the user has
			// sent an invalid JWT.  Return an error.
			// Handlers that intentionally delete sessions (DeleteSession, Forget) set skipPostAuthCheck
			// to avoid a race condition where this goroutine's check runs after the session is deleted.
			ret = fiber.NewError(fiber.StatusUnauthorized, "JWT for invalid user or session")
		}

		// Store the user's system role in locals for the Loki middleware to set X-User-Role header.
		// This allows HAProxy to exempt mods/support/admin from rate limiting.
		if userIdInDB.Systemrole != "" && userIdInDB.Systemrole != "User" {
			c.Locals("userRole", userIdInDB.Systemrole)
		}

		// Update the last access time for the user if it is null or older than ten minutes.
		if userIdInJWT > 0 && userIdInDB.Id > 0 && (userIdInDB.Lastaccess.IsZero() || userIdInDB.Lastaccess.Before(time.Now().Add(-10*time.Minute))) {
			db := database.DBConn
			db.Exec("UPDATE users SET lastaccess = NOW() WHERE id = ?", userIdInDB.Id)
		}

		return ret
	}
}
