package message

import (
	"github.com/freegle/iznik-server-go/database"
	"strings"
	"time"
)

type SearchResult struct {
	Msgid       uint64    `json:"id"`
	Arrival     uint64    `json:"-"`
	ArrivalTime time.Time `json:"arrival"`
	Groupid     uint64    `json:"groupid"`
	Lat         float32   `json:"lat"`
	Lng         float32   `json:"lng"`
	Tag         string    `json:"-"`
	Word        string    `json:"word"`
	Matchedon   struct {
		Type string `json:"type"`
		Word string `json:"word"`
	} `json:"matchedon"`
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

func GetWordsExact(word string, limit int64) []SearchResult {
	db := database.DBConn
	res := []SearchResult{}
	db.Raw("SELECT msgid, words.word, groupid, -arrival AS arrival FROM messages_index "+
		"INNER JOIN words ON messages_index.wordid = words.id "+
		"WHERE word = ? "+
		"ORDER BY popularity LIMIT ?;", word, limit).Scan(&res)

	for i, _ := range res {
		res[i].ArrivalTime = time.Unix(int64(res[i].Arrival), 0)
		res[i].Matchedon.Type = "Exact"
		res[i].Matchedon.Word = res[i].Word
	}

	return res
}
