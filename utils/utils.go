package utils

import (
	"github.com/tidwall/geodesic"
	"math"
	"strings"
)

const BLUR_NONE = 0

const BLUR_USER = 400

const BLUR_1K = 1000

func Blur(lat float64, lng float64, dist float64) (float64, float64) {
	var dlat, dlng float64
	var dir = (float64)(((int)(lat*1000) + (int)(lng*1000)) % 360)
	geodesic.WGS84.Direct(lat, lng, dir, dist, &dlat, &dlng, nil)

	// Don"t return pointless precision.
	return math.Round(dlat*1000) / 1000, math.Round(dlng*1000) / 1000
}

const SRID = 3857

type LatLng struct {
	Lat float32
	Lng float32
}

func OurDomain(email string) int {
	domains := [...]string{"users.ilovefreegle.org", "groups.ilovefreegle.org", "direct.ilovefreegle.org", "republisher.freegle.in"}

	for _, e := range domains {
		if strings.Index(email, e) != -1 {
			return 1
		}
	}

	return 0
}
