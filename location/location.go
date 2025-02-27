package location

import (
	"encoding/json"
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
const NEARBY = 50      // In miles.
const QUITENEARBY = 15 // In miles.

type Location struct {
	ID         uint64         `json:"id"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Lat        float32        `json:"lat"`
	Lng        float32        `json:"lng"`
	Areaid     uint64         `json:"areaid"`
	Areaname   string         `json:"areaname"`
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

			db.Raw("SELECT id, nameshort, namefull, settings, "+
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
						defer wg.Done()
					}
				} else {
					if count == 0 {
						// We've run out of areas to search.
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
		// Looking for a specific user.
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			loc := FetchSingle(id)

			if groupsnear {
				// TODO
				loc.GroupsNear = ClosestGroups(float64(loc.Lat), float64(loc.Lng), NEARBY, 1)
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

func Typeahead(c *fiber.Ctx) error {
	limit := c.Query("limit", "10")
	limit64, _ := strconv.ParseUint(limit, 10, 64)
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
		db.Raw("SELECT l1.id, l1.name, l1.areaid, l1.lat, l1.lng, l1.type, l2.name as areaname "+
			"FROM locations l1 "+
			"LEFT JOIN locations l2 ON l2.id = l1.areaid "+
			"WHERE l1.name LIKE ? "+pcq+" AND l1.name LIKE '% %' LIMIT ?;",
			typeahead+"%",
			limit64).Scan(&locations)

		return c.JSON(locations)
	}

	return fiber.NewError(fiber.StatusNotFound, "q parameter not found")
}
