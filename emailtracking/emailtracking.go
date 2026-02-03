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
	"gorm.io/gorm"
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
	HasAMP             bool       `json:"has_amp" gorm:"column:has_amp"`
	RepliedAt          *time.Time `json:"replied_at" gorm:"column:replied_at"`
	RepliedVia         *string    `json:"replied_via" gorm:"column:replied_via"`
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
	TotalSent       int64   `json:"total_sent"`
	Opened          int64   `json:"opened"`
	Clicked         int64   `json:"clicked"`
	Bounced         int64   `json:"bounced"`          // Bounces linked to email_tracking records
	OpenRate        float64 `json:"open_rate"`
	ClickRate       float64 `json:"click_rate"`
	ClickToOpenRate float64 `json:"click_to_open_rate"`
	BounceRate      float64 `json:"bounce_rate"`
	// Actual bounces from bounces_emails table (includes all bounces, not just tracked ones)
	TotalBounces     int64   `json:"total_bounces"`
	PermanentBounces int64   `json:"permanent_bounces"`
	TemporaryBounces int64   `json:"temporary_bounces"`
}

// AMPStats represents AMP-specific statistics
type AMPStats struct {
	TotalWithAMP    int64   `json:"total_with_amp"`
	TotalWithoutAMP int64   `json:"total_without_amp"`
	AMPPercentage   float64 `json:"amp_percentage"`
	// AMP rendering metrics - how many were actually rendered with AMP
	AMPRendered   int64   `json:"amp_rendered"`
	AMPRenderRate float64 `json:"amp_render_rate"`
	// AMP engagement metrics
	AMPOpened     int64   `json:"amp_opened"`
	AMPClicked    int64   `json:"amp_clicked"`
	AMPBounced    int64   `json:"amp_bounced"`
	AMPReplied    int64   `json:"amp_replied"`
	AMPOpenRate   float64 `json:"amp_open_rate"`
	AMPClickRate  float64 `json:"amp_click_rate"`
	AMPBounceRate float64 `json:"amp_bounce_rate"`
	AMPReplyRate  float64 `json:"amp_reply_rate"`
	// Reply breakdown by method for AMP-enabled emails
	AMPRepliedViaAMP   int64   `json:"amp_replied_via_amp"`   // Replies via AMP form
	AMPRepliedViaEmail int64   `json:"amp_replied_via_email"` // Replies via email
	AMPReplyViaAMPRate float64 `json:"amp_reply_via_amp_rate"`
	AMPReplyViaEmailRate float64 `json:"amp_reply_via_email_rate"`
	// Click breakdown: reply clicks (to message/chat pages) vs other clicks
	AMPReplyClicks    int64   `json:"amp_reply_clicks"`     // Clicks to reply on web
	AMPOtherClicks    int64   `json:"amp_other_clicks"`     // Other clicks (view item, etc.)
	AMPReplyClickRate float64 `json:"amp_reply_click_rate"` // Reply click rate
	AMPOtherClickRate float64 `json:"amp_other_click_rate"` // Other click rate
	// Response rate: all ways of responding (AMP reply + email reply + click to reply on web)
	AMPResponseRate float64 `json:"amp_response_rate"`
	// Legacy action rate (for backwards compatibility)
	AMPActionRate float64 `json:"amp_action_rate"`
	// Non-AMP engagement metrics (for comparison)
	NonAMPOpened      int64   `json:"non_amp_opened"`
	NonAMPClicked     int64   `json:"non_amp_clicked"`
	NonAMPBounced     int64   `json:"non_amp_bounced"`
	NonAMPReplied     int64   `json:"non_amp_replied"`
	NonAMPOpenRate    float64 `json:"non_amp_open_rate"`
	NonAMPClickRate   float64 `json:"non_amp_click_rate"`
	NonAMPBounceRate  float64 `json:"non_amp_bounce_rate"`
	NonAMPReplyRate   float64 `json:"non_amp_reply_rate"`
	// Click breakdown for non-AMP
	NonAMPReplyClicks    int64   `json:"non_amp_reply_clicks"`
	NonAMPOtherClicks    int64   `json:"non_amp_other_clicks"`
	NonAMPReplyClickRate float64 `json:"non_amp_reply_click_rate"`
	NonAMPOtherClickRate float64 `json:"non_amp_other_click_rate"`
	// Response rate: email reply + click to reply on web
	NonAMPResponseRate float64 `json:"non_amp_response_rate"`
	// Legacy action rate (for backwards compatibility)
	NonAMPActionRate float64 `json:"non_amp_action_rate"`
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

// TODO: Add endpoints/stats for scroll depth tracking data:
// - Average scroll depth per email type
// - Distribution of scroll depths (histogram)
// - Scroll depth by image position (which images are loaded)
// - Correlation between scroll depth and click-through rates
// The data is collected via email_tracking.scroll_depth_percent and
// email_tracking_images table, but not yet exposed in the stats API.

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
		// If endDate doesn't include time, add end of day
		endDateTime := endDate
		if !strings.Contains(endDate, " ") && !strings.Contains(endDate, "T") {
			endDateTime = endDate + " 23:59:59"
		}
		query = query.Where("sent_at BETWEEN ? AND ?", startDate, endDateTime)
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

	// Get actual bounce counts from bounces_emails table
	bounceStats := getBouncesEmailsStats(db, startDate, endDate)
	stats.TotalBounces = bounceStats.Total
	stats.PermanentBounces = bounceStats.Permanent
	stats.TemporaryBounces = bounceStats.Temporary

	// Get AMP statistics
	ampStats := getAMPStats(db, emailType, startDate, endDate)

	return c.JSON(fiber.Map{
		"stats":     stats,
		"amp_stats": ampStats,
		"period": fiber.Map{
			"start": startDate,
			"end":   endDate,
			"type":  emailType,
		},
	})
}

