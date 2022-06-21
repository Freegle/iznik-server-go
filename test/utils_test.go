package test

import (
	"github.com/freegle/iznik-server-go/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTidyName(t *testing.T) {
	assert.Equal(t, "test", utils.TidyName("test@test.com"))
	assert.Equal(t, "test", utils.TidyName(" test "))
	assert.Equal(t, "1.", utils.TidyName("1"))
	assert.Equal(t, "A freegler", utils.TidyName("01234567890abcdef01234567890abcd"))
	assert.Equal(t, "A freegler", utils.TidyName(" "))
	assert.Equal(t, "A freegler", utils.TidyName(" "))
	assert.Equal(t, "A freegler", utils.TidyName("FBUser123.4"))
	assert.Equal(t, "test", utils.TidyName("test-g123"))
	assert.Equal(t, "01234567890abcdef01234567890abcd...", utils.TidyName("01234567890abcdef01234567890abcd123"))
}
