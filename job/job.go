package job

import (
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	geo "github.com/kellydunn/golang-geo"
	"strconv"
	"sync"
)

type Job struct {
	ID           uint64  `json:"id" gorm:"primary_key"`
	Ambit        float64 `json:"ambit"`
	Dist         float64 `json:"dist"`
	Area         float64 `json:"area"`
	Url          string  `json:"url"`
	Title        string  `json:"title"`
	Location     string  `json:"location"`
	Body         string  `json:"body"`
	Reference    string  `json:"reference"`
	CPC          float64 `json:"cpc"`
	Clickability float64 `json:"clickability"`
}

const JOBS_LIMIT = 50
const JOBS_DISTANCE = 64
const JOBS_MINIMUM_CPC = 0.10

func GetJobs(c *fiber.Ctx) error {
	// To make efficient use of the spatial index we construct a box around our lat/lng, and search for jobs
	// where the geometry overlaps it.  We keep expanding our box until we find enough.
	//
	// We used to double the ambit each time, but that led to long queries, probably because we would suddenly
	// include a couple of cities or something.
	//
	// Because this is Go we can fire off these requests in parallel and just stop when we get enough results.
	// This reduces latency significantly, even though it's a bit mean to the database server.
	ret := []Job{}
	best := []Job{}

	lat, _ := strconv.ParseFloat(c.Query("lat"), 32)
	lng, _ := strconv.ParseFloat(c.Query("lng"), 32)

	if lat != 0 || lng != 0 {
		step := float64(2)
		ambit := step

		var mu sync.Mutex
		var wg sync.WaitGroup
		done := false
		count := 0

		for {
			ambit = ambit + step
			count++

			if ambit > JOBS_DISTANCE {
				break
			}
		}

		ambit = step

		wg.Add(1)

		for {
			go func(ambit float64) {
				db := database.DBConn
				var nelat, nelng, swlat, swlng float64
				var these []Job

				p := geo.NewPoint(float64(lat), float64(lng))
				ne := p.PointAtDistanceAndBearing(ambit, 45)
				nelat = ne.Lat()
				nelng = ne.Lng()
				sw := p.PointAtDistanceAndBearing(ambit, 225)
				swlat = sw.Lat()
				swlng = sw.Lng()

				// convert ambit to string
				ambitStr := strconv.FormatFloat(ambit, 'f', 0, 64)

				db.Raw("SELECT "+ambitStr+" AS ambit, "+
					"ST_Distance(geometry, ST_SRID(POINT(?, ?), ?)) AS dist, "+
					"CASE WHEN ST_Dimension(geometry) < 2 THEN 0 ELSE ST_Area(geometry) END AS area, "+
					"jobs.id, jobs.url, jobs.title, jobs.location, jobs.body, jobs.job_reference, jobs.cpc, jobs.clickability "+
					"FROM `jobs` "+
					"WHERE ST_Within(geometry, ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?)) "+
					"AND (ST_Dimension(geometry) < 2 OR ST_Area(geometry) / ST_Area(ST_SRID(POLYGON(LINESTRING(POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?), POINT(?, ?))), ?)) < 2) "+
					"AND cpc >= ? "+
					"AND visible = 1 "+
					"ORDER BY cpc DESC, dist ASC, posted_at DESC LIMIT ?;",
					lng,
					lat,
					utils.SRID,
					swlng, swlat,
					swlng, nelat,
					nelng, nelat,
					nelng, swlat,
					swlng, swlat,
					utils.SRID,
					swlng, swlat,
					swlng, nelat,
					nelng, nelat,
					nelng, swlat,
					swlng, swlat,
					utils.SRID,
					JOBS_MINIMUM_CPC,
					JOBS_LIMIT).Scan(&these)

				mu.Lock()
				defer mu.Unlock()

				if !done {
					if len(these) >= len(best) {
						best = these
					}

					count--

					if len(best) >= JOBS_LIMIT {
						// Either we found enough or we have finished.  Either way, stop and take the best we have.
						ret = best
						done = true
						defer wg.Done()
					}
				}
			}(ambit)

			ambit = ambit + step

			if ambit > JOBS_DISTANCE {
				break
			}
		}

		wg.Wait()
	}

	return c.JSON(ret)
}
