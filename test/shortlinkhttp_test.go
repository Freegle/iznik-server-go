package test

import (
	"net/http/httptest"
	"testing"

	"github.com/freegle/iznik-server-go/database"
	"github.com/stretchr/testify/assert"
)

func TestShortlinkRedirect_ValidName(t *testing.T) {
	prefix := uniquePrefix("shortlnk")
	db := database.DBConn

	// Create a shortlink with a direct URL.
	linkName := "test_" + prefix
	linkURL := "https://example.com/test-redirect"
	db.Exec("INSERT INTO shortlinks (name, type, url) VALUES (?, 'Other', ?)", linkName, linkURL)

	var linkID uint64
	db.Raw("SELECT id FROM shortlinks WHERE name = ?", linkName).Scan(&linkID)
	assert.NotZero(t, linkID, "Shortlink should be created")

	// Request the redirect endpoint.
	req := httptest.NewRequest("GET", "/shortlink?name="+linkName, nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)
	assert.Equal(t, linkURL, resp.Header.Get("Location"))

	// Verify click was recorded.
	var clicks int64
	db.Raw("SELECT clicks FROM shortlinks WHERE id = ?", linkID).Scan(&clicks)
	assert.Equal(t, int64(1), clicks, "Click should be recorded")

	// Clean up.
	db.Exec("DELETE FROM shortlink_clicks WHERE shortlinkid = ?", linkID)
	db.Exec("DELETE FROM shortlinks WHERE id = ?", linkID)
}

func TestShortlinkRedirect_UnknownName(t *testing.T) {
	// Unknown name should redirect to user site.
	req := httptest.NewRequest("GET", "/shortlink?name=nonexistent_xyz_999", nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "ilovefreegle.org", "Should redirect to user site")
}

func TestShortlinkRedirect_MissingName(t *testing.T) {
	// Missing name parameter should redirect to user site.
	req := httptest.NewRequest("GET", "/shortlink", nil)
	resp, err := getApp().Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode)

	location := resp.Header.Get("Location")
	assert.Contains(t, location, "ilovefreegle.org", "Should redirect to user site")
}
