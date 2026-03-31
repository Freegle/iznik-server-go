package session

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/freegle/iznik-server-go/auth"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
)

// facebookGraphURL can be overridden via env var for testing.
func getFacebookGraphURL() string {
	if u := os.Getenv("FACEBOOK_GRAPH_URL"); u != "" {
		return u
	}
	return "https://graph.facebook.com"
}

// facebookJWKSURL can be overridden via env var for testing.
func getFacebookJWKSURL() string {
	if u := os.Getenv("FACEBOOK_JWKS_URL"); u != "" {
		return u
	}
	return "https://limited.facebook.com/.well-known/oauth/openid/jwks/"
}

// facebookGraphResponse represents the response from Facebook's Graph API /me endpoint.
type facebookGraphResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// handleFacebookLogin verifies a Facebook access token via the Graph API and logs the user in.
func handleFacebookLogin(c *fiber.Ctx, accessToken string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(getFacebookGraphURL() + "/me?fields=id,name,first_name,last_name,email&access_token=" + accessToken)
	if err != nil {
		stdlog.Printf("Facebook Graph API request failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to verify Facebook token",
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to read Facebook response",
		})
	}

	if resp.StatusCode != 200 {
		stdlog.Printf("Facebook Graph API returned status %d: %s", resp.StatusCode, string(body))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Invalid Facebook token",
		})
	}

	var fbResp facebookGraphResponse
	if err := json.Unmarshal(body, &fbResp); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to parse Facebook response",
		})
	}

	if fbResp.ID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Facebook response missing user ID",
		})
	}

	return completeFacebookLogin(c, fbResp.ID, fbResp.Email, fbResp.FirstName, fbResp.LastName, fbResp.Name)
}

// handleFacebookLimitedLogin handles Facebook Limited Login by verifying a JWT
// signed with Facebook's public keys.
func handleFacebookLimitedLogin(c *fiber.Ctx, jwtToken string) error {
	// Fetch Facebook's JWKS.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(getFacebookJWKSURL())
	if err != nil {
		stdlog.Printf("Facebook JWKS fetch failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to fetch Facebook public keys",
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to read Facebook JWKS",
		})
	}

	keys, err := parseJWKS(body)
	if err != nil {
		stdlog.Printf("Facebook JWKS parse failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Failed to parse Facebook public keys",
		})
	}

	// Parse and verify the JWT.
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is RSA.
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in JWT header")
		}

		key, exists := keys[kid]
		if !exists {
			return nil, fmt.Errorf("unknown kid: %s", kid)
		}

		return key, nil
	})

	if err != nil {
		stdlog.Printf("Facebook Limited Login JWT verification failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Invalid Facebook Limited Login token",
		})
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Invalid Facebook Limited Login claims",
		})
	}

	// Extract fields from claims.
	sub, _ := claims["sub"].(string)
	email, _ := claims["email"].(string)
	givenName, _ := claims["given_name"].(string)
	familyName, _ := claims["family_name"].(string)
	name, _ := claims["name"].(string)

	if sub == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ret":    2,
			"status": "Facebook Limited Login token missing subject",
		})
	}

	return completeFacebookLogin(c, sub, email, givenName, familyName, name)
}

// completeFacebookLogin is shared by both regular and limited Facebook login flows.
func completeFacebookLogin(c *fiber.Ctx, fbID, email, firstName, lastName, fullName string) error {
	userID, err := socialMatchOrCreate(
		utils.LOGIN_TYPE_FACEBOOK,
		fbID,
		email,
		firstName,
		lastName,
		fullName,
	)
	if err != nil {
		stdlog.Printf("Facebook login socialMatchOrCreate failed: %v", err)
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

// jwksKey represents a single key from a JWKS response.
type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
}

// jwksResponse represents a JWKS response.
type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// parseJWKS parses a JWKS JSON response and returns a map of kid -> *rsa.PublicKey.
func parseJWKS(data []byte) (map[string]*rsa.PublicKey, error) {
	var jwks jwksResponse
	if err := json.Unmarshal(data, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, fmt.Errorf("failed to decode modulus for kid %s: %w", k.Kid, err)
		}

		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, fmt.Errorf("failed to decode exponent for kid %s: %w", k.Kid, err)
		}

		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)

		keys[k.Kid] = &rsa.PublicKey{
			N: n,
			E: int(e.Int64()),
		}
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no RSA keys found in JWKS")
	}

	return keys, nil
}
