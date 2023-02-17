package test

import (
	"github.com/freegle/iznik-server-go/message"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetWords(t *testing.T) {
	words := message.GetWords("Old sofa which is green")
	assert.Equal(t, 2, len(words))
	assert.Equal(t, "which", words[0])
	assert.Equal(t, "sofa", words[1])
}

func TestSearchExact(t *testing.T) {
	m := GetMessage(t)

	// Search on first word in subject - should find exact match.
	words := message.GetWords(m.Subject)

	results := message.GetWordsExact(words[0], 100)
	assert.Greater(t, len(results), 0)

	// Check that the message we expected is in the results
	found := false
	for _, result := range results {
		if result.Msgid == m.ID {
			found = true
			assert.Equal(t, words[0], result.Matchedon.Word)
		}
	}

	assert.True(t, found)
}
