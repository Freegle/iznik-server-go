package visualise

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"

	"github.com/freegle/iznik-server-go/database"
	"github.com/freegle/iznik-server-go/misc"
	"github.com/freegle/iznik-server-go/utils"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type VisualiseRow struct {
	ID        uint64  `json:"id"`
	Msgid     uint64  `json:"msgid"`
	Attid     uint64  `json:"attid"`
	Fromuser  uint64  `json:"fromuser"`
	Touser    uint64  `json:"touser"`
	Fromlat   float64 `json:"fromlat"`
	Fromlng   float64 `json:"fromlng"`
	Tolat     float64 `json:"tolat"`
	Tolng     float64 `json:"tolng"`
	Distance  int     `json:"distance"`
	Timestamp string  `json:"timestamp"`
}

type AttachmentInfo struct {
	ID           uint64          `json:"id"`
	Archived     int             `json:"-"`
	Externaluid  string          `json:"-"`
	Externalmods json.RawMessage `json:"-"`
}

type UserIcon struct {
	ID   uint64 `json:"id"`
	Icon string `json:"icon"`
}

type OtherUser struct {
	ID   uint64  `json:"id"`
	Icon string  `json:"icon"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type VisualiseResult struct {
	ID         uint64                 `json:"id"`
	Msgid      uint64                 `json:"msgid"`
	Fromuser   uint64                 `json:"fromuser"`
	Touser     uint64                 `json:"touser"`
	Fromlat    float64                `json:"fromlat"`
	Fromlng    float64                `json:"fromlng"`
	Tolat      float64                `json:"tolat"`
	Tolng      float64                `json:"tolng"`
	Distance   int                    `json:"distance"`
	Timestamp  string                 `json:"timestamp"`
	Attachment map[string]interface{} `json:"attachment"`
	From       UserIcon               `json:"from"`
	To         UserIcon               `json:"to"`
	Others     []OtherUser            `json:"others"`
}

// GetVisualise returns visualisation data for the map on the homepage.
//
// @Summary Get visualisation data
// @Description Returns items that have been given/taken, with locations and user icons
// @Tags visualise
// @Produce json
// @Param swlat query number true "Southwest latitude"
// @Param swlng query number true "Southwest longitude"
// @Param nelat query number true "Northeast latitude"
// @Param nelng query number true "Northeast longitude"
// @Param limit query integer false "Max results (default 5)"
// @Param context query integer false "Pagination cursor (last ID seen)"
// @Success 200 {object} map[string]interface{}
// @Router /api/visualise [get]
func GetVisualise(c *fiber.Ctx) error {
	swlat, _ := strconv.ParseFloat(c.Query("swlat", "0"), 64)
	swlng, _ := strconv.ParseFloat(c.Query("swlng", "0"), 64)
	nelat, _ := strconv.ParseFloat(c.Query("nelat", "0"), 64)
	nelng, _ := strconv.ParseFloat(c.Query("nelng", "0"), 64)
	limit, _ := strconv.Atoi(c.Query("limit", "5"))
	ctx, _ := strconv.ParseUint(c.Query("context", "0"), 10, 64)

	if (swlat == 0 && swlng == 0) || (nelat == 0 && nelng == 0) {
		return c.JSON(fiber.Map{
			"ret":    0,
			"status": "Success",
			"list":   []interface{}{},
		})
	}

	db := database.DBConn

	// Query visualise table with optional cursor.
	var rows []VisualiseRow
	if ctx > 0 {
		db.Raw("SELECT id, msgid, attid, fromuser, touser, fromlat, fromlng, tolat, tolng, distance, timestamp "+
			"FROM visualise WHERE id < ? AND fromlat BETWEEN ? AND ? AND fromlng BETWEEN ? AND ? "+
			"ORDER BY id DESC LIMIT ?", ctx, swlat, nelat, swlng, nelng, limit).Scan(&rows)
	} else {
		db.Raw("SELECT id, msgid, attid, fromuser, touser, fromlat, fromlng, tolat, tolng, distance, timestamp "+
			"FROM visualise WHERE fromlat BETWEEN ? AND ? AND fromlng BETWEEN ? AND ? "+
			"ORDER BY id DESC LIMIT ?", swlat, nelat, swlng, nelng, limit).Scan(&rows)
	}

	imageDomain := os.Getenv("IMAGE_DOMAIN")
	if imageDomain == "" {
		imageDomain = "images.ilovefreegle.org"
	}
	archivedDomain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
	if archivedDomain == "" {
		archivedDomain = "images.ilovefreegle.org"
	}

	results := make([]VisualiseResult, len(rows))
	var lastCtx uint64

	for i, row := range rows {
		lastCtx = row.ID

		// Blur locations.
		blurredFromLat, blurredFromLng := utils.Blur(row.Fromlat, row.Fromlng, utils.BLUR_USER)
		blurredToLat, blurredToLng := utils.Blur(row.Tolat, row.Tolng, utils.BLUR_USER)

		results[i] = VisualiseResult{
			ID:        row.ID,
			Msgid:     row.Msgid,
			Fromuser:  row.Fromuser,
			Touser:    row.Touser,
			Fromlat:   blurredFromLat,
			Fromlng:   blurredFromLng,
			Tolat:     blurredToLat,
			Tolng:     blurredToLng,
			Distance:  row.Distance,
			Timestamp: row.Timestamp,
			Others:    []OtherUser{},
		}

		// Fetch attachment, user icons, and others concurrently.
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Attachment info.
		wg.Add(1)
		go func(idx int, attid uint64) {
			defer wg.Done()
			var att AttachmentInfo
			db.Raw("SELECT id, archived, externaluid, externalmods FROM messages_attachments WHERE id = ?", attid).Scan(&att)

			attPath, attThumb := getAttachmentPaths(att, imageDomain, archivedDomain)
			mu.Lock()
			results[idx].Attachment = map[string]interface{}{
				"id":    attid,
				"path":  attPath,
				"thumb": attThumb,
			}
			mu.Unlock()
		}(i, row.Attid)

		// From user icon.
		wg.Add(1)
		go func(idx int, userid uint64) {
			defer wg.Done()
			icon := getUserIcon(db, userid, imageDomain, archivedDomain)
			mu.Lock()
			results[idx].From = UserIcon{ID: userid, Icon: icon}
			mu.Unlock()
		}(i, row.Fromuser)

		// To user icon.
		wg.Add(1)
		go func(idx int, userid uint64) {
			defer wg.Done()
			icon := getUserIcon(db, userid, imageDomain, archivedDomain)
			mu.Lock()
			results[idx].To = UserIcon{ID: userid, Icon: icon}
			mu.Unlock()
		}(i, row.Touser)

		// Others who replied.
		wg.Add(1)
		go func(idx int, msgid, touser, fromuser uint64) {
			defer wg.Done()

			var otherIDs []struct{ Userid uint64 }
			db.Raw("SELECT DISTINCT userid FROM chat_messages WHERE refmsgid = ? AND userid != ? AND userid != ?",
				msgid, touser, fromuser).Scan(&otherIDs)

			for _, o := range otherIDs {
				icon := getUserIcon(db, o.Userid, imageDomain, archivedDomain)

				// Get user location from settings JSON.
				var lat, lng float64
				db.Raw("SELECT CASE WHEN settings IS NOT NULL AND JSON_VALID(settings) "+
					"THEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(settings, '$.mylocation.lat')), 0) ELSE 0 END AS lat, "+
					"CASE WHEN settings IS NOT NULL AND JSON_VALID(settings) "+
					"THEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(settings, '$.mylocation.lng')), 0) ELSE 0 END AS lng "+
					"FROM users WHERE id = ?", o.Userid).Row().Scan(&lat, &lng)

				if lat != 0 || lng != 0 {
					bLat, bLng := utils.Blur(lat, lng, utils.BLUR_USER)
					mu.Lock()
					results[idx].Others = append(results[idx].Others, OtherUser{
						ID:   o.Userid,
						Icon: icon,
						Lat:  bLat,
						Lng:  bLng,
					})
					mu.Unlock()
				}
			}
		}(i, row.Msgid, row.Touser, row.Fromuser)

		wg.Wait()
	}

	return c.JSON(fiber.Map{
		"ret":     0,
		"status":  "Success",
		"list":    results,
		"context": lastCtx,
	})
}

func getAttachmentPaths(att AttachmentInfo, imageDomain, archivedDomain string) (string, string) {
	if att.Externaluid != "" {
		url := misc.GetImageDeliveryUrl(att.Externaluid, string(att.Externalmods))
		return url, url
	}
	if att.Archived > 0 {
		return "https://" + archivedDomain + "/img_" + strconv.FormatUint(att.ID, 10) + ".jpg",
			"https://" + archivedDomain + "/timg_" + strconv.FormatUint(att.ID, 10) + ".jpg"
	}
	return "https://" + imageDomain + "/img_" + strconv.FormatUint(att.ID, 10) + ".jpg",
		"https://" + imageDomain + "/timg_" + strconv.FormatUint(att.ID, 10) + ".jpg"
}

func getUserIcon(db *gorm.DB, userid uint64, imageDomain, archivedDomain string) string {
	type profileRow struct {
		Profileid    uint64
		Url          string
		Externaluid  string
		Externalmods json.RawMessage
		Archived     int
	}

	var p profileRow
	db.Raw("SELECT ui.id AS profileid, ui.url, ui.externaluid, ui.externalmods, ui.archived "+
		"FROM users_images ui INNER JOIN users u ON u.profile = ui.id WHERE u.id = ?", userid).Scan(&p)

	if p.Profileid == 0 {
		return "https://" + imageDomain + "/defaultprofile.png"
	}

	if p.Url != "" {
		return p.Url
	}
	if p.Externaluid != "" {
		return misc.GetImageDeliveryUrl(p.Externaluid, string(p.Externalmods))
	}
	if p.Archived > 0 {
		return "https://" + archivedDomain + "/tuimg_" + strconv.FormatUint(p.Profileid, 10) + ".jpg"
	}
	return "https://" + imageDomain + "/tuimg_" + strconv.FormatUint(p.Profileid, 10) + ".jpg"
}
