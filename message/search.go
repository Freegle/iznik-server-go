package message

import (
	"github.com/freegle/iznik-server-go/utils"
	"gorm.io/gorm"
	"log"
	"strconv"
	"strings"
	"time"
)

const SEARCH_LIMIT = 100

type Matchedon struct {
	Type string `json:"type"`
	Word string `json:"word"`
}

type SearchResult struct {
	ID        uint64    `json:"-" gorm:"primary_key"`
	Msgid     uint64    `json:"id"`
	Arrival   time.Time `json:"arrival"`
	Groupid   uint64    `json:"groupid"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Tag       string    `json:"-"`
	Word      string    `json:"word"`
	Type      string    `json:"type"`
	Matchedon Matchedon `json:"matchedon" gorm:"-"`
}

func GetWords(search string) []string {
	common := [...]string{
		"the", "old", "new", "please", "thanks", "with", "offer", "taken", "wanted", "received", "attachment", "offered", "and",
		"freegle", "freecycle", "for", "large", "small", "are", "but", "not", "you", "all", "any", "can", "her", "was", "one", "our",
		"out", "day", "get", "has", "him", "how", "now", "see", "two", "who", "did", "its", "let", "she", "too", "use", "plz",
		"of", "to", "in", "it", "is", "be", "as", "at", "so", "we", "he", "by", "or", "on", "do", "if", "me", "my", "up", "an", "go", "no", "us", "am",
		"working", "broken", "black", "white", "grey", "blue", "green", "red", "yellow", "brown", "orange", "pink", "machine", "size", "set",
		"various", "assorted", "different", "bits", "ladies", "gents", "kids", "nice", "brand", "pack", "soft", "single", "double",
		"top", "plastic", "electric",
	}

	// Remove all punctuation and split on word boundaries
	words := strings.FieldsFunc(strings.ToLower(search), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	})

	// Filter out common words
	var filtered []string
	for _, word := range words {
		if len(word) > 2 {
			found := false
			for _, c := range common {
				if word == c {
					found = true
					break
				}
			}

			if !found {
				// Trim space
				filtered = append(filtered, strings.TrimSpace(word))
			}
		}
	}

	return filtered
}

func processResults(tag string, results []SearchResult) []SearchResult {
	for i, _ := range results {
		results[i].Matchedon.Type = tag
		results[i].Matchedon.Word = results[i].Word
	}

	return results
}

func groupFilter(groupids []uint64) string {
	ret := ""

	if groupids != nil && len(groupids) > 0 {
		ret = " AND messages_spatial.groupid IN ("
		for i, id := range groupids {
			if i > 0 {
				ret += ","
			}
			ret += strconv.FormatUint(id, 10)
		}
		ret += ") "
	}

	return ret
}

func typeFilter(msgtype string) string {
	var ret string

	switch msgtype {
	case utils.OFFER:
		ret = " AND messages_spatial.msgtype = 'Offer' "
	case utils.WANTED:
		ret = " AND messages_spatial.msgtype = 'Wanted' "
	default:
		ret = ""
	}

	return ret
}

func boxFilter(nelatf float32, nelngf float32, swlatf float32, swlngf float32) string {
	var ret string

	// Add in some padding.  This copes with blurring and also shows some fairly nearby results which might not be
	// on the map.
	if nelatf != 0 && nelngf != 0 && swlatf != 0 && swlngf != 0 {
		nelat := strconv.FormatFloat(float64(nelatf+0.02), 'f', -1, 32)
		nelng := strconv.FormatFloat(float64(nelngf+0.02), 'f', -1, 32)
		swlat := strconv.FormatFloat(float64(swlatf-0.02), 'f', -1, 32)
		swlng := strconv.FormatFloat(float64(swlngf-0.02), 'f', -1, 32)
		srid := strconv.FormatInt(utils.SRID, 10)
		ret = " ST_Contains(ST_SRID(POLYGON(LINESTRING(" +
			"POINT(" + swlng + ", " + swlat + "), " +
			"POINT(" + swlng + ", " + nelat + "), " +
			"POINT(" + nelng + ", " + nelat + "), " +
			"POINT(" + nelng + ", " + swlat + "), " +
			"POINT(" + swlng + ", " + swlat + "))), " + srid + "), point) AND "
	}

	return ret
}

func GetWordsExact(db *gorm.DB, words []string, limit int64, groupids []uint64, msgtype string, nelat float32, nelng float32, swlat float32, swlng float32) []SearchResult {
	sql := "SELECT COUNT(*) AS wordmatch, messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, messages_spatial.msgtype as type, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index " +
		"INNER JOIN words ON messages_index.wordid = words.id " +
		"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid " +
		"WHERE " +
		boxFilter(nelat, nelng, swlat, swlng) +
		"word IN ("

	args := []interface{}{}

	for i, w := range words {
		if i > 0 {
			sql += ","
		}

		sql += "? "
		args = append(args, w)
	}

	sql += ") " +
		groupFilter(groupids) +
		typeFilter(msgtype) +
		"GROUP BY msgid ORDER BY wordmatch DESC, popularity DESC LIMIT ?;"

	args = append(args, limit)

	var res []SearchResult
	db.Raw(sql, args...).Scan(&res)

	return processResults("Exact", res)
}

func GetWordsTypo(db *gorm.DB, words []string, limit int64, groupids []uint64, msgtype string, nelat float32, nelng float32, swlat float32, swlng float32) []SearchResult {
	var res []SearchResult

	if len(words) > 0 {
		// TODO Multiple words.
		var prefix = words[0][0:1] + "%"

		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered in GetWordsTypo: %v", r)
			}
		}()

		db.Raw("SELECT messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, messages_spatial.msgtype as type, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index "+
			"INNER JOIN words ON messages_index.wordid = words.id "+
			"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid "+
			"WHERE "+
			boxFilter(nelat, nelng, swlat, swlng)+
			"word LIKE ? AND damlevlim(word, ?, ?) < 2 "+
			groupFilter(groupids)+
			typeFilter(msgtype)+
			"ORDER BY popularity DESC LIMIT ?;", prefix, words[0], len(words[0]), limit).Scan(&res)
	}

	return processResults("Typo", res)
}

func GetWordsStarts(db *gorm.DB, words []string, limit int64, groupids []uint64, msgtype string, nelat float32, nelng float32, swlat float32, swlng float32) []SearchResult {
	// TODO Multiple words.
	var res []SearchResult
	db.Raw("SELECT messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, messages_spatial.msgtype as type, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index "+
		"INNER JOIN words ON messages_index.wordid = words.id "+
		"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid "+
		"WHERE "+
		boxFilter(nelat, nelng, swlat, swlng)+
		"word LIKE ? "+
		groupFilter(groupids)+
		typeFilter(msgtype)+
		"ORDER BY popularity DESC LIMIT ?;", words[0]+"%", limit).Scan(&res)

	return processResults("StartsWith", res)
}

func GetWordsSounds(db *gorm.DB, words []string, limit int64, groupids []uint64, msgtype string, nelat float32, nelng float32, swlat float32, swlng float32) []SearchResult {
	// TODO Multiple words.
	var res []SearchResult
	db.Raw("SELECT messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, messages_spatial.msgtype as type, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index "+
		"INNER JOIN words ON messages_index.wordid = words.id "+
		"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid "+
		"WHERE "+
		boxFilter(nelat, nelng, swlat, swlng)+
		"soundex = SUBSTRING(SOUNDEX(?), 1, 10) "+
		groupFilter(groupids)+
		typeFilter(msgtype)+
		"ORDER BY popularity DESC LIMIT ?;", words[0]+"%", limit).Scan(&res)

	return processResults("SoundsLike", res)
}
