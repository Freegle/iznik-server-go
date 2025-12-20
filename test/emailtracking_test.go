package test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/emailtracking"
	"github.com/stretchr/testify/assert"
)

// getTestUserSite returns the user site URL for tests
func getTestUserSite() string {
	site := os.Getenv("USER_SITE")
	if site == "" {
		site = "www.ilovefreegle.org"
	}
	return "https://" + site
}

// getTestImageDomain returns the image domain URL for tests
func getTestImageDomain() string {
	domain := os.Getenv("IMAGE_DOMAIN")
	if domain == "" {
		domain = "images.ilovefreegle.org"
	}
	return "https://" + domain
}

// createTestTrackingRecord creates a test email tracking record for testing
func createTestTrackingRecord(t *testing.T) *emailtracking.EmailTracking {
	db := database.DBConn

	tracking := &emailtracking.EmailTracking{
		TrackingID:     "test-" + randomString(16),
		EmailType:      "Test",
		RecipientEmail: "test@example.com",
	}

	result := db.Create(tracking)
	assert.NoError(t, result.Error)

	return tracking
}

// randomString generates a random string for test tracking IDs
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}

// cleanupTestTracking removes test tracking records
func cleanupTestTracking(t *testing.T, trackingID string) {
	db := database.DBConn
	db.Where("tracking_id = ?", trackingID).Delete(&emailtracking.EmailTracking{})
}

func TestEmailTrackingPixel(t *testing.T) {
	// Create test tracking record
	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	// Request the tracking pixel using bland path
	req := httptest.NewRequest("GET", "/e/d/p/"+tracking.TrackingID, nil)
	resp, err := getApp().Test(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "image/gif", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-store, no-cache, must-revalidate, max-age=0", resp.Header.Get("Cache-Control"))

	// Verify the tracking record was updated
	db := database.DBConn
	var updated emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&updated)

	assert.NotNil(t, updated.OpenedAt)
	assert.Equal(t, "pixel", *updated.OpenedVia)
}

func TestEmailTrackingPixelInvalidID(t *testing.T) {
	// Request with non-existent tracking ID
	req := httptest.NewRequest("GET", "/e/d/p/nonexistent123", nil)
	resp, err := getApp().Test(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode) // Still returns GIF
	assert.Equal(t, "image/gif", resp.Header.Get("Content-Type"))
}

func TestEmailTrackingClick(t *testing.T) {
	// Create test tracking record
	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	destinationURL := getTestUserSite() + "/give"
	encodedURL := base64.StdEncoding.EncodeToString([]byte(destinationURL))

	// Request click tracking using bland path
	req := httptest.NewRequest("GET", "/e/d/r/"+tracking.TrackingID+"?url="+encodedURL+"&p=cta_button&a=cta", nil)
	resp, err := getApp().Test(req, -1) // -1 to not follow redirects

	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode) // 302 redirect
	assert.Equal(t, destinationURL, resp.Header.Get("Location"))

	// Verify the tracking record was updated
	db := database.DBConn
	var updated emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&updated)

	assert.NotNil(t, updated.OpenedAt)
	assert.Equal(t, "click", *updated.OpenedVia)
	assert.NotNil(t, updated.ClickedAt)
	assert.Equal(t, destinationURL, *updated.ClickedLink)
	assert.Equal(t, uint16(1), updated.LinksClicked)

	// Verify click record was created
	var clicks []emailtracking.EmailTrackingClick
	db.Where("email_tracking_id = ?", updated.ID).Find(&clicks)
	assert.Equal(t, 1, len(clicks))
	assert.Equal(t, destinationURL, clicks[0].LinkURL)
	assert.Equal(t, "cta_button", *clicks[0].LinkPosition)
	assert.Equal(t, "cta", *clicks[0].Action)
}

func TestEmailTrackingClickInvalidURL(t *testing.T) {
	// Create test tracking record
	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	// Request with empty URL using bland path
	req := httptest.NewRequest("GET", "/e/d/r/"+tracking.TrackingID, nil)
	resp, err := getApp().Test(req, -1)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/", resp.Header.Get("Location")) // Redirects to home
}

func TestEmailTrackingImage(t *testing.T) {
	// Create test tracking record
	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	imageURL := getTestImageDomain() + "/test.jpg"
	encodedURL := base64.StdEncoding.EncodeToString([]byte(imageURL))

	// Request image tracking using bland path
	req := httptest.NewRequest("GET", "/e/d/i/"+tracking.TrackingID+"?url="+encodedURL+"&p=item_3&s=75", nil)
	resp, err := getApp().Test(req, -1)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, imageURL, resp.Header.Get("Location"))

	// Verify the tracking record was updated
	db := database.DBConn
	var updated emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&updated)

	assert.NotNil(t, updated.OpenedAt)
	assert.Equal(t, "image", *updated.OpenedVia)
	assert.Equal(t, uint8(75), *updated.ScrollDepthPercent)
	assert.Equal(t, uint16(1), updated.ImagesLoaded)

	// Verify image load record was created
	var images []emailtracking.EmailTrackingImage
	db.Where("email_tracking_id = ?", updated.ID).Find(&images)
	assert.Equal(t, 1, len(images))
	assert.Equal(t, "item_3", images[0].ImagePosition)
	assert.Equal(t, uint8(75), *images[0].EstimatedScrollPercent)
}

func TestEmailTrackingMDN(t *testing.T) {
	// MDN read receipts are handled by PHP's incoming mail handler
	// which updates the database directly. This test verifies the data model
	// supports MDN tracking by simulating what PHP would do.

	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	// Simulate PHP updating the record when MDN email is received
	db := database.DBConn
	now := time.Now()
	openedVia := "mdn"
	db.Model(&emailtracking.EmailTracking{}).
		Where("tracking_id = ?", tracking.TrackingID).
		Updates(map[string]interface{}{
			"opened_at":  now,
			"opened_via": openedVia,
		})

	// Verify the tracking record was updated
	var updated emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&updated)

	assert.NotNil(t, updated.OpenedAt)
	assert.Equal(t, "mdn", *updated.OpenedVia)
}

