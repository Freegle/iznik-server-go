package test

import (
	json2 "encoding/json"
	"fmt"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/job"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

func TestJobs(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/job?lat=52.5833189&lng=-2.0455619", nil))
	assert.Equal(t, 200, resp.StatusCode)

	var jobs []job.Job
	json2.Unmarshal(rsp(resp), &jobs)
	assert.Greater(t, len(jobs), 0)

	// Get one of them.
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/job/"+fmt.Sprint(jobs[0].ID), nil))
	assert.Equal(t, 200, resp.StatusCode)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/job/0", nil))
	assert.Equal(t, 404, resp.StatusCode)
}
