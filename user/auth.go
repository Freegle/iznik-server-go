package user

import (
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"os"
)

func WhoAmI(c *fiber.Ctx) uint64 {
	// Passing JWT via URL parameters is not a great idea, but it's useful to support that for testing.
	var tokenString = c.Query("jwt")

	if tokenString != "" {
		fmt.Println("JWT param %s", tokenString)

		token, err := jwt.Parse(string(tokenString), func(token *jwt.Token) (interface{}, error) {
			key := os.Getenv("JWT_SECRET")
			fmt.Println("Secret", key)
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(key), nil
		})

		if err != nil {
			fmt.Println("Failed", err)
		}

		if !token.Valid {
			fmt.Println("Token invalid")
		}

		fmt.Println("Token valid", token)

	}

	return 0
}
