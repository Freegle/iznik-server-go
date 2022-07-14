package address

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

type Address struct {
	ID                              uint64  `json:"id" gorm:"primary_key"`
	Userid                          uint64  `json:"userid"`
	Instructions                    string  `json:"instructions"`
	Lat                             float64 `json:"lat"`
	Lng                             float64 `json:"lng"`
	Postcode                        string  `json:"postcode"`
	Posttown                        string  `json:"posttown"`
	Dependentlocality               string  `json:"dependentlocality"`
	Doubledependentlocality         string  `json:"doubledependentlocality"`
	Thoroughfaredescriptor          string  `json:"thoroughfaredescriptor"`
	Dependentthoroughfaredescriptor string  `json:"dependentthoroughfaredescriptor"`
	Buildingname                    string  `json:"buildingname"`
	Subbuildingname                 string  `json:"subbuildingname"`
	Pobox                           string  `json:"pobox"`
	Departmentname                  string  `json:"departmentname"`
	Organisationname                string  `json:"organisationname"`
}

func ListForUser(c *fiber.Ctx) error {
	var r []Address

	myid := user.WhoAmI(c)

	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	db := database.DBConn
	db.Raw("SELECT "+
		"users_addresses.id, users_addresses.userid, instructions,"+
		"COALESCE(users_addresses.lat, locations.lat) AS lat, "+
		"COALESCE(users_addresses.lng, locations.lng) AS lng, "+
		"locations.name AS postcode, "+
		"posttown,dependentlocality,doubledependentlocality,thoroughfaredescriptor,dependentthoroughfaredescriptor,buildingname,subbuildingname,pobox,departmentname,organisationname "+
		"FROM users_addresses "+
		"INNER JOIN paf_addresses ON paf_addresses.id = users_addresses.pafid "+
		"INNER JOIN locations ON locations.id = paf_addresses.postcodeid "+
		"LEFT JOIN paf_posttown ON paf_posttown.id = paf_addresses.posttownid "+
		"LEFT JOIN paf_dependentlocality ON paf_dependentlocality.id = paf_addresses.dependentlocalityid "+
		"LEFT JOIN paf_doubledependentlocality ON paf_doubledependentlocality.id = paf_addresses.doubledependentlocalityid "+
		"LEFT JOIN paf_thoroughfaredescriptor ON paf_thoroughfaredescriptor.id = paf_addresses.thoroughfaredescriptorid "+
		"LEFT JOIN paf_dependentthoroughfaredescriptor ON paf_dependentthoroughfaredescriptor.id = paf_addresses.dependentthoroughfaredescriptorid "+
		"LEFT JOIN paf_buildingname ON paf_buildingname.id = paf_addresses.buildingnameid "+
		"LEFT JOIN paf_subbuildingname ON paf_subbuildingname.id = paf_addresses.subbuildingnameid "+
		"LEFT JOIN paf_pobox ON paf_pobox.id = paf_addresses.poboxid "+
		"LEFT JOIN paf_departmentname ON paf_departmentname.id = paf_addresses.departmentnameid "+
		"LEFT JOIN paf_organisationname ON paf_organisationname.id = paf_addresses.organisationnameid "+
		"WHERE users_addresses.userid = ?", myid).Scan(&r)

	return c.JSON(r)
}
