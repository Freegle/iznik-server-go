package shortlink

import (
	"log"
	"os"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

// RedirectShortlink handles GET /shortlink?name=xxx — the public-facing redirect endpoint.
// This is the Go equivalent of iznik-server/http/shortlink.php.
//
// If the name matches a shortlink, it 302-redirects to the resolved URL.
// If no match or no name, it redirects to the user site.
//
// @Summary Redirect shortlink
// @Tags shortlink
// @Param name query string false "Shortlink name"
// @Success 302
// @Router /shortlink [get]
func RedirectShortlink(c *fiber.Ctx) error {
	name := c.Query("name")

	userSite := os.Getenv("USER_SITE")
	if userSite == "" {
		userSite = "www.ilovefreegle.org"
	}

	defaultURL := "https://" + userSite

	if name == "" {
		log.Printf("[Shortlink] No name parameter, redirecting to %s", defaultURL)
		return c.Redirect(defaultURL, fiber.StatusFound)
	}

	db := database.DBConn

	// Look up the shortlink by name.
	var s Shortlink
	db.Raw("SELECT * FROM shortlinks WHERE name LIKE ?", name).Scan(&s)

	if s.ID == 0 {
		log.Printf("[Shortlink] Name '%s' not found, redirecting to %s", name, defaultURL)
		return c.Redirect(defaultURL, fiber.StatusFound)
	}

	// Resolve the URL the same way as the API handler.
	resolveShortlinkURL(&s, userSite)

	var redirectURL string
	if s.Url != nil && *s.Url != "" {
		redirectURL = *s.Url
	} else {
		redirectURL = defaultURL
	}

	// Record the click.
	db.Exec("UPDATE shortlinks SET clicks = clicks + 1 WHERE id = ?", s.ID)
	db.Exec("INSERT INTO shortlink_clicks (shortlinkid) VALUES (?)", s.ID)

	log.Printf("[Shortlink] Redirecting '%s' (id=%d) to %s", name, s.ID, redirectURL)
	return c.Redirect(redirectURL, fiber.StatusFound)
}
