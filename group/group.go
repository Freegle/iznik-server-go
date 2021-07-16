package group

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/gofiber/fiber"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
)

type Group struct {
	gorm.Model
	id        uint
	Nameshort string `json:"nameshort"`
}

func GetGroup(c *fiber.Ctx) {
	id := c.Params("id")
	fmt.Println("Get group %id", id)
	db := database.DBConn
	var group Group
	db.Debug().Unscoped().Find(&group, id)
	c.JSON(group)
}
