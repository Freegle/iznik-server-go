package job

import (
	"context"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	geo "github.com/kellydunn/golang-geo"
	"regexp"
	"strconv"
	"sync"
	"time"
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
	// This reduces latency significantly, even though it's a bit mean to the database server.  To cancel the queries
	// properly we need to use the Pool.
	ret := []Job{}
	best := []Job{}

	lat, _ := strconv.ParseFloat(c.Query("lat"), 32)
	lng, _ := strconv.ParseFloat(c.Query("lng"), 32)
	category := c.Query("category", "")

	// Remove any characters other than letters, space and forward slash.
	r := regexp.MustCompile(`[^a-zA-Z/ ]+`)
	category = r.ReplaceAllString(category, "")

	categoryq := "IS NOT NULL"

	if len(category) > 0 {
		categoryq = "REGEXP '(^|;)" + category + ".*'"
	}

	if lat != 0 || lng != 0 {
		step := float64(10)
		ambit := step

		var mu sync.Mutex
		var wg sync.WaitGroup
		done := false
		count := 0

		for {
			ambit = ambit * 2
			count++

			if ambit > JOBS_DISTANCE {
				break
			}
		}

		var cancels []context.CancelFunc

		ambit = step

		wg.Add(1)

		for {
			// Use a timeout context - partly so that we don't wait for too long, and partly so that we can
			// cancel queries if we get enough results.
			timeoutContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			cancels = append(cancels, cancel)

			go func(ambit float64) {
				var nelat, nelng, swlat, swlng float64
				var these []Job

				// Get an exclusive connection.
				db, err := database.Pool.Conn(timeoutContext)

				if err != nil {
					return
				}

				p := geo.NewPoint(float64(lat), float64(lng))
				ne := p.PointAtDistanceAndBearing(ambit, 45)
				nelat = ne.Lat()
				nelng = ne.Lng()
				sw := p.PointAtDistanceAndBearing(ambit, 225)
				swlat = sw.Lat()
				swlng = sw.Lng()

				lats := fmt.Sprint(lat)
				lngs := fmt.Sprint(lng)
				nelats := fmt.Sprint(nelat)
				nelngs := fmt.Sprint(nelng)
				swlats := fmt.Sprint(swlat)
				swlngs := fmt.Sprint(swlng)
				srids := fmt.Sprint(utils.SRID)

				ambitStr := strconv.FormatFloat(ambit, 'f', 0, 64)

				// We sort by cpc/dist, so that we will tend to show better paying jobs a bit further away.
				sql := "SELECT " + ambitStr + " AS ambit, " +
					"ST_Distance(geometry, ST_SRID(POINT(" + lats + ", " + lngs + "), " + srids + ")) AS dist, " +
					"CASE WHEN ST_Dimension(geometry) < 2 THEN 0 ELSE ST_Area(geometry) END AS area, " +
					"jobs.id, jobs.url, jobs.title, jobs.location, jobs.body, jobs.job_reference, jobs.cpc, jobs.clickability " +
					"FROM `jobs` " +
					"WHERE ST_Within(geometry, ST_SRID(POLYGON(LINESTRING(" +
					"POINT(" + swlngs + ", " + swlats + "), " +
					"POINT(" + swlngs + ", " + nelats + "), " +
					"POINT(" + nelngs + ", " + nelats + "), " +
					"POINT(" + nelngs + ", " + swlats + "), " +
					"POINT(" + swlngs + ", " + swlats + "))), " +
					srids + ")) " +
					"AND (ST_Dimension(geometry) < 2 OR ST_Area(geometry) / ST_Area(ST_SRID(POLYGON(LINESTRING(" +
					"POINT(" + swlngs + ", " + swlats + "), " +
					"POINT(" + swlngs + ", " + nelats + "), " +
					"POINT(" + nelngs + ", " + nelats + "), " +
					"POINT(" + nelngs + ", " + swlats + "), " +
					"POINT(" + swlngs + ", " + swlats + "))), " +
					srids + ")) < 2) " +
					"AND cpc >= " + fmt.Sprint(JOBS_MINIMUM_CPC) + " " +
					"AND visible = 1 " +
					"AND category " + categoryq + " " +
					"ORDER BY cpc / (CASE WHEN dist > 0 THEN dist ELSE 0.01 END) DESC, dist ASC, posted_at DESC LIMIT " + fmt.Sprint(JOBS_LIMIT) + ";"

				rows, err := db.QueryContext(timeoutContext, sql)

				// Return the connection to the pool.
				defer db.Close()

				// We might be cancelled/timed out, in which case we have no rows to process.
				if err == nil {
					for rows.Next() {
						var job Job
						err = rows.Scan(
							&job.Ambit,
							&job.Dist,
							&job.Area,
							&job.ID,
							&job.Url,
							&job.Title,
							&job.Location,
							&job.Body,
							&job.Reference,
							&job.CPC,
							&job.Clickability)

						these = append(these, job)
					}
				}

				mu.Lock()
				defer mu.Unlock()

				if !done {
					if len(these) >= len(best) {
						best = these
					}

					count--

					if len(best) >= JOBS_LIMIT || count == 0 {
						// Either we found enough or we have finished looking.  Either way, stop and take the best we
						// have found.
						ret = best
						done = true
						defer wg.Done()
					}
				}
			}(ambit)

			ambit = ambit * 2

			if ambit > JOBS_DISTANCE {
				break
			}
		}

		wg.Wait()

		// Cancel any outstanding ops.
		for _, cancel := range cancels {
			defer func() {
				go cancel()
			}()
		}
	}

	if len(ret) == 0 {
		// Force [] rather than null to be returned.
		return c.JSON(make([]string, 0))
	} else {
		return c.JSON(ret)
	}
}

func GetJob(c *fiber.Ctx) error {
	var job Job

	if c.Params("id") != "" {
		id, err := strconv.ParseUint(c.Params("id"), 10, 64)

		if err == nil {
			db := database.DBConn

			db.Raw("SELECT jobs.id, jobs.url, jobs.title, jobs.location, jobs.body, jobs.job_reference, jobs.cpc, jobs.clickability "+
				"FROM `jobs` "+
				"WHERE id = ? "+
				"AND visible = 1;",
				id).Scan(&job)

			if job.ID != 0 {
				return c.JSON(job)
			}
		}
	}

	return fiber.NewError(fiber.StatusNotFound, "Job not found")
}
