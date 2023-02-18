package message

import (
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

const SEARCH_LIMIT = 100

type SearchResult struct {
	Msgid     uint64    `json:"id"`
	Arrival   time.Time `json:"arrival"`
	Groupid   uint64    `json:"groupid"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Tag       string    `json:"-"`
	Word      string    `json:"word"`
	Matchedon struct {
		Type string `json:"type"`
		Word string `json:"word"`
	} `json:"matchedon" gorm:"-"`
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
				filtered = append(filtered, word)
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

func GetWordsExact(db *gorm.DB, word string, limit int64, groupids []uint64) []SearchResult {
	var res []SearchResult
	db.Raw("SELECT messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index "+
		"INNER JOIN words ON messages_index.wordid = words.id "+
		"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid "+
		"WHERE word = ? "+
		groupFilter(groupids)+
		"ORDER BY popularity DESC LIMIT ?;", word, limit).Scan(&res)

	return processResults("Exact", res)
}

func GetWordsTypo(db *gorm.DB, word string, limit int64, groupids []uint64) []SearchResult {
	var res []SearchResult

	if len(word) > 0 {
		var prefix = word[0:1] + "%"

		db.Raw("SELECT messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index "+
			"INNER JOIN words ON messages_index.wordid = words.id "+
			"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid "+
			"WHERE word COLLATE 'utf8mb4_unicode_ci' LIKE ? AND damlevlim(word, ?, ?) < 2 "+
			groupFilter(groupids)+
			"ORDER BY popularity DESC LIMIT ?;", prefix, word, len(word), limit).Scan(&res)
	}

	return processResults("Typo", res)
}

func GetWordsStarts(db *gorm.DB, word string, limit int64, groupids []uint64) []SearchResult {
	var res []SearchResult
	db.Raw("SELECT messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index "+
		"INNER JOIN words ON messages_index.wordid = words.id "+
		"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid "+
		"WHERE word LIKE ? "+
		groupFilter(groupids)+
		"ORDER BY popularity DESC LIMIT ?;", word+"%", limit).Scan(&res)

	return processResults("StartsWith", res)
}

func GetWordsSounds(db *gorm.DB, word string, limit int64, groupids []uint64) []SearchResult {
	var res []SearchResult
	db.Raw("SELECT messages_spatial.msgid, words.word, messages_spatial.groupid, messages_spatial.arrival, ST_Y(point) AS lat, ST_X(point) AS lng FROM messages_index "+
		"INNER JOIN words ON messages_index.wordid = words.id "+
		"INNER JOIN messages_spatial ON messages_index.msgid = messages_spatial.msgid "+
		"WHERE soundex = SUBSTRING(SOUNDEX(?), 1, 10) "+
		groupFilter(groupids)+
		"ORDER BY popularity DESC LIMIT ?;", word+"%", limit).Scan(&res)

	return processResults("SoundsLike", res)
}
