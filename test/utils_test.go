package test

import (
	"github.com/freegle/iznik-server-go/utils"
	"github.com/stretchr/testify/assert"
	"math"
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

func TestBlurBasic(t *testing.T) {
	// Test that blur returns different coordinates when blurring.
	lat, lng := utils.Blur(51.5074, -0.1278, 1000)

	// Should not be exactly the same.
	assert.NotEqual(t, 51.5074, lat)
	assert.NotEqual(t, -0.1278, lng)
}

func TestBlurDeterministic(t *testing.T) {
	// Same input should produce same output.
	lat1, lng1 := utils.Blur(51.5074, -0.1278, 1000)
	lat2, lng2 := utils.Blur(51.5074, -0.1278, 1000)

	assert.Equal(t, lat1, lat2)
	assert.Equal(t, lng1, lng2)
}

func TestBlurZeroDistance(t *testing.T) {
	// Zero blur distance should return approximately the same coordinates.
	lat, lng := utils.Blur(51.5074, -0.1278, 0)

	assert.InDelta(t, 51.507, lat, 0.001)
	assert.InDelta(t, -0.128, lng, 0.001)
}

func TestBlurInvalidCoordinates(t *testing.T) {
	// Invalid coordinates should return Dunsop Bridge (center of Britain).
	lat, lng := utils.Blur(200, 500, 0)

	assert.InDelta(t, 53.945, lat, 0.001)
	assert.InDelta(t, -2.521, lng, 0.001)
}

func TestBlurPrecision(t *testing.T) {
	// Should return coordinates with limited precision (3 decimal places).
	lat, lng := utils.Blur(51.5074567890, -0.127812345, 100)

	// Check it's rounded.
	latRounded := math.Round(lat*1000) / 1000
	lngRounded := math.Round(lng*1000) / 1000
	assert.Equal(t, lat, latRounded)
	assert.Equal(t, lng, lngRounded)
}

func TestOurDomainTrue(t *testing.T) {
	assert.Equal(t, 1, utils.OurDomain("test@users.ilovefreegle.org"))
	assert.Equal(t, 1, utils.OurDomain("test@groups.ilovefreegle.org"))
	assert.Equal(t, 1, utils.OurDomain("test@direct.ilovefreegle.org"))
	assert.Equal(t, 1, utils.OurDomain("test@republisher.freegle.in"))
}

func TestOurDomainFalse(t *testing.T) {
	assert.Equal(t, 0, utils.OurDomain("test@gmail.com"))
	assert.Equal(t, 0, utils.OurDomain("test@yahoo.com"))
	assert.Equal(t, 0, utils.OurDomain("test@example.org"))
}

func TestOurDomainPartialMatch(t *testing.T) {
	// Should match if domain appears anywhere in email.
	assert.Equal(t, 1, utils.OurDomain("something-users.ilovefreegle.org@proxy.com"))
}
