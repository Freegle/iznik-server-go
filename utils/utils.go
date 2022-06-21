package utils

import (
	"github.com/tidwall/geodesic"
	"math"
	"regexp"
	"strconv"
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

func TidyName(name string) string {
	name = strings.TrimSpace(name)

	i := strings.Index(name, "@")

	if i != -1 {
		name = name[0:i]
	}

	if strings.Index(name, "FBUser") != -1 {
		// Very old name.
		name = ""
	}

	if len(name) == 32 {
		// A name derived from a Yahoo ID which is a hex string, which looks silly
		matched, _ := regexp.MatchString("[A-Za-z].*[0-9]|[0-9].*[A-Za-z]", name)

		if matched {
			name = ""
		}
	}

	if len(name) > 32 {
		// Stop silly long names.
		name = name[0:32] + "..."
	}

	if _, err := strconv.Atoi(name); err == nil {
		// Numeric names confuse the client.
		name = name + "."
	}

	if len(name) == 0 {
		// The PHP server will hopefully invent a better name for us soon.
		name = "A freegler"
	}

	// We hide the "-gxxx" part of names, which will almost always be for TN members.
	tnre := regexp.MustCompile("^([\\s\\S]+?)-g[0-9]+$")
	name = tnre.ReplaceAllString(name, "$1")

	return name
}
