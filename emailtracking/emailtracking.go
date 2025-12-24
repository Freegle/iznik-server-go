package emailtracking

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/user"
	"github.com/gofiber/fiber/v2"
)

// EmailTracking represents an email tracking record
type EmailTracking struct {
	ID                 uint64     `json:"id" gorm:"primaryKey;column:id"`
	TrackingID         string     `json:"tracking_id" gorm:"column:tracking_id"`
	EmailType          string     `json:"email_type" gorm:"column:email_type"`
	UserID             *uint64    `json:"userid" gorm:"column:userid"`
	GroupID            *uint64    `json:"groupid" gorm:"column:groupid"`
	RecipientEmail     string     `json:"recipient_email" gorm:"column:recipient_email"`
	Subject            *string    `json:"subject" gorm:"column:subject"`
	Metadata           *string    `json:"metadata" gorm:"column:metadata"`
	SentAt             *time.Time `json:"sent_at" gorm:"column:sent_at"`
	DeliveredAt        *time.Time `json:"delivered_at" gorm:"column:delivered_at"`
	BouncedAt          *time.Time `json:"bounced_at" gorm:"column:bounced_at"`
	BounceType         *string    `json:"bounce_type" gorm:"column:bounce_type"`
	OpenedAt           *time.Time `json:"opened_at" gorm:"column:opened_at"`
	OpenedVia          *string    `json:"opened_via" gorm:"column:opened_via"`
	ClickedAt          *time.Time `json:"clicked_at" gorm:"column:clicked_at"`
	ClickedLink        *string    `json:"clicked_link" gorm:"column:clicked_link"`
	ScrollDepthPercent *uint8     `json:"scroll_depth_percent" gorm:"column:scroll_depth_percent"`
	ImagesLoaded       uint16     `json:"images_loaded" gorm:"column:images_loaded"`
	LinksClicked       uint16     `json:"links_clicked" gorm:"column:links_clicked"`
	UnsubscribedAt     *time.Time `json:"unsubscribed_at" gorm:"column:unsubscribed_at"`
	CreatedAt          time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt          time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (EmailTracking) TableName() string {
	return "email_tracking"
}

// EmailTrackingClick represents a click event (includes button actions like unsubscribe)
type EmailTrackingClick struct {
	ID              uint64    `json:"id" gorm:"primary_key"`
	EmailTrackingID uint64    `json:"email_tracking_id"`
	LinkURL         string    `json:"link_url"`
	LinkPosition    *string   `json:"link_position"`
	Action          *string   `json:"action"` // e.g., "unsubscribe", "cta", "view_item"
	IPAddress       *string   `json:"ip_address"`
	UserAgent       *string   `json:"user_agent"`
	ClickedAt       time.Time `json:"clicked_at"`
}

func (EmailTrackingClick) TableName() string {
	return "email_tracking_clicks"
}

// EmailTrackingImage represents an image load event
type EmailTrackingImage struct {
	ID                     uint64    `json:"id" gorm:"primary_key"`
	EmailTrackingID        uint64    `json:"email_tracking_id"`
	ImagePosition          string    `json:"image_position"`
	EstimatedScrollPercent *uint8    `json:"estimated_scroll_percent"`
	LoadedAt               time.Time `json:"loaded_at"`
}

func (EmailTrackingImage) TableName() string {
	return "email_tracking_images"
}

// EmailStats represents aggregate statistics
type EmailStats struct {
	TotalSent        int64   `json:"total_sent"`
	Opened           int64   `json:"opened"`
	Clicked          int64   `json:"clicked"`
	Bounced          int64   `json:"bounced"`
	OpenRate         float64 `json:"open_rate"`
	ClickRate        float64 `json:"click_rate"`
	ClickToOpenRate  float64 `json:"click_to_open_rate"`
	BounceRate       float64 `json:"bounce_rate"`
}

// Transparent 1x1 GIF
var transparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00,
	0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21,
	0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00,
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x01, 0x44,
	0x00, 0x3b,
}

