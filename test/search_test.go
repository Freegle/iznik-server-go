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
