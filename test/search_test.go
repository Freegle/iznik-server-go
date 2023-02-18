package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/message"
	"github.com/freegle/iznik-server-go/router"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestGetWords(t *testing.T) {
	words := message.GetWords("Old sofa which is green")
	assert.Equal(t, 2, len(words))
	assert.Equal(t, "sofa", words[0])
	assert.Equal(t, "which", words[1])
}

func TestSearchExact(t *testing.T) {
	m := GetMessage(t)

	// Search on first word in subject - should find exact match.
	words := message.GetWords(m.Subject)

	results := message.GetWordsExact(database.DBConn, words[0], 100, nil, "All", 0, 0, 0, 0)

	// We might not find the one we were looking for, if it's a common term.  But we've tested that a basic
	// search finds something.
	assert.Greater(t, len(results), 0)
	assert.Equal(t, words[0], results[0].Matchedon.Word)
}

func TestSearchTypo(t *testing.T) {
	results := message.GetWordsTypo(database.DBConn, "basic", 100, nil, "All", 0, 0, 0, 0)
	assert.Greater(t, len(results), 0)
	assert.Equal(t, "basic", results[0].Matchedon.Word)
}

func TestSearchStarts(t *testing.T) {
	m := GetMessage(t)

	// Search on first word in subject - should find exact match.
	words := message.GetWords(m.Subject)

	// Get the first 3 letters.
	results := message.GetWordsStarts(database.DBConn, words[0][:3], 100, nil, "All", 0, 0, 0, 0)

	// We might not find the one we were looking for, if it's a common term.  But we've tested that a basic
	// search finds something.
	assert.Greater(t, len(results), 0)
	assert.Equal(t, words[0][:3], results[0].Matchedon.Word[:3])
}

func TestAPISearch(t *testing.T) {
	app := fiber.New()
	database.InitDatabase()
	router.SetupRoutes(app)

	// Search on first word in subject - should find exact match.
	m := GetMessage(t)
	words := message.GetWords(m.Subject)

	resp, _ := app.Test(httptest.NewRequest("GET", "/api/message/search/"+words[0], nil))
	assert.Equal(t, 200, resp.StatusCode)

	var results []message.SearchResult
	json2.Unmarshal(rsp(resp), &results)
	assert.Greater(t, len(results), 0)
	assert.Equal(t, words[0], results[0].Matchedon.Word)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/message/search/tset", nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &results)
	assert.Greater(t, len(results), 0)

	resp, _ = app.Test(httptest.NewRequest("GET", "/api/message/search/£78jhdfhjdsfhjsafhsjjdsfkhjk", nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &results)
	assert.Equal(t, len(results), 0)

	groupid := strconv.FormatUint(m.MessageGroups[0].Groupid, 10)
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/message/search/"+words[0]+"?groupids="+groupid, nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &results)
	assert.Greater(t, len(results), 0)

	if len(results) > 0 {
		assert.Equal(t, words[0], results[0].Matchedon.Word)
	}
}