// generateTrackingID creates a random tracking ID
func generateTrackingID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Pixel serves a tracking pixel and records an email open
// @Router /e/d/p/{id} [get]
// @Summary Delivery pixel
// @Description Returns a 1x1 transparent GIF
// @Tags delivery
// @Produce image/gif
// @Param id path string true "ID"
// @Success 200 {file} file
func Pixel(c *fiber.Ctx) error {
	trackingID := c.Params("id")

	recordOpen(trackingID, "pixel")

	c.Set("Content-Type", "image/gif")
	c.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")

	return c.Send(transparentGIF)
}

// Click tracks a link click and redirects to the original URL
// Also handles button actions like unsubscribe via the 'a' (action) parameter
// @Router /e/d/r/{id} [get]
// @Summary Delivery redirect
// @Description Redirects to the destination URL
// @Tags delivery
// @Param id path string true "ID"
// @Param url query string true "Base64 encoded destination URL"
// @Param p query string false "Position identifier"
// @Param a query string false "Action type (e.g., unsubscribe, cta)"
// @Success 302 {string} string "Redirect"
func Click(c *fiber.Ctx) error {
	db := database.DBConn
	trackingID := c.Params("id")

	// Decode the URL
	urlEncoded := c.Query("url", "")
	urlBytes, err := base64.StdEncoding.DecodeString(urlEncoded)
	if err != nil {
		return c.Redirect("/")
	}
	destinationURL := string(urlBytes)

	// Validate URL
	if destinationURL == "" || !isValidRedirectURL(destinationURL) {
		return c.Redirect("/")
	}

	// Get tracking record
	var tracking EmailTracking
	result := db.Where("tracking_id = ?", trackingID).First(&tracking)
	if result.Error != nil {
		return c.Redirect(destinationURL)
	}

	position := c.Query("p", "")
	action := c.Query("a", "")
	ipAddress := c.IP()
	userAgent := c.Get("User-Agent")

	// Record the click
	now := time.Now()

	// If not opened yet, mark as opened via click
	if tracking.OpenedAt == nil {
		openedVia := "click"
		db.Model(&tracking).Updates(map[string]interface{}{
			"opened_at":  now,
			"opened_via": openedVia,
		})
	}

	// Handle special actions
	if action == "unsubscribe" {
		db.Model(&tracking).Update("unsubscribed_at", now)
	}

	// Update first click info
	if tracking.ClickedAt == nil {
		db.Model(&tracking).Updates(map[string]interface{}{
			"clicked_at":   now,
			"clicked_link": destinationURL,
		})
	}

	// Increment click count
	db.Model(&tracking).UpdateColumn("links_clicked", tracking.LinksClicked+1)

	// Create click record
	click := EmailTrackingClick{
		EmailTrackingID: tracking.ID,
		LinkURL:         destinationURL,
		LinkPosition:    &position,
		Action:          &action,
		IPAddress:       &ipAddress,
		UserAgent:       &userAgent,
		ClickedAt:       now,
	}
	db.Create(&click)

	return c.Redirect(destinationURL)
}

