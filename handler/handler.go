package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
)

type response struct {
	Status    string  `json:"status"`
	Country   string  `json:"country"`
	Region    string  `json:"regionName"`
	Latitude  float32 `json:"lat"`
	Longitude float32 `json:"lon"`
	ISP       string  `json:"isp"`
}

// CacheRequest caches the request for subsequent use
func CacheRequest(exp time.Duration) fiber.Handler {
	return cache.New(cache.Config{
		Expiration:   exp,
		CacheControl: true,
	})
}

// geoClient is a shared HTTP client with a reasonable timeout for external API calls.
var geoClient = &http.Client{Timeout: 5 * time.Second}

// GeoLocation fetches the details of the IP from a public http API.
func GeoLocation(c *fiber.Ctx) error {
	ip := c.Params("ip")

	res, err := geoClient.Get("http://ip-api.com/json/" + ip)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "Failed to reach geolocation service")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "Failed to read geolocation response")
	}

	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "Invalid geolocation response")
	}

	if resp.Status == "fail" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "enter an ip",
		})
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}
