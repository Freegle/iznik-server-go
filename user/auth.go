package user

import (
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"os"
	"strconv"
)

func WhoAmI(c *fiber.Ctx) uint64 {
	// Passing JWT via URL parameters is not a great idea, but it's useful to support that for testing.
	tokenString := c.Query("jwt")

	var ret uint64 = 0

	if tokenString != "" {
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

	return ret
}
