package test

import (
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/router"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
	"os"
)

var app *fiber.App

func init() {
	// Set environment variables needed for tests
	os.Setenv("LOVEJUNK_PARTNER_KEY", "testkey123")

	app = fiber.New()
	app.Use(user.NewAuthMiddleware(user.Config{}))
	database.InitDatabase()

	// Ensure required reference data exists for location tests
	setupLocationTestData()

	// Verify required tables exist (created by Laravel migrations in iznik-batch)
	verifyRequiredTables()

	// Set up swagger routes BEFORE other API routes (same as main.go)
	// Handle swagger redirect - redirect exact /swagger path to /swagger/index.html
	app.Get("/swagger", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html", 302)
	})

	// Serve swagger static files from swagger directory
	// Use absolute path to ensure it works regardless of where tests are run from
	swaggerPath := "/app/swagger"
	if _, err := os.Stat(swaggerPath); os.IsNotExist(err) {
		// Fallback to relative path if absolute doesn't exist (for local development)
		swaggerPath = "../swagger"
	}
	app.Static("/swagger", swaggerPath, fiber.Static{
		Index: "index.html",
	})

	// Set up all other API routes
	router.SetupRoutes(app)

	// NOTE: Tests now create their own data using factory functions in testUtils.go
	// Location reference data is set up by setupLocationTestData()
}

// setupLocationTestData ensures the required reference data exists for location tests
// This data is idempotent - safe to run multiple times
func setupLocationTestData() {
	db := database.DBConn

	// Create area location first (for areaname in ClosestPostcode query)
	db.Exec(`INSERT INTO locations (id, osm_id, name, type, osm_place, gridid, postcodeid, areaid, canon, popularity, osm_amenity, osm_shop, maxdimension, lat, lng, timestamp, geometry)
		VALUES (999999, '999999', 'Edinburgh', 'Area', 0, NULL, NULL, NULL, 'edinburgh', 0, 0, 0, '0.1', '55.957571', '-3.205333', '2016-08-23 06:01:25',
		ST_GeomFromText('POINT(-3.205333 55.957571)', 4326))
		ON DUPLICATE KEY UPDATE name='Edinburgh', type='Area'`)

	// Edinburgh postcode area for TestClosest, TestTypeahead, TestLatLng
	// These are real UK postcodes needed for the location API tests
	db.Exec(`INSERT INTO locations (id, osm_id, name, type, osm_place, gridid, postcodeid, areaid, canon, popularity, osm_amenity, osm_shop, maxdimension, lat, lng, timestamp, geometry)
		VALUES (1687412, '189543628', 'SA65 9ET', 'Postcode', 0, NULL, NULL, 999999, 'sa659et', 0, 0, 0, '0.002916', '52.006292', '-4.939858', '2016-08-23 06:01:25',
		ST_GeomFromText('POINT(-4.939858 52.006292)', 4326))
		ON DUPLICATE KEY UPDATE areaid=999999`)

	// Edinburgh postcodes for typeahead tests (EH3 area)
	db.Exec(`INSERT INTO locations (id, osm_id, name, type, osm_place, gridid, postcodeid, areaid, canon, popularity, osm_amenity, osm_shop, maxdimension, lat, lng, timestamp, geometry)
		VALUES (1000001, '100001', 'EH3 6SS', 'Postcode', 0, NULL, NULL, 999999, 'eh36ss', 0, 0, 0, '0.002916', '55.957571', '-3.205333', '2016-08-23 06:01:25',
		ST_GeomFromText('POINT(-3.205333 55.957571)', 4326))
		ON DUPLICATE KEY UPDATE areaid=999999`)

	db.Exec(`INSERT INTO locations (id, osm_id, name, type, osm_place, gridid, postcodeid, areaid, canon, popularity, osm_amenity, osm_shop, maxdimension, lat, lng, timestamp, geometry)
		VALUES (1000002, '100002', 'EH3 5AA', 'Postcode', 0, NULL, NULL, 999999, 'eh35aa', 0, 0, 0, '0.002916', '55.958000', '-3.206000', '2016-08-23 06:01:25',
		ST_GeomFromText('POINT(-3.206000 55.958000)', 4326))
		ON DUPLICATE KEY UPDATE areaid=999999`)

	// locations_spatial entries for spatial queries (used by ClosestPostcode)
	// Note: locations_spatial uses SRID 3857 (Web Mercator)
	db.Exec(`INSERT IGNORE INTO locations_spatial (locationid, geometry)
		VALUES (1687412, ST_GeomFromText('POINT(-4.939858 52.006292)', 3857))`)
	db.Exec(`INSERT IGNORE INTO locations_spatial (locationid, geometry)
		VALUES (1000001, ST_GeomFromText('POINT(-3.205333 55.957571)', 3857))`)
	db.Exec(`INSERT IGNORE INTO locations_spatial (locationid, geometry)
		VALUES (1000002, ST_GeomFromText('POINT(-3.206000 55.958000)', 3857))`)

	// PAF addresses for TestAddresses (linked to location 1687412)
	db.Exec(`INSERT IGNORE INTO paf_addresses (id, postcodeid, udprn) VALUES (102367696, 1687412, 50464672)`)

	// LoveJunk partner key for TestCreateChatMessageLoveJunk
	db.Exec("INSERT IGNORE INTO partners_keys (partner, `key`) VALUES ('lovejunk', 'testkey123')")
}

// verifyRequiredTables checks that tables created by Laravel migrations exist.
// These tables are not in schema.sql and are created by iznik-batch migrations.
// If missing, it means migrations haven't been run before tests.
func verifyRequiredTables() {
	db := database.DBConn
	for _, table := range []string{"background_tasks"} {
		var count int64
		db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", table).Scan(&count)
		if count == 0 {
			panic(fmt.Sprintf("Required table '%s' not found - ensure Laravel migrations have been run (setup-test-database.sh)", table))
		}
	}
}

func getApp() *fiber.App {
	// We use this so that we only initialise fiber once.
	return app
}
