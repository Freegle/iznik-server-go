package abtest

import (
	"math/rand"

	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber/v2"
)

type GetABTestRequest struct {
	UID string `query:"uid"`
}

type PostABTestRequest struct {
	UID     string `json:"uid"`
	Variant string `json:"variant"`
	Shown   *bool  `json:"shown"`
	Action  *bool  `json:"action"`
	Score   *int   `json:"score"`
	App     *bool  `json:"app"`
}

type ABTestVariant struct {
	ID      uint64  `json:"id"`
	UID     string  `json:"uid"`
	Variant string  `json:"variant"`
	Shown   uint64  `json:"shown"`
	Action  uint64  `json:"action"`
	Rate    float64 `json:"rate"`
	Suggest bool    `json:"suggest"`
}

func GetABTest(c *fiber.Ctx) error {
	uid := c.Query("uid")
	if uid == "" {
		return fiber.NewError(fiber.StatusBadRequest, "uid is required")
	}

	db := database.DBConn

	var variants []ABTestVariant
	db.Raw("SELECT * FROM abtest WHERE uid = ? AND suggest = 1 ORDER BY rate DESC, RAND()", uid).Scan(&variants)

	if len(variants) == 0 {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success", "variant": nil})
	}

	// Epsilon-greedy bandit: 10% exploration, 90% exploitation
	var chosen ABTestVariant
	if rand.Float64() < 0.1 {
		chosen = variants[rand.Intn(len(variants))]
	} else {
		chosen = variants[0]
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success", "variant": chosen})
}

func PostABTest(c *fiber.Ctx) error {
	var req PostABTestRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid request body")
	}

	// Ignore app requests (old code may contaminate experiments)
	if req.App != nil && *req.App {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	if req.UID == "" || req.Variant == "" {
		return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
	}

	db := database.DBConn

	if req.Shown != nil && *req.Shown {
		db.Exec("INSERT INTO abtest (uid, variant, shown, action, rate) VALUES (?, ?, 1, 0, 0) ON DUPLICATE KEY UPDATE shown = shown + 1, rate = COALESCE(100 * action / shown, 0)", req.UID, req.Variant)
	}

	if req.Action != nil && *req.Action {
		score := 1
		if req.Score != nil && *req.Score > 0 {
			score = *req.Score
		}
		db.Exec("INSERT INTO abtest (uid, variant, shown, action, rate) VALUES (?, ?, 0, ?, 0) ON DUPLICATE KEY UPDATE action = action + ?, rate = COALESCE(100 * action / shown, 0)", req.UID, req.Variant, score, score)
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