func TestEmailTrackingUnsubscribe(t *testing.T) {
	// Create test tracking record
	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	// Unsubscribe is tracked via the click endpoint with action=unsubscribe
	unsubscribeURL := getTestUserSite() + "/unsubscribe"
	encodedURL := base64.StdEncoding.EncodeToString([]byte(unsubscribeURL))

	req := httptest.NewRequest("GET", "/e/d/r/"+tracking.TrackingID+"?url="+encodedURL+"&a=unsubscribe", nil)
	resp, err := getApp().Test(req, -1)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, unsubscribeURL, resp.Header.Get("Location"))

	// Verify the tracking record was updated
	db := database.DBConn
	var updated emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&updated)

	assert.NotNil(t, updated.UnsubscribedAt)
	assert.NotNil(t, updated.ClickedAt)

	// Verify click record was created with unsubscribe action
	var clicks []emailtracking.EmailTrackingClick
	db.Where("email_tracking_id = ?", updated.ID).Find(&clicks)
	assert.Equal(t, 1, len(clicks))
	assert.Equal(t, "unsubscribe", *clicks[0].Action)
}

func TestEmailTrackingStatsUnauthorized(t *testing.T) {
	// Request without authentication
	req := httptest.NewRequest("GET", "/api/email/stats", nil)
	resp, err := getApp().Test(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestEmailTrackingStatsWithAuth(t *testing.T) {
	// Create a support user with token using existing test utilities
	prefix := uniquePrefix("emailstats")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// Request with authentication
	req := httptest.NewRequest("GET", "/api/email/stats?jwt="+token, nil)
	resp, err := getApp().Test(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Contains(t, result, "stats")
	assert.Contains(t, result, "period")
}

func TestEmailTrackingUserEmailsUnauthorized(t *testing.T) {
	// Request without authentication
	req := httptest.NewRequest("GET", "/api/email/user/123", nil)
	resp, err := getApp().Test(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestEmailTrackingUserEmailsWithAuth(t *testing.T) {
	// Create a support user with token using existing test utilities
	prefix := uniquePrefix("emailuser")
	userID := CreateTestUser(t, prefix, "Support")
	_, token := CreateTestSession(t, userID)

	// Create a test tracking record for the support user
	db := database.DBConn
	tracking := &emailtracking.EmailTracking{
		TrackingID:     "usertest-" + randomString(16),
		EmailType:      "Test",
		UserID:         &userID,
		RecipientEmail: "test@example.com",
	}
	db.Create(tracking)
	defer db.Where("tracking_id = ?", tracking.TrackingID).Delete(&emailtracking.EmailTracking{})

	// Request user emails with authentication
	req := httptest.NewRequest("GET", "/api/email/user/"+strconv.FormatUint(userID, 10)+"?jwt="+token, nil)
	resp, err := getApp().Test(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Parse response
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Contains(t, result, "emails")
	assert.Contains(t, result, "total")
}

func TestEmailTrackingOnlyRecordsFirstOpen(t *testing.T) {
	// Create test tracking record
	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	// First open via pixel
	req1 := httptest.NewRequest("GET", "/e/d/p/"+tracking.TrackingID, nil)
	getApp().Test(req1)

	// Get first opened_at
	db := database.DBConn
	var first emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&first)
	firstOpenedAt := first.OpenedAt
	firstOpenedVia := first.OpenedVia

	// Second open via image (should not update opened_via)
	imageURL := getTestImageDomain() + "/test.jpg"
	encodedURL := base64.StdEncoding.EncodeToString([]byte(imageURL))
	req2 := httptest.NewRequest("GET", "/e/d/i/"+tracking.TrackingID+"?url="+encodedURL+"&p=item_1", nil)
	getApp().Test(req2, -1)

	// Verify opened_at wasn't changed
	var second emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&second)

	assert.Equal(t, firstOpenedAt, second.OpenedAt)
	assert.Equal(t, firstOpenedVia, second.OpenedVia)
	assert.Equal(t, "pixel", *second.OpenedVia)
}

func TestEmailTrackingMultipleClicks(t *testing.T) {
	// Create test tracking record
	tracking := createTestTrackingRecord(t)
	defer cleanupTestTracking(t, tracking.TrackingID)

	// First click
	firstClickURL := getTestUserSite()
	url1 := base64.StdEncoding.EncodeToString([]byte(firstClickURL))
	req1 := httptest.NewRequest("GET", "/e/d/r/"+tracking.TrackingID+"?url="+url1+"&p=link1", nil)
	getApp().Test(req1, -1)

	// Second click
	url2 := base64.StdEncoding.EncodeToString([]byte(getTestUserSite() + "/give"))
	req2 := httptest.NewRequest("GET", "/e/d/r/"+tracking.TrackingID+"?url="+url2+"&p=link2", nil)
	getApp().Test(req2, -1)

	// Verify click count
	db := database.DBConn
	var updated emailtracking.EmailTracking
	db.Where("tracking_id = ?", tracking.TrackingID).First(&updated)

	assert.Equal(t, uint16(2), updated.LinksClicked)

	// First clicked_link should be preserved
	assert.Equal(t, firstClickURL, *updated.ClickedLink)

	// Verify both click records exist
	var clicks []emailtracking.EmailTrackingClick
	db.Where("email_tracking_id = ?", updated.ID).Find(&clicks)
	assert.Equal(t, 2, len(clicks))
}
