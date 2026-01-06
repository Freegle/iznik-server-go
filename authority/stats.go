package authority

import (
	"fmt"
	"time"

	"github.com/freegle/iznik-server-go/database"
)

// Constants matching PHP Stats class.
const (
	TypeOffer    = "Offer"
	TypeWanted   = "Wanted"
	StatSearches = "Searches"
	StatWeight   = "Weight"
	StatOutcomes = "Outcomes"
	StatReplies  = "Replies"
)

// PostcodeStats represents stats for a partial postcode.
type PostcodeStats struct {
	Offer    int     `json:"Offer"`
	Wanted   int     `json:"Wanted"`
	Searches int     `json:"Searches"`
	Weight   float64 `json:"Weight"`
	Replies  int     `json:"Replies"`
	Outcomes int     `json:"Outcomes"`
}

// GetStatsByAuthority retrieves statistics for an authority area.
// Returns a map of partial postcodes to their stats.
func GetStatsByAuthority(authorityID uint64, start, end string) (map[string]PostcodeStats, error) {
	db := database.DBConn

	// Parse dates, defaulting to last 365 days.
	startTime, err := parseRelativeDate(start)
	if err != nil {
		startTime = time.Now().AddDate(-1, 0, 0)
	}
	endTime, err := parseRelativeDate(end)
	if err != nil {
		endTime = time.Now()
	}

	startStr := startTime.Format("2006-01-02")
	endStr := endTime.Format("2006-01-02 23:59:59")

	// Create temporary table of locationids for postcodes within the authority.
	// This mirrors the PHP implementation.
	err = db.Exec(`DROP TEMPORARY TABLE IF EXISTS pc`).Error
	if err != nil {
		return nil, err
	}

	err = db.Exec(`CREATE TEMPORARY TABLE pc AS (
		SELECT locationid
		FROM authorities
		INNER JOIN locations_spatial ON authorities.id = ?
			AND ST_Contains(authorities.polygon, locations_spatial.geometry)
	)`, authorityID).Error
	if err != nil {
		return nil, err
	}

	ret := make(map[string]PostcodeStats)

	// Query offers and wanteds.
	for _, msgType := range []string{TypeOffer, TypeWanted} {
		var stats []struct {
			PartialPostcode string `gorm:"column:PartialPostcode"`
			Count           int    `gorm:"column:count"`
		}

		db.Raw(`
			SELECT SUBSTRING(locations.name, 1, LENGTH(locations.name) - 2) AS PartialPostcode,
				   COUNT(*) as count
			FROM pc
			INNER JOIN messages ON messages.locationid = pc.locationid
			INNER JOIN locations ON messages.locationid = locations.id
			WHERE locations.type = 'Postcode'
				AND LOCATE(' ', locations.name) > 0
				AND messages.type = ?
				AND messages.arrival BETWEEN ? AND ?
			GROUP BY PartialPostcode
			ORDER BY locations.name`, msgType, startStr, endStr).Scan(&stats)

		for _, stat := range stats {
			ps := ret[stat.PartialPostcode]
			if msgType == TypeOffer {
				ps.Offer += stat.Count
			} else {
				ps.Wanted += stat.Count
			}
			ret[stat.PartialPostcode] = ps
		}
	}

	// Query replies (Interested chat messages).
	for _, msgType := range []string{TypeOffer, TypeWanted} {
		var stats []struct {
			PartialPostcode string `gorm:"column:PartialPostcode"`
			Count           int    `gorm:"column:count"`
		}

		db.Raw(`
			SELECT SUBSTRING(locations.name, 1, LENGTH(locations.name) - 2) AS PartialPostcode,
				   COUNT(*) as count
			FROM pc
			INNER JOIN messages ON messages.locationid = pc.locationid
			INNER JOIN locations ON messages.locationid = locations.id
			INNER JOIN chat_messages cm ON messages.id = cm.refmsgid AND cm.type = 'Interested'
			WHERE locations.type = 'Postcode'
				AND LOCATE(' ', locations.name) > 0
				AND messages.type = ?
				AND messages.arrival BETWEEN ? AND ?
			GROUP BY PartialPostcode
			ORDER BY locations.name`, msgType, startStr, endStr).Scan(&stats)

		for _, stat := range stats {
			ps := ret[stat.PartialPostcode]
			ps.Replies += stat.Count
			ret[stat.PartialPostcode] = ps
		}
	}

	// Query outcomes (Taken/Received).
	var outcomeStats []struct {
		PartialPostcode string `gorm:"column:PartialPostcode"`
		Count           int    `gorm:"column:count"`
	}

	db.Raw(`
		SELECT SUBSTRING(locations.name, 1, LENGTH(locations.name) - 2) AS PartialPostcode,
			   COUNT(*) AS count
		FROM pc
		INNER JOIN messages ON messages.locationid = pc.locationid
		INNER JOIN messages_outcomes ON messages_outcomes.msgid = messages.id
		INNER JOIN locations ON messages.locationid = locations.id
		WHERE locations.type = 'Postcode'
			AND LOCATE(' ', locations.name) > 0
			AND messages.arrival BETWEEN ? AND ?
			AND outcome IN ('Taken', 'Received')
		GROUP BY PartialPostcode
		ORDER BY locations.name`, startStr, endStr).Scan(&outcomeStats)

	for _, stat := range outcomeStats {
		ps := ret[stat.PartialPostcode]
		ps.Outcomes += stat.Count
		ret[stat.PartialPostcode] = ps
	}

	// Query weights.
	// First get the average weight.
	var avgResult struct {
		Average *float64 `gorm:"column:average"`
	}
	db.Raw(`SELECT SUM(popularity * weight) / SUM(popularity) AS average
		FROM items WHERE weight IS NOT NULL AND weight != 0`).Scan(&avgResult)

	avg := float64(0)
	if avgResult.Average != nil {
		avg = *avgResult.Average
	}

	var weightStats []struct {
		PartialPostcode string  `gorm:"column:PartialPostcode"`
		Weight          float64 `gorm:"column:weight"`
	}

	db.Raw(fmt.Sprintf(`
		SELECT SUBSTRING(locations.name, 1, LENGTH(locations.name) - 2) AS PartialPostcode,
			   SUM(COALESCE(weight, %f)) AS weight
		FROM pc
		INNER JOIN messages ON messages.locationid = pc.locationid
		INNER JOIN messages_outcomes ON messages_outcomes.msgid = messages.id
		INNER JOIN messages_items mi ON messages.id = mi.msgid
		INNER JOIN items i ON mi.itemid = i.id
		INNER JOIN locations ON messages.locationid = locations.id
		WHERE locations.type = 'Postcode'
			AND LOCATE(' ', locations.name) > 0
			AND messages.arrival BETWEEN ? AND ?
			AND outcome IN ('Taken', 'Received')
		GROUP BY PartialPostcode
		ORDER BY locations.name`, avg), startStr, endStr).Scan(&weightStats)

	for _, stat := range weightStats {
		ps := ret[stat.PartialPostcode]
		ps.Weight += stat.Weight
		ret[stat.PartialPostcode] = ps
	}

	// Query searches.
	var searchStats []struct {
		PartialPostcode string `gorm:"column:PartialPostcode"`
		Count           int    `gorm:"column:count"`
	}

	db.Raw(`
		SELECT SUBSTRING(locations.name, 1, LENGTH(locations.name) - 2) AS PartialPostcode,
			   COUNT(*) AS count
		FROM pc
		INNER JOIN search_history ON search_history.locationid = pc.locationid
		INNER JOIN locations ON search_history.locationid = locations.id
		WHERE locations.type = 'Postcode'
			AND LOCATE(' ', locations.name) > 0
			AND search_history.date BETWEEN ? AND ?
		GROUP BY PartialPostcode
		ORDER BY locations.name`, startStr, endStr).Scan(&searchStats)

	for _, stat := range searchStats {
		ps := ret[stat.PartialPostcode]
		ps.Searches += stat.Count
		ret[stat.PartialPostcode] = ps
	}

	// Clean up temporary table.
	db.Exec(`DROP TEMPORARY TABLE IF EXISTS pc`)

	return ret, nil
}

// parseRelativeDate parses dates like "365 days ago", "30 days ago", "today".
func parseRelativeDate(s string) (time.Time, error) {
	if s == "" || s == "today" {
		return time.Now(), nil
	}

	// Try parsing as a standard date first.
	t, err := time.Parse("2006-01-02", s)
	if err == nil {
		return t, nil
	}

	// Try parsing relative dates like "365 days ago", "30 days ago".
	var days int
	if _, err := fmt.Sscanf(s, "%d days ago", &days); err == nil {
		return time.Now().AddDate(0, 0, -days), nil
	}

	// Try single day "1 day ago".
	if _, err := fmt.Sscanf(s, "%d day ago", &days); err == nil {
		return time.Now().AddDate(0, 0, -days), nil
	}

	return time.Time{}, fmt.Errorf("cannot parse date: %s", s)
}