// Image tracks an image load for scroll depth estimation
// @Router /e/d/i/{id} [get]
// @Summary Delivery image
// @Description Redirects to the original image
// @Tags delivery
// @Param id path string true "ID"
// @Param url query string true "Base64 encoded original image URL"
// @Param p query string true "Position identifier"
// @Param s query integer false "Scroll percentage"
// @Success 302 {string} string "Redirect to original image"
func Image(c *fiber.Ctx) error {
	db := database.DBConn
	trackingID := c.Params("id")

	// Get tracking record
	var tracking EmailTracking
	result := db.Where("tracking_id = ?", trackingID).First(&tracking)

	if result.Error == nil {
		position := c.Query("p", "unknown")
		scrollPercent := c.QueryInt("s", -1)

		now := time.Now()

		// If not opened yet, mark as opened via image
		if tracking.OpenedAt == nil {
			openedVia := "image"
			db.Model(&tracking).Updates(map[string]interface{}{
				"opened_at":  now,
				"opened_via": openedVia,
			})
		}

		// Create image load record
		imageLoad := EmailTrackingImage{
			EmailTrackingID: tracking.ID,
			ImagePosition:   position,
			LoadedAt:        now,
		}

		if scrollPercent >= 0 && scrollPercent <= 100 {
			sp := uint8(scrollPercent)
			imageLoad.EstimatedScrollPercent = &sp

			// Update scroll depth if this is deeper
			if tracking.ScrollDepthPercent == nil || sp > *tracking.ScrollDepthPercent {
				db.Model(&tracking).Update("scroll_depth_percent", sp)
			}
		}

		db.Create(&imageLoad)

		// Increment image count
		db.Model(&tracking).UpdateColumn("images_loaded", tracking.ImagesLoaded+1)
	}

	// Redirect to original image
	urlEncoded := c.Query("url", "")
	urlBytes, err := base64.StdEncoding.DecodeString(urlEncoded)
	if err != nil || len(urlBytes) == 0 {
		// Return transparent GIF as fallback
		c.Set("Content-Type", "image/gif")
		return c.Send(transparentGIF)
	}

	return c.Redirect(string(urlBytes))
}

// Note: MDN read receipts are processed by PHP's incoming mail handler
// which updates the database directly. No HTTP endpoint needed here.

// Stats returns email statistics (requires authentication)
// @Router /email/stats [get]
// @Summary Get email statistics
// @Description Returns aggregate email statistics for authorized users
// @Tags emailtracking
// @Produce json
// @Security BearerAuth
// @Param type query string false "Email type filter"
// @Param start query string false "Start date (YYYY-MM-DD)"
// @Param end query string false "End date (YYYY-MM-DD)"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} fiber.Error "Unauthorized"
func Stats(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Check if user has support/admin role
	var userInfo struct {
		Systemrole string `json:"systemrole"`
	}
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&userInfo)
	if userInfo.Systemrole != "Support" && userInfo.Systemrole != "Admin" {
		return fiber.NewError(fiber.StatusForbidden, "Support or Admin role required")
	}

	emailType := c.Query("type", "")
	startDate := c.Query("start", "")
	endDate := c.Query("end", "")

	// Build query
	query := db.Model(&EmailTracking{})
	if emailType != "" {
		query = query.Where("email_type = ?", emailType)
	}
	if startDate != "" && endDate != "" {
		query = query.Where("sent_at BETWEEN ? AND ?", startDate, endDate+" 23:59:59")
	}

	// Get counts
	var totalSent, opened, clicked, bounced int64
	query.Count(&totalSent)
	query.Where("opened_at IS NOT NULL").Count(&opened)
	query.Where("clicked_at IS NOT NULL").Count(&clicked)
	query.Where("bounced_at IS NOT NULL").Count(&bounced)

	// Calculate rates
	var openRate, clickRate, clickToOpenRate, bounceRate float64
	if totalSent > 0 {
		openRate = float64(opened) / float64(totalSent) * 100
		clickRate = float64(clicked) / float64(totalSent) * 100
		bounceRate = float64(bounced) / float64(totalSent) * 100
	}
	if opened > 0 {
		clickToOpenRate = float64(clicked) / float64(opened) * 100
	}

	stats := EmailStats{
		TotalSent:       totalSent,
		Opened:          opened,
		Clicked:         clicked,
		Bounced:         bounced,
		OpenRate:        openRate,
		ClickRate:       clickRate,
		ClickToOpenRate: clickToOpenRate,
		BounceRate:      bounceRate,
	}

	return c.JSON(fiber.Map{
		"stats": stats,
		"period": fiber.Map{
			"start": startDate,
			"end":   endDate,
			"type":  emailType,
		},
	})
}

