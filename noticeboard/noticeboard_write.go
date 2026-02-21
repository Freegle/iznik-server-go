package noticeboard

import (
	"strconv"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// DeleteNoticeboard deletes a noticeboard. Requires moderator or admin role.
// @Summary Delete noticeboard
// @Description Deletes a noticeboard by ID. Requires mod/admin.
// @Tags noticeboard
// @Produce json
// @Param id path integer true "Noticeboard ID"
// @Security BearerAuth
// @Success 200 {object} fiber.Map "Success"
// @Failure 400 {object} fiber.Error "Invalid ID"
// @Failure 401 {object} fiber.Error "Not logged in"
// @Failure 403 {object} fiber.Error "Not authorized"
// @Failure 404 {object} fiber.Error "Noticeboard not found"
// @Router /noticeboard/{id} [delete]
func DeleteNoticeboard(c *fiber.Ctx) error {
	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid id")
	}

	db := database.DBConn

	// Check the user has mod/admin role
	var systemrole string
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&systemrole)

	if systemrole != "Moderator" && systemrole != "Support" && systemrole != "Admin" {
		return fiber.NewError(fiber.StatusForbidden, "Not authorized")
	}

	// Check noticeboard exists
	var count int64
	db.Raw("SELECT COUNT(*) FROM noticeboards WHERE id = ?", id).Scan(&count)
	if count == 0 {
		return fiber.NewError(fiber.StatusNotFound, "Noticeboard not found")
	}

	// Delete the noticeboard
	result := db.Exec("DELETE FROM noticeboards WHERE id = ?", id)
	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to delete noticeboard")
	}

	return c.JSON(fiber.Map{"ret": 0, "status": "Success"})
}
