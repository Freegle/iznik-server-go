package session

import (
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"os"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
)

// googleTokenInfoURL can be overridden via env var for testing.
var googleTokenInfoURL = ""

func getGoogleTokenInfoURL() string {
	if u := os.Getenv("GOOGLE_TOKENINFO_URL"); u != "" {
		return u
	}
	if googleTokenInfoURL != "" {
		return googleTokenInfoURL
	}
	return "https://oauth2.googleapis.com/tokeninfo"
}

// googleTokenInfoResponse represents the response from Google's tokeninfo endpoint.
type googleTokenInfoResponse struct {
	Sub        string `json:"sub"`
	Email      string `json:"email"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Name       string `json:"name"`
	Aud        string `json:"aud"`
}

// handleGoogleLogin verifies a Google ID token and logs the user in.
func handleGoogleLogin(c *fiber.Ctx, jwtToken string) error {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		stdlog.Printf("GOOGLE_CLIENT_ID not configured")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ret":    1,
			"status": "Google login not configured",
		})
	}

	// Verify the token via Google's tokeninfo endpoint.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(getGoogleTokenInfoURL() + "?id_token=" + jwtToken)
	if err != nil {
		stdlog.Printf("Google tokeninfo request failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to verify Google token",
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to read Google token response",
		})
	}

	if resp.StatusCode != 200 {
		stdlog.Printf("Google tokeninfo returned status %d: %s", resp.StatusCode, string(body))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Invalid Google token",
		})
	}

	var tokenInfo googleTokenInfoResponse
	if err := json.Unmarshal(body, &tokenInfo); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to parse Google token info",
		})
	}

	// Verify the audience matches our client ID.
	if tokenInfo.Aud != clientID {
		stdlog.Printf("Google token audience mismatch: got %s, expected %s", tokenInfo.Aud, clientID)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Google token audience mismatch",
		})
	}

	if tokenInfo.Sub == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Google token missing subject",
		})
	}

	// Find or create the user.
	userID, err := socialMatchOrCreate(
		utils.LOGIN_TYPE_GOOGLE,
		tokenInfo.Sub,
		tokenInfo.Email,
		tokenInfo.GivenName,
		tokenInfo.FamilyName,
		tokenInfo.Name,
	)
	if err != nil {
		stdlog.Printf("Google login socialMatchOrCreate failed: %v", err)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"ret":    3,
			"status": fmt.Sprintf("Login failed: %v", err),
		})
	}

	persistent, jwtString, err := auth.CreateSessionAndJWT(userID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to create session")
	}

	return c.JSON(fiber.Map{
		"ret":        0,
		"status":     "Success",
		"persistent": persistent,
		"jwt":        jwtString,
	})
}
