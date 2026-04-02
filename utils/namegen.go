package utils

import (
	_ "embed"
	"encoding/json"
	"math/rand"
	"strconv"
	"strings"
	"sync"
)

//go:embed namedata/distinct_word_lengths.json
var wordLengthsData []byte

//go:embed namedata/word_start_bigrams.json
var wordStartBigramsData []byte

//go:embed namedata/trigrams.json
var trigramsData []byte

var (
	namegenWordLengths  []int64
	namegenBigrams      map[string]int64
	namegenTrigrams     map[string]map[string]int64
	namegenOnce         sync.Once
)

func initNamegen() {
	namegenOnce.Do(func() {
		json.Unmarshal(wordLengthsData, &namegenWordLengths)

		var rawBigrams map[string]string
		json.Unmarshal(wordStartBigramsData, &rawBigrams)
		namegenBigrams = make(map[string]int64, len(rawBigrams))
		for k, v := range rawBigrams {
			n, _ := strconv.ParseInt(v, 10, 64)
			namegenBigrams[strings.ToLower(k)] = n
		}

		var rawTrigrams map[string]map[string]string
		json.Unmarshal(trigramsData, &rawTrigrams)
		namegenTrigrams = make(map[string]map[string]int64, len(rawTrigrams))
		for k, inner := range rawTrigrams {
			lk := strings.ToLower(k)
			namegenTrigrams[lk] = make(map[string]int64, len(inner))
			for ik, iv := range inner {
				n, _ := strconv.ParseInt(iv, 10, 64)
				namegenTrigrams[lk][strings.ToLower(ik)] = n
			}
		}
	})
}

func weightedRandFromSlice(weights []int64) int {
	var total int64
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return 0
	}
	n := rand.Int63n(total) + 1
	for i, w := range weights {
		n -= w
		if n <= 0 {
			return i
		}
	}
	return len(weights) - 1
}

func weightedRandFromMap(weights map[string]int64) string {
	var total int64
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return ""
	}
	n := rand.Int63n(total) + 1
	for k, w := range weights {
		n -= w
		if n <= 0 {
			return k
		}
	}
	for k := range weights {
		return k
	}
	return ""
}

func fillWord(word string, length int, trigrams map[string]map[string]int64) string {
	for len(word) < length {
		tail := word
		if len(tail) > 2 {
			tail = tail[len(tail)-2:]
		}
		next, ok := trigrams[tail]
		if !ok {
			break
		}
		word += weightedRandFromMap(next)
	}
	return word
}

// GenerateName returns a plausible-sounding lowercase name using English
// trigram statistics, matching the V1 PHP Wordle-based name invention approach.
// Word length is chosen from lengths 4-10, weighted by real-word frequency.
func GenerateName() string {
	initNamegen()

	// Use lengths 4-10 (reasonable for display names), weighted by frequency.
	const minLen, maxLen = 4, 10
	if len(namegenWordLengths) <= maxLen {
		return "A freegler"
	}
	subset := namegenWordLengths[minLen : maxLen+1]
	length := weightedRandFromSlice(subset) + minLen

	start := weightedRandFromMap(namegenBigrams)
	if start == "" {
		return "A freegler"
	}

	return fillWord(start, length, namegenTrigrams)
}
