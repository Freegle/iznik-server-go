package location

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
)

type Location struct {
	ID       uint64  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Lat      float32 `json:"lat"`
	Lng      float32 `json:"lng"`
	Areaid   uint64  `json:"areaid"`
	Areaname string  `json:"areaname"`
}

func ClosestPostcode(lat float32, lng float32) (uint64, string, string) {
	// We use our spatial index to narrow down the locations to search through; we start off very close to the
	// point and work outwards. That way in densely postcoded areas we have a fast query, and in less dense
	// areas we have some queries which are quick but don't return anything.
	var scan = float32(0.00001953125)
	var id uint64 = 0
	var name = ""
	var areaname = ""

	db := database.DBConn

	for {
		swlat := lat - scan
		swlng := lng - scan
		nelat := lat + scan
		nelng := lng + scan

		var locs []Location

		db.Raw("SELECT l1.id, l1.name, l1.areaid, l1.lat, l1.lng, l2.name as areaname, "+
			"ST_distance(locations_spatial.geometry, ST_SRID(POINT(?, ?), ?)) AS dist "+
			"FROM locations_spatial INNER JOIN locations l1 ON l1.id = locations_spatial.locationid "+
			"LEFT JOIN locations l2 ON l2.id = l1.areaid "+
			"WHERE MBRContains(ST_Envelope("+
			"ST_SRID("+
			"	POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))"+
			"	), ?)"+
			"), locations_spatial.geometry) AND "+
			"l1.type = 'Postcode' "+
			"ORDER BY dist ASC, CASE WHEN ST_Dimension(locations_spatial.geometry) < 2 THEN 0 ELSE ST_AREA(locations_spatial.geometry) END ASC LIMIT 1;",
			lng,
			lng,
			utils.SRID,
			swlng, swlat,
			swlng, nelat,
			nelng, nelat,
			nelng, swlat,
			swlng, swlat,
			utils.SRID,
		).Scan(&locs)

		if len(locs) > 0 {
			id = locs[0].ID
			name = locs[0].Name
			areaname = locs[0].Areaname
			break
		} else {
			scan = scan * 2

			if scan > 0.2 {
				break
			}
		}
	}

	return id, name, areaname
}
