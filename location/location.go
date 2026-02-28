package location

import (
	"encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	geo "github.com/kellydunn/golang-geo"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const TYPE_POSTCODE = "Postcode"
const NEARBY = 50 // In miles.

type AreaInfo struct {
	ID   uint64  `json:"id"`
	Name string  `json:"name"`
	Lat  float32 `json:"lat"`
	Lng  float32 `json:"lng"`
}

type Location struct {
	ID         uint64         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Lat        float32        `json:"lat"`
	Lng        float32        `json:"lng"`
	Areaid     uint64         `json:"areaid"`
	Areaname   string         `json:"areaname"`
	Area       *AreaInfo      `json:"area,omitempty" gorm:"-"`
	GroupsNear []ClosestGroup `json:"groupsnear" gorm:"-"`
	Dist       float32        `json:"dist" gorm:"-"`
}

func ClosestPostcode(lat float32, lng float32) Location {
	// We use our spatial index to narrow down the locations to search through; we start off very close to the
	// point and work outwards. That way in densely postcoded areas we have a fast query, and in less dense
	// areas we have some queries which are quick but don't return anything.
	var scan = float32(0.00001953125)
	var loc Location

	db := database.DBConn

	for {
		swlat := lat - scan
		swlng := lng - scan
		nelat := lat + scan
		nelng := lng + scan

		var locs []Location

		db.Raw("SELECT l1.id, l1.name, l1.areaid, l1.lat, l1.lng, l1.type, l2.name as areaname, "+
			"ST_distance(locations_spatial.geometry, ST_SRID(POINT(?, ?), ?)) AS dist "+
			"FROM locations_spatial INNER JOIN locations l1 ON l1.id = locations_spatial.locationid "+
			"LEFT JOIN locations l2 ON l2.id = l1.areaid "+
			"WHERE MBRContains(ST_Envelope(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?)), locations_spatial.geometry) AND "+
			"l1.type = 'Postcode' "+
			"ORDER BY dist ASC, CASE WHEN ST_Dimension(locations_spatial.geometry) < 2 THEN 0 ELSE ST_AREA(locations_spatial.geometry) END ASC LIMIT 1;",
			lng,
			lat,
			utils.SRID,
			swlng, swlat,
			swlng, nelat,
			nelng, nelat,
			nelng, swlat,
			swlng, swlat,
			utils.SRID,
		).Scan(&locs)

		if len(locs) > 0 {
			loc = locs[0]
			break
		} else {
			scan = scan * 2

			if scan > 0.2 {
				break
			}
		}
	}

	return loc
}

type ClosestGroup struct {
	ID          uint64          `json:"id"`
	Nameshort   string          `json:"nameshort"`
	Namefull    string          `json:"namefull"`
	Namedisplay string          `json:"namedisplay"`
	Ontn        bool            `json:"ontn"`
	Dist        float32         `json:"dist"`
	Settings    json.RawMessage `json:"settings"` // This is JSON stored in the DB as a string.
}

func ClosestSingleGroup(lat float64, lng float64, radius float64) *ClosestGroup {
	groups := ClosestGroups(lat, lng, radius, 1)

	if len(groups) > 0 {
		return &groups[0]
	} else {
		return nil
	}
}

func ClosestGroups(lat float64, lng float64, radius float64, limit int) []ClosestGroup {
	// To make this efficient we want to use the spatial index on polyindex.  But our groups are not evenly
	// distributed, so if we search immediately upto $radius, which is the maximum we need to cover, then we
	// will often have to scan many more groups than we need in order to determine the closest groups
	// (via the LIMIT clause), and this may be slow even with a spatial index.
	//
	// For example, searching in London will find ~120 groups within 50 miles, of which we are only interested
	// in 10, and the query will take ~0.03s.  If we search within 4 miles, that will typically find what we
	// need and the query takes ~0.00s.
	//
	// So we step up, using a bounding box that covers the point and radius and searching based on the lat/lng
	// centre of the group.  That's much faster.  But (infuriatingly) there are some groups which are so large that
	// the centre of the group is further away than the centre of lots of other groups, and that means that
	// we don't find the correct group.  So to deal with such groups we have an alt lat/lng which we can set to
	// be somewhere else, effectively giving the group two "centres".  This is a fudge which clearly wouldn't
	// cope with arbitrary geographies or hyperdimensional quintuple manifolds or whatever, but works ok for our
	// little old UK reuse network.
	//
	// Because this is Go we can fire off these requests in parallel and just stop when we get enough results.
	// This reduces latency significantly, even though it's a bit mean to the database server.
	db := database.DBConn

	var currradius = math.Round(float64(radius)/16.0 + 0.5)
	results := []ClosestGroup{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0

	for {
		count++
		currradius = currradius * 2

		if currradius >= radius {
			break
		}
	}

	currradius = math.Round(float64(radius)/16.0 + 0.5)
	wg.Add(1)

	done := false

	for {
		go func(currradius float64) {
			batch := []ClosestGroup{}
			var nelat, nelng, swlat, swlng float64
			p := geo.NewPoint(lat, lng)
			ne := p.PointAtDistanceAndBearing(currradius, 45)
			nelat = ne.Lat()
			nelng = ne.Lng()
			sw := p.PointAtDistanceAndBearing(currradius, 225)
			swlat = sw.Lat()
			swlng = sw.Lng()

			db.Raw("SELECT id, nameshort, namefull, ontn, settings, "+
				"ST_distance(ST_SRID(POINT(?, ?), ?), polyindex) * 111195 * 0.000621371 AS dist, "+
				"haversine(lat, lng, ?, ?) AS hav, CASE WHEN altlat IS NOT NULL THEN haversine(altlat, altlng, ?, ?) ELSE NULL END AS hav2 FROM `groups` WHERE "+
				"MBRIntersects(polyindex, ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?)) "+
				"AND publish = 1 AND listable = 1 HAVING (hav IS NOT NULL AND hav < ? OR hav2 IS NOT NULL AND hav2 < ?) ORDER BY dist ASC, hav ASC, external ASC LIMIT ?;",
				lng,
				lat,
				utils.SRID,
				lat,
				lng,
				lat,
				lng,
				swlng, swlat,
				swlng, nelat,
				nelng, nelat,
				nelng, swlat,
				swlng, swlat,
				utils.SRID,
				currradius,
				currradius,
				limit).Scan(&batch)

			mu.Lock()
			defer mu.Unlock()

			count--

			if len(results) < limit {
				if len(batch) > 0 {
					// We found some.
					for i, r := range batch {
						if len(r.Namefull) > 0 {
							batch[i].Namedisplay = r.Namefull
						} else {
							batch[i].Namedisplay = r.Nameshort
						}
					}

					results = append(results, batch...)

					if len(results) >= limit {
						if !done {
							done = true
							defer wg.Done()
						}
					}
				}

				if count == 0 {
					// We've run out of areas to search.
					if !done {
						done = true
						defer wg.Done()
					}
				}
			}
		}(currradius)

		currradius = currradius * 2

		if currradius >= radius {
			break
		}
	}

	wg.Wait()

	// Sort results by distance, ascending.
	if len(results) > 1 {
		sort.Slice(results, func(i, j int) bool {
			return results[i].Dist < results[j].Dist
		})
	}

	// Remove duplicates by id
	seen := make(map[uint64]struct{}, len(results))
	j := 0
	for _, v := range results {
		if _, ok := seen[v.ID]; ok {
			continue
		}
		seen[v.ID] = struct{}{}
		results[j] = v
		j++
	}

	// Limit results to the first `limit` items.
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

func FetchSingle(id uint64) *Location {
	db := database.DBConn

	var location Location

	db.Raw("SELECT l1.id, l1.name, l1.areaid, l1.lat, l1.lng, l2.name as areaname "+
		"FROM locations l1 "+
		"LEFT JOIN locations l2 ON l2.id = l1.areaid "+
		"WHERE l1.id = ? "+
		"LIMIT 1;",
		id,
	).Scan(&location)

	return &location
}

func GetLocation(c *fiber.Ctx) error {
	groupsnear := c.QueryBool("groupsnear", true)

	if c.Params("id") != "" {
		// Looking for a specific location.
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			loc := FetchSingle(id)

			if groupsnear && loc.ID > 0 {
				loc.GroupsNear = ClosestGroups(float64(loc.Lat), float64(loc.Lng), NEARBY, 10)
			}

			return c.JSON(loc)
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Location not found")
}

func LatLng(c *fiber.Ctx) error {
	lat, _ := strconv.ParseFloat(c.Query("lat"), 32)
	lng, _ := strconv.ParseFloat(c.Query("lng"), 32)

	loc := ClosestPostcode(float32(lat), float32(lng))
	loc.GroupsNear = ClosestGroups(float64(loc.Lat), float64(loc.Lng), NEARBY, 10)

	return c.JSON(loc)
}

// BoxLocation represents a location returned from bounding box queries, including its polygon.
type BoxLocation struct {
	ID      uint64  `json:"id"`
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Lat     float32 `json:"lat"`
	Lng     float32 `json:"lng"`
	Areaid  uint64  `json:"areaid"`
	Polygon string  `json:"polygon"`
}

// DodgyLocation represents a dodgy location entry.
type DodgyLocation struct {
	Locationid    uint64  `json:"locationid"`
	Oldlocationid uint64  `json:"oldlocationid"`
	Newlocationid uint64  `json:"newlocationid"`
	Lat           float32 `json:"lat"`
	Lng           float32 `json:"lng"`
	Name          string  `json:"name"`
	Oldname       string  `json:"oldname"`
	Newname       string  `json:"newname"`
}

// SearchLocations handles GET /locations - search for locations by lat/lng, typeahead, or bounding box.
func SearchLocations(c *fiber.Ctx) error {
	latStr := c.Query("lat")
	lngStr := c.Query("lng")
	swlatStr := c.Query("swlat")
	nelatStr := c.Query("nelat")
	swlngStr := c.Query("swlng")
	nelngStr := c.Query("nelng")
	typeaheadStr := c.Query("typeahead")
	dodgyFlag := c.QueryBool("dodgy", false)
	areasFlag := c.QueryBool("areas", true)
	limitStr := c.Query("limit", "10")
	groupsnear := c.QueryBool("groupsnear", true)
	pconly := c.QueryBool("pconly", true)

	if latStr != "" && lngStr != "" {
		// Find closest postcode and nearby groups.
		lat, _ := strconv.ParseFloat(latStr, 32)
		lng, _ := strconv.ParseFloat(lngStr, 32)

		loc := ClosestPostcode(float32(lat), float32(lng))
		if loc.ID > 0 && groupsnear {
			loc.GroupsNear = ClosestGroups(float64(loc.Lat), float64(loc.Lng), NEARBY, 10)
		}

		return c.JSON(fiber.Map{
			"ret":      0,
			"status":   "Success",
			"location": loc,
		})
	} else if typeaheadStr != "" {
		// Typeahead search.
		limit, _ := strconv.ParseUint(limitStr, 10, 64)
		if limit > 100 {
			limit = 100
		}

		pcq := ""
		if pconly {
			pcq = "AND l1.type = '" + TYPE_POSTCODE + "'"
		}

		locations := []Location{}
		db := database.DBConn

		type locationWithArea struct {
			Location
			AreaLat float32 `json:"-" gorm:"column:arealat"`
			AreaLng float32 `json:"-" gorm:"column:arealng"`
		}

		var locs []locationWithArea
		db.Raw("SELECT l1.id, l1.name, l1.areaid, l1.lat, l1.lng, l1.type, l2.name as areaname, l2.lat as arealat, l2.lng as arealng "+
			"FROM locations l1 "+
			"LEFT JOIN locations l2 ON l2.id = l1.areaid "+
			"WHERE l1.name LIKE ? "+pcq+" AND l1.name LIKE '% %' LIMIT ?;",
			typeaheadStr+"%",
			limit).Scan(&locs)

		for i, l := range locs {
			locations = append(locations, l.Location)
			if l.Areaid > 0 {
				locations[i].Area = &AreaInfo{
					ID:   l.Areaid,
					Name: l.Areaname,
					Lat:  l.AreaLat,
					Lng:  l.AreaLng,
				}
			}
		}

		if groupsnear {
			var wg sync.WaitGroup
			wg.Add(len(locations))
			for i := range locations {
				go func(i int) {
					locations[i].GroupsNear = ClosestGroups(float64(locations[i].Lat), float64(locations[i].Lng), NEARBY, 10)
					wg.Done()
				}(i)
			}
			wg.Wait()
		}

		return c.JSON(fiber.Map{
			"ret":       0,
			"status":    "Success",
			"locations": locations,
		})
	} else if swlatStr != "" || nelatStr != "" {
		// Bounding box search.
		swlat, _ := strconv.ParseFloat(swlatStr, 64)
		swlng, _ := strconv.ParseFloat(swlngStr, 64)
		nelat, _ := strconv.ParseFloat(nelatStr, 64)
		nelng, _ := strconv.ParseFloat(nelngStr, 64)

		ret := fiber.Map{"ret": 0, "status": "Success"}

		if areasFlag {
			db := database.DBConn
			var boxLocs []BoxLocation

			db.Raw("SELECT DISTINCT l.id, l.name, l.type, l.lat, l.lng, l.areaid, "+
				"ST_AsText("+
				"CASE WHEN ST_Simplify(CASE WHEN l.ourgeometry IS NOT NULL THEN l.ourgeometry ELSE l.geometry END, 0.001) IS NULL "+
				"THEN CASE WHEN l.ourgeometry IS NOT NULL THEN l.ourgeometry ELSE l.geometry END "+
				"ELSE ST_Simplify(CASE WHEN l.ourgeometry IS NOT NULL THEN l.ourgeometry ELSE l.geometry END, 0.001) "+
				"END) AS polygon "+
				"FROM (SELECT DISTINCT locationid FROM locations_spatial "+
				"INNER JOIN locations l2 ON l2.areaid = locations_spatial.locationid "+
				"WHERE ST_Intersects(locations_spatial.geometry, "+
				"ST_GeomFromText(?, ?)) "+
				"AND l2.type = 'Postcode') ls "+
				"INNER JOIN locations l ON l.id = ls.locationid "+
				"LEFT JOIN locations_excluded ON ls.locationid = locations_excluded.locationid "+
				"WHERE locations_excluded.locationid IS NULL "+
				"LIMIT 500;",
				fmt.Sprintf("POLYGON((%f %f, %f %f, %f %f, %f %f, %f %f))",
					swlng, swlat, nelng, swlat, nelng, nelat, swlng, nelat, swlng, swlat),
				utils.SRID,
			).Scan(&boxLocs)

			// Handle POINT geometries - convert to small polygons.
			for i, loc := range boxLocs {
				if strings.HasPrefix(loc.Polygon, "POINT(") {
					sw_lat := loc.Lat - 0.0005
					sw_lng := loc.Lng - 0.0005
					ne_lat := loc.Lat + 0.0005
					ne_lng := loc.Lng + 0.0005
					boxLocs[i].Polygon = fmt.Sprintf("POLYGON((%f %f, %f %f, %f %f, %f %f, %f %f))",
						sw_lng, sw_lat, sw_lng, ne_lat, ne_lng, ne_lat, ne_lng, sw_lat, sw_lng, sw_lat)
				}
			}

			if boxLocs == nil {
				boxLocs = []BoxLocation{}
			}
			ret["locations"] = boxLocs
		}

		if dodgyFlag {
			db := database.DBConn
			var dodgyLocs []DodgyLocation
			db.Raw("SELECT ld.locationid, ld.oldlocationid, ld.newlocationid, ld.lat, ld.lng, "+
				"l0.name AS name, l1.name AS oldname, l2.name AS newname "+
				"FROM locations_dodgy ld "+
				"INNER JOIN locations l0 ON l0.id = ld.locationid "+
				"INNER JOIN locations l1 ON l1.id = ld.oldlocationid "+
				"INNER JOIN locations l2 ON l2.id = ld.newlocationid "+
				"WHERE ld.lat BETWEEN ? AND ? AND ld.lng BETWEEN ? AND ?;",
				swlat, nelat, swlng, nelng,
			).Scan(&dodgyLocs)

			if dodgyLocs == nil {
				dodgyLocs = []DodgyLocation{}
			}
			ret["dodgy"] = dodgyLocs
		}

		return c.JSON(ret)
	}

	return fiber.NewError(fiber.StatusBadRequest, "Missing required parameters (lat/lng, typeahead, or swlat/nelat)")
}

func Typeahead(c *fiber.Ctx) error {
	limit := c.Query("limit", "10")
	limit64, _ := strconv.ParseUint(limit, 10, 64)

	if limit64 > 10 {
		limit64 = 10
	}

	typeahead := c.Query("q")
	pconly := c.QueryBool("pconly", true)

	pcq := ""

	if pconly {
		pcq = "AND l1.type = '" + TYPE_POSTCODE + "'"
	}

	// We want to select full postcodes (with a space in them).
	typeahead = strings.ReplaceAll(typeahead, `\s`, "")

	locations := []Location{}

	if typeahead != "" {
		db := database.DBConn

		type locationWithArea struct {
			Location
			AreaLat float32 `json:"-" gorm:"column:arealat"`
			AreaLng float32 `json:"-" gorm:"column:arealng"`
		}

		var locs []locationWithArea
		db.Raw("SELECT l1.id, l1.name, l1.areaid, l1.lat, l1.lng, l1.type, l2.name as areaname, l2.lat as arealat, l2.lng as arealng "+
			"FROM locations l1 "+
			"LEFT JOIN locations l2 ON l2.id = l1.areaid "+
			"WHERE l1.name LIKE ? "+pcq+" AND l1.name LIKE '% %' LIMIT ?;",
			typeahead+"%",
			limit64).Scan(&locs)

		for i, l := range locs {
			locations = append(locations, l.Location)
			if l.Areaid > 0 {
				locations[i].Area = &AreaInfo{
					ID:   l.Areaid,
					Name: l.Areaname,
					Lat:  l.AreaLat,
					Lng:  l.AreaLng,
				}
			}
		}

		// Fetch the groups near each postcode, in parallel
		var wg sync.WaitGroup
		wg.Add(len(locations))

		for i := range locations {
			go func(i int) {
				locations[i].GroupsNear = ClosestGroups(float64(locations[i].Lat), float64(locations[i].Lng), NEARBY, 10)
				wg.Done()
			}(i)
		}

		wg.Wait()

		return c.JSON(locations)
	}

	return fiber.NewError(fiber.StatusNotFound, "q parameter not found")
}

type Address struct {
	ID                       uint64 `json:"id"`
	Buildingname             string `json:"buildingname"`
	Buildingnumber           string `json:"buildingnumber"`
	Subbuildingname          string `json:"subbuildingname"`
	Departmentname           string `json:"departmentname"`
	Dependentlocality        string `json:"dependentlocality"`
	Dependentthoroughfare    string `json:"dependentthoroughfare"`
	Organisationname         string `json:"organisationname"`
	SubOrganisationindicator string `json:"suborganisationindicator"`
	Deliverypointsuffix      string `json:"deliverypointsuffix"`
	Udprn                    string `json:"udprn"`
	Posttown                 string `json:"posttown"`
	Postcodetype             string `json:"postcodetype"`
	Pobox                    string `json:"pobox"`
	Postcode                 string `json:"postcode"`
	Thoroughfaredescriptor   string `json:"thoroughfaredescriptor"`
}

func GetLocationAddresses(c *fiber.Ctx) error {
	if c.Params("id") != "" {
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			var addresses []Address
			db := database.DBConn

			db.Raw("SELECT paf_addresses.id,"+
				"locations.name as postcode, "+
				"buildingname, "+
				"buildingnumber, "+
				"p.subbuildingname, "+
				"departmentname, "+
				"dependentlocality, "+
				"doubledependentlocality, "+
				"dependentthoroughfaredescriptor, "+
				"organisationname, "+
				"suorganisationindicator, "+
				"deliverypointsuffix, "+
				"udprn, "+
				"posttown, "+
				"postcodetype, "+
				"pobox, "+
				"thoroughfaredescriptor "+
				"FROM paf_addresses "+
				"INNER JOIN locations ON locations.id = paf_addresses.postcodeid "+
				"LEFT JOIN paf_buildingname ON buildingnameid = paf_buildingname.id "+
				"LEFT JOIN paf_subbuildingname ON subbuildingnameid = paf_subbuildingname.id "+
				"LEFT JOIN paf_departmentname ON departmentnameid = paf_departmentname.id "+
				"LEFT JOIN paf_dependentlocality ON dependentlocalityid = paf_dependentlocality.id "+
				"LEFT JOIN paf_doubledependentlocality ON doubledependentlocalityid = paf_doubledependentlocality.id "+
				"LEFT JOIN paf_dependentthoroughfaredescriptor ON dependentthoroughfaredescriptorid = paf_dependentthoroughfaredescriptor.id "+
				"LEFT JOIN paf_organisationname ON organisationnameid = paf_organisationname.id "+
				"LEFT JOIN paf_pobox ON poboxid = paf_pobox.id "+
				"LEFT JOIN paf_posttown ON posttownid = paf_posttown.id "+
				"LEFT JOIN paf_subbuildingname p ON subbuildingnameid = p.id "+
				"LEFT JOIN paf_thoroughfaredescriptor ON thoroughfaredescriptorid = paf_thoroughfaredescriptor.id "+
				"WHERE paf_addresses.postcodeid = ?;", id).Scan(&addresses)

			// If buildingnumber is the same as buildingname, remove buildingnumber - this happens and causes dups.
			for i, address := range addresses {
				if address.Buildingnumber == address.Buildingname {
					addresses[i].Buildingnumber = ""
				}
			}

			if len(addresses) == 0 {
				// Force [] rather than null to be returned.
				return c.JSON(make([]string, 0))
			} else {
				return c.JSON(addresses)
			}
		}
	}

	return fiber.NewError(fiber.StatusBadRequest, "Valid id parameter required")
}
