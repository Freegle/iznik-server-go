package location

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	geo "github.com/kellydunn/golang-geo"
	"math"
	"sync"
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

type ClosestGroup struct {
	ID          uint64  `json:"id"`
	Nameshort   string  `json:"nameshort"`
	Namefull    string  `json:"namefull"`
	Namedisplay string  `json:"namedisplay"`
	Dist        float32 `json:"dist"`
}

func ClosestGroups(lat float64, lng float64, radius float64, limit int) []ClosestGroup {
	db := database.DBConn

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
	var currradius = math.Round(float64(radius)/16.0 + 0.5)
	var nelat, nelng, swlat, swlng float64
	var ret []ClosestGroup

	for {
		p := geo.NewPoint(lat, lng)
		ne := p.PointAtDistanceAndBearing(currradius, 45)
		nelat = ne.Lat()
		nelng = ne.Lng()
		sw := p.PointAtDistanceAndBearing(currradius, 225)
		swlat = sw.Lat()
		swlng = sw.Lng()

		db.Raw("SELECT id, nameshort, namefull, "+
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
			limit).Scan(&ret)

		for ix, group := range ret {
			if len(group.Namefull) > 0 {
				ret[ix].Namedisplay = group.Namefull
			} else {
				ret[ix].Namedisplay = group.Nameshort
			}
		}

		currradius = currradius * 2

		if len(ret) >= limit || currradius >= radius {
			break
		}
	}

	return ret
}

func ClosestSingleGroup(lat float64, lng float64, radius float64) *ClosestGroup {
	// As above, but just return the first group we find.  Because this is Go we can fire off these requests in
	// parallel and just stop when we get the first result.  This reduces latency significantly, even though it's a
	// bit mean to the database server.
	db := database.DBConn

	var currradius = math.Round(float64(radius)/16.0 + 0.5)
	var result ClosestGroup
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
	found := false

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
			var ret ClosestGroup
			var nelat, nelng, swlat, swlng float64
			p := geo.NewPoint(lat, lng)
			ne := p.PointAtDistanceAndBearing(currradius, 45)
			nelat = ne.Lat()
			nelng = ne.Lng()
			sw := p.PointAtDistanceAndBearing(currradius, 225)
			swlat = sw.Lat()
			swlng = sw.Lng()

			db.Raw("SELECT id, nameshort, namefull, "+
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
				1).Scan(&ret)

			mu.Lock()
			defer mu.Unlock()

			if !found {
				if ret.ID > 0 {
					// We found one.
					count--

					found = true
					result = ret
					defer wg.Done()

					if len(ret.Namefull) > 0 {
						ret.Namedisplay = ret.Namefull
					} else {
						ret.Namedisplay = ret.Nameshort
					}
				} else {
					count--

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

	if result.ID > 0 {
		return &result
	} else {
		return nil
	}
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