// BouncesEmailsStats represents bounce statistics from the bounces_emails table
type BouncesEmailsStats struct {
	Total     int64
	Permanent int64
	Temporary int64
}

// getBouncesEmailsStats queries the bounces_emails table for actual bounce counts
func getBouncesEmailsStats(db *gorm.DB, startDate, endDate string) BouncesEmailsStats {
	var stats BouncesEmailsStats

	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN permanent = 1 THEN 1 ELSE 0 END) as permanent,
			SUM(CASE WHEN permanent = 0 THEN 1 ELSE 0 END) as temporary
		FROM bounces_emails
		WHERE reset = 0
	`

	var args []interface{}

	if startDate != "" && endDate != "" {
		// If endDate doesn't include time, add end of day
		endDateTime := endDate
		if !strings.Contains(endDate, " ") && !strings.Contains(endDate, "T") {
			endDateTime = endDate + " 23:59:59"
		}
		query += " AND date BETWEEN ? AND ?"
		args = append(args, startDate, endDateTime)
	}

	db.Raw(query, args...).Scan(&stats)

	return stats
}

// getAMPStats calculates AMP-specific statistics for the given filters
func getAMPStats(db *gorm.DB, emailType, startDate, endDate string) AMPStats {
	var stats AMPStats

	// Build base query conditions
	conditions := "1=1"
	var args []interface{}

	if emailType != "" {
		conditions += " AND email_type = ?"
		args = append(args, emailType)
	}
	if startDate != "" && endDate != "" {
		// If endDate doesn't include time, add end of day
		endDateTime := endDate
		if !strings.Contains(endDate, " ") && !strings.Contains(endDate, "T") {
			endDateTime = endDate + " 23:59:59"
		}
		conditions += " AND sent_at BETWEEN ? AND ?"
		args = append(args, startDate, endDateTime)
	}

	// Query for AMP emails
	ampConditions := conditions + " AND has_amp = 1"
	var ampCounts struct {
		Total          int64
		Opened         int64
		Clicked        int64
		Bounced        int64
		Replied        int64
		RepliedViaAMP  int64
		RepliedViaEmail int64
		Rendered       int64
	}
	db.Raw(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END) as opened,
			SUM(CASE WHEN clicked_at IS NOT NULL THEN 1 ELSE 0 END) as clicked,
			SUM(CASE WHEN bounced_at IS NOT NULL THEN 1 ELSE 0 END) as bounced,
			SUM(CASE WHEN replied_at IS NOT NULL THEN 1 ELSE 0 END) as replied,
			SUM(CASE WHEN replied_via = 'amp' THEN 1 ELSE 0 END) as replied_via_amp,
			SUM(CASE WHEN replied_via = 'email' THEN 1 ELSE 0 END) as replied_via_email,
			SUM(CASE WHEN opened_via = 'amp' THEN 1 ELSE 0 END) as rendered
		FROM email_tracking
		WHERE `+ampConditions, args...).Scan(&ampCounts)

	// Query for non-AMP emails
	nonAMPConditions := conditions + " AND has_amp = 0"
	var nonAMPCounts struct {
		Total   int64
		Opened  int64
		Clicked int64
		Bounced int64
		Replied int64
	}
	db.Raw(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END) as opened,
			SUM(CASE WHEN clicked_at IS NOT NULL THEN 1 ELSE 0 END) as clicked,
			SUM(CASE WHEN bounced_at IS NOT NULL THEN 1 ELSE 0 END) as bounced,
			SUM(CASE WHEN replied_at IS NOT NULL THEN 1 ELSE 0 END) as replied
		FROM email_tracking
		WHERE `+nonAMPConditions, args...).Scan(&nonAMPCounts)

	// Populate stats
	stats.TotalWithAMP = ampCounts.Total
	stats.TotalWithoutAMP = nonAMPCounts.Total
	stats.AMPRendered = ampCounts.Rendered
	stats.AMPOpened = ampCounts.Opened
	stats.AMPClicked = ampCounts.Clicked
	stats.AMPBounced = ampCounts.Bounced
	stats.AMPReplied = ampCounts.Replied
	stats.AMPRepliedViaAMP = ampCounts.RepliedViaAMP
	stats.AMPRepliedViaEmail = ampCounts.RepliedViaEmail
	stats.NonAMPOpened = nonAMPCounts.Opened
	stats.NonAMPClicked = nonAMPCounts.Clicked
	stats.NonAMPBounced = nonAMPCounts.Bounced
	stats.NonAMPReplied = nonAMPCounts.Replied

	// Query for click breakdown (reply clicks vs other clicks)
	// Reply clicks are clicks to message/chat pages where users can reply
	var ampClickBreakdown struct {
		ReplyClicks int64
		OtherClicks int64
	}
	db.Raw(`
		SELECT
			COUNT(DISTINCT CASE WHEN c.link_url LIKE '%/message/%' OR c.link_url LIKE '%/chat/%' OR c.link_url LIKE '%/chats/%' THEN c.email_tracking_id END) as reply_clicks,
			COUNT(DISTINCT CASE WHEN c.link_url NOT LIKE '%/message/%' AND c.link_url NOT LIKE '%/chat/%' AND c.link_url NOT LIKE '%/chats/%' AND c.link_url NOT LIKE 'amp://%' THEN c.email_tracking_id END) as other_clicks
		FROM email_tracking_clicks c
		JOIN email_tracking e ON c.email_tracking_id = e.id
		WHERE e.has_amp = 1 AND `+strings.Replace(conditions, "sent_at", "e.sent_at", -1), args...).Scan(&ampClickBreakdown)

	var nonAMPClickBreakdown struct {
		ReplyClicks int64
		OtherClicks int64
	}
	db.Raw(`
		SELECT
			COUNT(DISTINCT CASE WHEN c.link_url LIKE '%/message/%' OR c.link_url LIKE '%/chat/%' OR c.link_url LIKE '%/chats/%' THEN c.email_tracking_id END) as reply_clicks,
			COUNT(DISTINCT CASE WHEN c.link_url NOT LIKE '%/message/%' AND c.link_url NOT LIKE '%/chat/%' AND c.link_url NOT LIKE '%/chats/%' THEN c.email_tracking_id END) as other_clicks
		FROM email_tracking_clicks c
		JOIN email_tracking e ON c.email_tracking_id = e.id
		WHERE e.has_amp = 0 AND `+strings.Replace(conditions, "sent_at", "e.sent_at", -1), args...).Scan(&nonAMPClickBreakdown)

	stats.AMPReplyClicks = ampClickBreakdown.ReplyClicks
	stats.AMPOtherClicks = ampClickBreakdown.OtherClicks
	stats.NonAMPReplyClicks = nonAMPClickBreakdown.ReplyClicks
	stats.NonAMPOtherClicks = nonAMPClickBreakdown.OtherClicks

	// Calculate percentages
	totalEmails := stats.TotalWithAMP + stats.TotalWithoutAMP
	if totalEmails > 0 {
		stats.AMPPercentage = float64(stats.TotalWithAMP) / float64(totalEmails) * 100
	}

	// AMP render rate: of AMP emails sent, how many were actually rendered with AMP
	if stats.TotalWithAMP > 0 {
		stats.AMPRenderRate = float64(stats.AMPRendered) / float64(stats.TotalWithAMP) * 100
	}

	// AMP rates
	if stats.TotalWithAMP > 0 {
		stats.AMPOpenRate = float64(stats.AMPOpened) / float64(stats.TotalWithAMP) * 100
		stats.AMPClickRate = float64(stats.AMPClicked) / float64(stats.TotalWithAMP) * 100
		stats.AMPBounceRate = float64(stats.AMPBounced) / float64(stats.TotalWithAMP) * 100
		stats.AMPReplyRate = float64(stats.AMPReplied) / float64(stats.TotalWithAMP) * 100
		// Reply breakdown by method
		stats.AMPReplyViaAMPRate = float64(stats.AMPRepliedViaAMP) / float64(stats.TotalWithAMP) * 100
		stats.AMPReplyViaEmailRate = float64(stats.AMPRepliedViaEmail) / float64(stats.TotalWithAMP) * 100
		// Click breakdown
		stats.AMPReplyClickRate = float64(stats.AMPReplyClicks) / float64(stats.TotalWithAMP) * 100
		stats.AMPOtherClickRate = float64(stats.AMPOtherClicks) / float64(stats.TotalWithAMP) * 100
		// Response rate: all ways of responding (AMP replies + email replies + clicks to reply on web)
		stats.AMPResponseRate = float64(stats.AMPReplied+stats.AMPReplyClicks) / float64(stats.TotalWithAMP) * 100
		// Legacy action rate (for backwards compatibility)
		stats.AMPActionRate = float64(stats.AMPClicked+stats.AMPReplied) / float64(stats.TotalWithAMP) * 100
	}

	// Non-AMP rates
	if stats.TotalWithoutAMP > 0 {
		stats.NonAMPOpenRate = float64(stats.NonAMPOpened) / float64(stats.TotalWithoutAMP) * 100
		stats.NonAMPClickRate = float64(stats.NonAMPClicked) / float64(stats.TotalWithoutAMP) * 100
		stats.NonAMPBounceRate = float64(stats.NonAMPBounced) / float64(stats.TotalWithoutAMP) * 100
		stats.NonAMPReplyRate = float64(stats.NonAMPReplied) / float64(stats.TotalWithoutAMP) * 100
		// Click breakdown
		stats.NonAMPReplyClickRate = float64(stats.NonAMPReplyClicks) / float64(stats.TotalWithoutAMP) * 100
		stats.NonAMPOtherClickRate = float64(stats.NonAMPOtherClicks) / float64(stats.TotalWithoutAMP) * 100
		// Response rate: email replies + clicks to reply on web
		stats.NonAMPResponseRate = float64(stats.NonAMPReplied+stats.NonAMPReplyClicks) / float64(stats.TotalWithoutAMP) * 100
		// Legacy action rate (for backwards compatibility)
		stats.NonAMPActionRate = float64(stats.NonAMPClicked+stats.NonAMPReplied) / float64(stats.TotalWithoutAMP) * 100
	}

	return stats
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
// @Description Returns email tracking records for a specific user (Support/Admin only). Can specify user by ID in path or email in query. When searching by email, also searches recipient_email in tracking records.
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
	searchByRecipientEmail := false

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
				// No user found - search by recipient_email in email_tracking table directly
				searchByRecipientEmail = true
			} else {
				userLookup.UserID = userFallback.ID
			}
		}
		if !searchByRecipientEmail {
			targetUserID = int(userLookup.UserID)
		}
	}

	if targetUserID <= 0 && !searchByRecipientEmail {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid user ID or email")
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	// Cap limit at 100
	if limit > 100 {
		limit = 100
	}

	var emails []EmailTracking
	var total int64

	if searchByRecipientEmail {
		// Search by recipient_email directly in email_tracking table
		result := db.Where("recipient_email = ?", email).
			Order("created_at DESC").
			Limit(limit).
			Offset(offset).
			Find(&emails)

		if result.Error != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Database error")
		}

		// Get total count
		db.Model(&EmailTracking{}).Where("recipient_email = ?", email).Count(&total)
	} else {
		// Search by user ID
		result := db.Where("userid = ?", targetUserID).
			Order("created_at DESC").
			Limit(limit).
			Offset(offset).
			Find(&emails)

		if result.Error != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "Database error")
		}

		// Get total count
		db.Model(&EmailTracking{}).Where("userid = ?", targetUserID).Count(&total)
	}

	// Convert to response format
	response := make([]UserEmailTrackingResponse, len(emails))
	for i, e := range emails {
		response[i] = UserEmailTrackingResponse{
			ID:             e.ID,
			EmailType:      e.EmailType,
			Subject:        e.Subject,
			SentAt:         e.SentAt,
			OpenedAt:       e.OpenedAt,
			OpenedVia:      e.OpenedVia,
			ClickedAt:      e.ClickedAt,
			ClickedLink:    e.ClickedLink,
			LinksClicked:   e.LinksClicked,
			BouncedAt:      e.BouncedAt,
			UnsubscribedAt: e.UnsubscribedAt,
			CreatedAt:      e.CreatedAt,
		}
	}

	// Build response - include email when searching by recipient_email
	responseMap := fiber.Map{
		"emails": response,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	if searchByRecipientEmail {
		responseMap["email"] = email
	} else {
		responseMap["userid"] = targetUserID
	}

	return c.JSON(responseMap)
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
	Bounced int64  `json:"bounced"` // Bounces linked to email_tracking
	// Actual bounces from bounces_emails table
	TotalBounces     int64 `json:"total_bounces"`
	PermanentBounces int64 `json:"permanent_bounces"`
	TemporaryBounces int64 `json:"temporary_bounces"`
	// AMP-specific metrics
	AMPSent        int64 `json:"amp_sent"`
	AMPOpened      int64 `json:"amp_opened"`
	AMPClicked     int64 `json:"amp_clicked"`
	AMPBounced     int64 `json:"amp_bounced"`
	AMPReplied     int64 `json:"amp_replied"`
	NonAMPSent     int64 `json:"non_amp_sent"`
	NonAMPOpened   int64 `json:"non_amp_opened"`
	NonAMPClicked  int64 `json:"non_amp_clicked"`
	NonAMPBounced  int64 `json:"non_amp_bounced"`
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

	// Build query for daily stats including AMP breakdown
	query := `
		SELECT
			DATE(sent_at) as date,
			COUNT(*) as sent,
			SUM(CASE WHEN opened_at IS NOT NULL THEN 1 ELSE 0 END) as opened,
			SUM(CASE WHEN clicked_at IS NOT NULL THEN 1 ELSE 0 END) as clicked,
			SUM(CASE WHEN bounced_at IS NOT NULL THEN 1 ELSE 0 END) as bounced,
			SUM(CASE WHEN has_amp = 1 THEN 1 ELSE 0 END) as amp_sent,
			SUM(CASE WHEN has_amp = 1 AND opened_at IS NOT NULL THEN 1 ELSE 0 END) as amp_opened,
			SUM(CASE WHEN has_amp = 1 AND clicked_at IS NOT NULL THEN 1 ELSE 0 END) as amp_clicked,
			SUM(CASE WHEN has_amp = 1 AND bounced_at IS NOT NULL THEN 1 ELSE 0 END) as amp_bounced,
			SUM(CASE WHEN has_amp = 1 AND replied_at IS NOT NULL THEN 1 ELSE 0 END) as amp_replied,
			SUM(CASE WHEN has_amp = 0 THEN 1 ELSE 0 END) as non_amp_sent,
			SUM(CASE WHEN has_amp = 0 AND opened_at IS NOT NULL THEN 1 ELSE 0 END) as non_amp_opened,
			SUM(CASE WHEN has_amp = 0 AND clicked_at IS NOT NULL THEN 1 ELSE 0 END) as non_amp_clicked,
			SUM(CASE WHEN has_amp = 0 AND bounced_at IS NOT NULL THEN 1 ELSE 0 END) as non_amp_bounced
		FROM email_tracking
		WHERE sent_at BETWEEN ? AND ?
	`

	// If endDate doesn't include time, add end of day
	endDateTime := endDate
	if !strings.Contains(endDate, " ") && !strings.Contains(endDate, "T") {
		endDateTime = endDate + " 23:59:59"
	}
	args := []interface{}{startDate, endDateTime}

	if emailType != "" {
		query += " AND email_type = ?"
		args = append(args, emailType)
	}

	query += " GROUP BY DATE(sent_at) ORDER BY date ASC"

	var dailyStats []DailyStats
	db.Raw(query, args...).Scan(&dailyStats)

	// Get daily bounce counts from bounces_emails table
	bounceQuery := `
		SELECT
			DATE(date) as date,
			COUNT(*) as total_bounces,
			SUM(CASE WHEN permanent = 1 THEN 1 ELSE 0 END) as permanent_bounces,
			SUM(CASE WHEN permanent = 0 THEN 1 ELSE 0 END) as temporary_bounces
		FROM bounces_emails
		WHERE reset = 0 AND date BETWEEN ? AND ?
		GROUP BY DATE(date)
	`
	var dailyBounces []struct {
		Date             string `gorm:"column:date"`
		TotalBounces     int64  `gorm:"column:total_bounces"`
		PermanentBounces int64  `gorm:"column:permanent_bounces"`
		TemporaryBounces int64  `gorm:"column:temporary_bounces"`
	}
	db.Raw(bounceQuery, startDate, endDateTime).Scan(&dailyBounces)

	// Create a map for quick lookup
	bounceMap := make(map[string]struct {
		Total     int64
		Permanent int64
		Temporary int64
	})
	for _, b := range dailyBounces {
		bounceMap[b.Date] = struct {
			Total     int64
			Permanent int64
			Temporary int64
		}{b.TotalBounces, b.PermanentBounces, b.TemporaryBounces}
	}

	// Merge bounce data into daily stats
	for i := range dailyStats {
		if bounces, ok := bounceMap[dailyStats[i].Date]; ok {
			dailyStats[i].TotalBounces = bounces.Total
			dailyStats[i].PermanentBounces = bounces.Permanent
			dailyStats[i].TemporaryBounces = bounces.Temporary
		}
	}

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
		// If endDate doesn't include time, add end of day
		endDateTime := endDate
		if !strings.Contains(endDate, " ") && !strings.Contains(endDate, "T") {
			endDateTime = endDate + " 23:59:59"
		}
		query += " AND sent_at BETWEEN ? AND ?"
		args = append(args, startDate, endDateTime)
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

// ClickedLinkStats represents a clicked link with count
type ClickedLinkStats struct {
	NormalizedURL string   `json:"normalized_url,omitempty"`
	URL           string   `json:"url,omitempty"`
	ClickCount    int64    `json:"click_count"`
	ExampleURLs   []string `json:"example_urls,omitempty"`
}

// normalizeURL removes user-specific data from URLs for aggregation
func normalizeURL(url string) string {
	// Parse the URL
	if url == "" {
		return ""
	}

	// Remove common tracking/user-specific query parameters
	// Keep the path but normalize numeric IDs
	result := url

	// Find query string start
	queryIdx := strings.Index(result, "?")
	path := result
	if queryIdx != -1 {
		path = result[:queryIdx]
	}

	// Normalize numeric IDs in the path (e.g., /message/12345 -> /message/{id})
	// Common patterns: /message/123, /user/123, /chat/123, /group/123
	pathParts := strings.Split(path, "/")
	for i, part := range pathParts {
		// Check if this part is purely numeric
		if len(part) > 0 && isNumeric(part) {
			pathParts[i] = "{id}"
		}
	}

	return strings.Join(pathParts, "/")
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// TopClickedLinks returns the most clicked links (requires authentication)
// @Router /email/stats/clicks [get]
// @Summary Get top clicked links
// @Description Returns the most clicked links from emails, optionally normalized to remove user-specific data
// @Tags emailtracking
// @Produce json
// @Security BearerAuth
// @Param start query string false "Start date (YYYY-MM-DD)"
// @Param end query string false "End date (YYYY-MM-DD)"
// @Param limit query int false "Number of links to return (default 5, use 0 for all)"
// @Param aggregate query bool false "Whether to aggregate similar URLs by normalizing IDs (default true)"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} fiber.Error "Unauthorized"
func TopClickedLinks(c *fiber.Ctx) error {
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
	limit := c.QueryInt("limit", 5)
	// Default to aggregated (true) unless explicitly set to false
	aggregate := c.Query("aggregate", "true") != "false"

	// Get all clicked links within the date range
	query := `
		SELECT c.link_url, COUNT(*) as click_count
		FROM email_tracking_clicks c
		JOIN email_tracking e ON c.email_tracking_id = e.id
		WHERE 1=1
	`

	var args []interface{}

	if startDate != "" && endDate != "" {
		// If endDate doesn't include time, add end of day
		endDateTime := endDate
		if !strings.Contains(endDate, " ") && !strings.Contains(endDate, "T") {
			endDateTime = endDate + " 23:59:59"
		}
		query += " AND c.clicked_at BETWEEN ? AND ?"
		args = append(args, startDate, endDateTime)
	}

	query += " GROUP BY c.link_url ORDER BY click_count DESC"

	var rawClicks []struct {
		LinkURL    string `gorm:"column:link_url"`
		ClickCount int64  `gorm:"column:click_count"`
	}
	db.Raw(query, args...).Scan(&rawClicks)

	var results []ClickedLinkStats

	if aggregate {
		// Aggregate by normalized URL
		normalizedMap := make(map[string]*ClickedLinkStats)
		for _, click := range rawClicks {
			normalized := normalizeURL(click.LinkURL)
			if normalized == "" {
				continue
			}

			if existing, ok := normalizedMap[normalized]; ok {
				existing.ClickCount += click.ClickCount
				// Keep up to 3 example URLs
				if len(existing.ExampleURLs) < 3 && !containsString(existing.ExampleURLs, click.LinkURL) {
					existing.ExampleURLs = append(existing.ExampleURLs, click.LinkURL)
				}
			} else {
				normalizedMap[normalized] = &ClickedLinkStats{
					NormalizedURL: normalized,
					ClickCount:    click.ClickCount,
					ExampleURLs:   []string{click.LinkURL},
				}
			}
		}

		// Convert map to slice
		results = make([]ClickedLinkStats, 0, len(normalizedMap))
		for _, stats := range normalizedMap {
			results = append(results, *stats)
		}

		// Sort by click count descending
		for i := 0; i < len(results); i++ {
			for j := i + 1; j < len(results); j++ {
				if results[j].ClickCount > results[i].ClickCount {
					results[i], results[j] = results[j], results[i]
				}
			}
		}
	} else {
		// Return raw URLs without aggregation
		results = make([]ClickedLinkStats, 0, len(rawClicks))
		for _, click := range rawClicks {
			if click.LinkURL == "" {
				continue
			}
			results = append(results, ClickedLinkStats{
				URL:        click.LinkURL,
				ClickCount: click.ClickCount,
			})
		}
	}

	// Apply limit (0 means all)
	totalCount := len(results)
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return c.JSON(fiber.Map{
		"data":      results,
		"total":     totalCount,
		"aggregate": aggregate,
		"period": fiber.Map{
			"start": startDate,
			"end":   endDate,
		},
	})
}

// containsString checks if a slice contains a string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
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
	if modSite := os.Getenv("MOD_SITE"); modSite != "" {
		allowedDomains = append(allowedDomains, modSite)
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

	// Allow Google Maps for address sharing in emails
	allowedDomains = append(allowedDomains, "maps.google.com")

	// Allow delivery service for image optimization (tracked images redirect here)
	allowedDomains = append(allowedDomains, "delivery.ilovefreegle.org")

	// Allow modtools.org for moderator chat links
	allowedDomains = append(allowedDomains, "modtools.org")

	for _, domain := range allowedDomains {
		if strings.Contains(url, domain) {
			return true
		}
	}

	// Reject URLs not matching our domains
	return false
}