// UserEmailTrackingResponse represents email tracking data for a user
type UserEmailTrackingResponse struct {
	ID            uint64     `json:"id"`
	EmailType     string     `json:"email_type"`
	Subject       *string    `json:"subject"`
	SentAt        *time.Time `json:"sent_at"`
	OpenedAt      *time.Time `json:"opened_at"`
	OpenedVia     *string    `json:"opened_via"`
	ClickedAt     *time.Time `json:"clicked_at"`
	ClickedLink   *string    `json:"clicked_link"`
	LinksClicked  uint16     `json:"links_clicked"`
	BouncedAt     *time.Time `json:"bounced_at"`
	UnsubscribedAt *time.Time `json:"unsubscribed_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// UserEmails returns email tracking for a specific user (requires authentication)
// @Router /email/user/{id} [get]
// @Summary Get email tracking for a user
// @Description Returns email tracking records for a specific user (Support/Admin only). Can specify user by ID in path or email in query.
// @Tags emailtracking
// @Produce json
// @Security BearerAuth
// @Param id path int true "User ID (use 0 if searching by email)"
// @Param email query string false "User email address (alternative to ID)"
// @Param limit query int false "Number of records (default 50)"
// @Param offset query int false "Offset for pagination"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} fiber.Error "Unauthorized"
// @Failure 403 {object} fiber.Error "Forbidden"
func UserEmails(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Check if user has support/admin role
	var userInfo struct {
		Systemrole string `json:"systemrole"`
	}
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&userInfo)
	if userInfo.Systemrole != "Support" && userInfo.Systemrole != "Admin" {
		return fiber.NewError(fiber.StatusForbidden, "Support or Admin role required")
	}

	// Get target user ID from path or resolve from email
	targetUserID, _ := c.ParamsInt("id")
	email := c.Query("email", "")

	// If no valid ID but email provided, look up user by email
	if targetUserID <= 0 && email != "" {
		var userLookup struct {
			UserID uint64 `gorm:"column:userid"`
		}
		// First try users_emails table (for users with multiple emails)
		result := db.Raw("SELECT userid FROM users_emails WHERE email = ? AND backwards IS NULL LIMIT 1", email).Scan(&userLookup)
		if result.Error != nil || userLookup.UserID == 0 {
			// Fallback to users table (for new users whose email is only in users.email)
			var userFallback struct {
				ID uint64 `gorm:"column:id"`
			}
			result = db.Raw("SELECT id FROM users WHERE email = ? LIMIT 1", email).Scan(&userFallback)
			if result.Error != nil || userFallback.ID == 0 {
				return fiber.NewError(fiber.StatusNotFound, "No user found with that email address")
			}
			userLookup.UserID = userFallback.ID
		}
		targetUserID = int(userLookup.UserID)
	}

	if targetUserID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID or email")
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	// Cap limit at 100
	if limit > 100 {
		limit = 100
	}

	var emails []EmailTracking
	result := db.Where("userid = ?", targetUserID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&emails)

	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Database error")
	}

	// Get total count
	var total int64
	db.Model(&EmailTracking{}).Where("userid = ?", targetUserID).Count(&total)

	// Convert to response format
	response := make([]UserEmailTrackingResponse, len(emails))
	for i, e := range emails {
		response[i] = UserEmailTrackingResponse{
			ID:            e.ID,
			EmailType:     e.EmailType,
			Subject:       e.Subject,
			SentAt:        e.SentAt,
			OpenedAt:      e.OpenedAt,
			OpenedVia:     e.OpenedVia,
			ClickedAt:     e.ClickedAt,
			ClickedLink:   e.ClickedLink,
			LinksClicked:  e.LinksClicked,
			BouncedAt:     e.BouncedAt,
			UnsubscribedAt: e.UnsubscribedAt,
			CreatedAt:     e.CreatedAt,
		}
	}

	return c.JSON(fiber.Map{
		"emails": response,
		"userid": targetUserID,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// recordOpen records an email open event
func recordOpen(trackingID string, via string) {
	db := database.DBConn

	var tracking EmailTracking
	result := db.Where("tracking_id = ?", trackingID).First(&tracking)
	if result.Error != nil {
		return
	}

	// Only record first open
	if tracking.OpenedAt != nil {
		return
	}

	now := time.Now()
	db.Model(&tracking).Updates(map[string]interface{}{
		"opened_at":  now,
		"opened_via": via,
	})
}

// DailyStats represents statistics for a single day
type DailyStats struct {
	Date    string `json:"date"`
	Sent    int64  `json:"sent"`
	Opened  int64  `json:"opened"`
	Clicked int64  `json:"clicked"`
	Bounced int64  `json:"bounced"`
}

// EmailTypeStats represents statistics for a specific email type
type EmailTypeStats struct {
	EmailType       string  `json:"email_type"`
	TotalSent       int64   `json:"total_sent"`
	Opened          int64   `json:"opened"`
	Clicked         int64   `json:"clicked"`
	Bounced         int64   `json:"bounced"`
	OpenRate        float64 `json:"open_rate"`
	ClickRate       float64 `json:"click_rate"`
	ClickToOpenRate float64 `json:"click_to_open_rate"`
	BounceRate      float64 `json:"bounce_rate"`
}

// TimeSeries returns daily email statistics for charting (requires authentication)
// @Router /email/stats/timeseries [get]
// @Summary Get daily email statistics for charting
// @Description Returns daily sent/opened/clicked/bounced counts for date range
// @Tags emailtracking
// @Produce json
// @Security BearerAuth
// @Param type query string false "Email type filter"
// @Param start query string false "Start date (YYYY-MM-DD)"
// @Param end query string false "End date (YYYY-MM-DD)"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} fiber.Error "Unauthorized"
func TimeSeries(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Check if user has support/admin role
	var userInfo struct {
		Systemrole string `json:"systemrole"`
	}
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&userInfo)
	if userInfo.Systemrole != "Support" && userInfo.Systemrole != "Admin" {
		return fiber.NewError(fiber.StatusForbidden, "Support or Admin role required")
	}

	emailType := c.Query("type", "")
	startDate := c.Query("start", "")
	endDate := c.Query("end", "")

	// Default to last 30 days if no dates provided
	if startDate == "" || endDate == "" {
		now := time.Now()
		endDate = now.Format("2006-01-02")
		startDate = now.AddDate(0, 0, -30).Format("2006-01-02")
	}

	// Build query for daily stats
	query := `
		SELECT
			DATE(sent_at) as date,
			COUNT(*) as sent,
			SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END) as opened,
			SUM(CASE WHEN clicked_at IS NOT NULL THEN 1 ELSE 0 END) as clicked,
			SUM(CASE WHEN bounced_at IS NOT NULL THEN 1 ELSE 0 END) as bounced
		FROM email_tracking
		WHERE sent_at BETWEEN ? AND ?
	`

	args := []interface{}{startDate, endDate + " 23:59:59"}

	if emailType != "" {
		query += " AND email_type = ?"
		args = append(args, emailType)
	}

	query += " GROUP BY DATE(sent_at) ORDER BY date ASC"

	var dailyStats []DailyStats
	db.Raw(query, args...).Scan(&dailyStats)

	return c.JSON(fiber.Map{
		"data": dailyStats,
		"period": fiber.Map{
			"start": startDate,
			"end":   endDate,
			"type":  emailType,
		},
	})
}

// StatsByType returns email statistics broken down by email type (requires authentication)
// @Router /email/stats/bytype [get]
// @Summary Get email statistics by email type
// @Description Returns statistics for each email type for comparison charts
// @Tags emailtracking
// @Produce json
// @Security BearerAuth
// @Param start query string false "Start date (YYYY-MM-DD)"
// @Param end query string false "End date (YYYY-MM-DD)"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} fiber.Error "Unauthorized"
func StatsByType(c *fiber.Ctx) error {
	db := database.DBConn

	myid := user.WhoAmI(c)
	if myid == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "Not logged in")
	}

	// Check if user has support/admin role
	var userInfo struct {
		Systemrole string `json:"systemrole"`
	}
	db.Raw("SELECT systemrole FROM users WHERE id = ?", myid).Scan(&userInfo)
	if userInfo.Systemrole != "Support" && userInfo.Systemrole != "Admin" {
		return fiber.NewError(fiber.StatusForbidden, "Support or Admin role required")
	}

	startDate := c.Query("start", "")
	endDate := c.Query("end", "")

	// Build query for stats by type
	query := `
		SELECT
			email_type,
			COUNT(*) as total_sent,
			SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END) as opened,
			SUM(CASE WHEN clicked_at IS NOT NULL THEN 1 ELSE 0 END) as clicked,
			SUM(CASE WHEN bounced_at IS NOT NULL THEN 1 ELSE 0 END) as bounced
		FROM email_tracking
		WHERE 1=1
	`

	var args []interface{}

	if startDate != "" && endDate != "" {
		query += " AND sent_at BETWEEN ? AND ?"
		args = append(args, startDate, endDate+" 23:59:59")
	}

	query += " GROUP BY email_type ORDER BY total_sent DESC"

	var rawStats []struct {
		EmailType string `gorm:"column:email_type"`
		TotalSent int64  `gorm:"column:total_sent"`
		Opened    int64  `gorm:"column:opened"`
		Clicked   int64  `gorm:"column:clicked"`
		Bounced   int64  `gorm:"column:bounced"`
	}
	db.Raw(query, args...).Scan(&rawStats)

	// Calculate rates
	stats := make([]EmailTypeStats, len(rawStats))
	for i, r := range rawStats {
		stats[i] = EmailTypeStats{
			EmailType: r.EmailType,
			TotalSent: r.TotalSent,
			Opened:    r.Opened,
			Clicked:   r.Clicked,
			Bounced:   r.Bounced,
		}
		if r.TotalSent > 0 {
			stats[i].OpenRate = float64(r.Opened) / float64(r.TotalSent) * 100
			stats[i].ClickRate = float64(r.Clicked) / float64(r.TotalSent) * 100
			stats[i].BounceRate = float64(r.Bounced) / float64(r.TotalSent) * 100
		}
		if r.Opened > 0 {
			stats[i].ClickToOpenRate = float64(r.Clicked) / float64(r.Opened) * 100
		}
	}

	return c.JSON(fiber.Map{
		"data": stats,
		"period": fiber.Map{
			"start": startDate,
			"end":   endDate,
		},
	})
}

// isValidRedirectURL validates URL is safe for redirect
func isValidRedirectURL(url string) bool {
	if url == "" {
		return false
	}

	// Must start with http:// or https://
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}

	// Build allowed domains from environment variables
	var allowedDomains []string

	if userSite := os.Getenv("USER_SITE"); userSite != "" {
		allowedDomains = append(allowedDomains, userSite)
	}
	if imageDomain := os.Getenv("IMAGE_DOMAIN"); imageDomain != "" {
		allowedDomains = append(allowedDomains, imageDomain)
	}
	if archivedDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN"); archivedDomain != "" {
		allowedDomains = append(allowedDomains, archivedDomain)
	}
	if groupDomain := os.Getenv("GROUP_DOMAIN"); groupDomain != "" {
		allowedDomains = append(allowedDomains, groupDomain)
	}

	// Allow localhost for development
	allowedDomains = append(allowedDomains, "localhost")

	for _, domain := range allowedDomains {
		if strings.Contains(url, domain) {
			return true
		}
	}

	// Reject URLs not matching our domains
	return false
}
