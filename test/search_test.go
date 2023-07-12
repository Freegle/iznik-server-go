package test

import (
	json2 "encoding/json"
	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/message"
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

	results := message.GetWordsExact(database.DBConn, words, 100, nil, "All", 0, 0, 0, 0)

	// We might not find the one we were looking for, if it's a common term.  But we've tested that a basic
	// search finds something.
	assert.Greater(t, len(results), 0)
	assert.Equal(t, m.ID, results[0].Msgid)
	assert.Contains(t, words, results[0].Matchedon.Word)
}

func TestSearchTypo(t *testing.T) {
	results := message.GetWordsTypo(database.DBConn, []string{"basic"}, 100, nil, "All", 0, 0, 0, 0)
	assert.Greater(t, len(results), 0)
}

func TestSearchStarts(t *testing.T) {
	m := GetMessage(t)

	// Search on first word in subject - should find exact match.
	words := message.GetWords(m.Subject)

	// Get the first 3 letters.
	results := message.GetWordsStarts(database.DBConn, []string{words[0][:3]}, 100, nil, "All", 0, 0, 0, 0)

	// We might not find the one we were looking for, if it's a common term.  But we've tested that a basic
	// search finds something.
	assert.Greater(t, len(results), 0)
	assert.Equal(t, words[0][:3], results[0].Matchedon.Word[:3])
}

func TestAPISearch(t *testing.T) {
	// Use token so that we record search history.
	_, token := GetUserWithToken(t)

	// Search on first word in subject - should find exact match.
	m := GetMessage(t)
	words := message.GetWords(m.Subject)

	resp, _ := getApp().Test(httptest.NewRequest("GET", "/api/message/search/"+words[0]+"?jwt="+token, nil), 60000)
	assert.Equal(t, 200, resp.StatusCode)

	var results []message.SearchResult
	json2.Unmarshal(rsp(resp), &results)
	assert.Greater(t, len(results), 0)
	assert.Equal(t, words[0], results[0].Matchedon.Word)

	// This is slow.  We pass a high timeout, because if the request times out what happens is that we get a very
	// cryptic crash in the test framework.
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/search/tset", nil), 60000)
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &results)
	assert.Greater(t, len(results), 0)

	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/search/£78jhdfhjdsfhjsafhsjjdsfkhjk", nil), 60000)
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &results)
	assert.Equal(t, len(results), 0)

	groupid := strconv.FormatUint(m.MessageGroups[0].Groupid, 10)
	resp, _ = getApp().Test(httptest.NewRequest("GET", "/api/message/search/"+words[0]+"?groupids="+groupid, nil))
	assert.Equal(t, 200, resp.StatusCode)

	json2.Unmarshal(rsp(resp), &results)
	assert.Greater(t, len(results), 0)

	if len(results) > 0 {
		assert.Equal(t, words[0], results[0].Matchedon.Word)
	}
}
