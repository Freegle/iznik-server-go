package utils

import (
	"github.com/tidwall/geodesic"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// We have constants here rather than in the packages you might expect to avoid import loops.
const MESSAGE_INTERESTED = "Interested"

const OFFER = "Offer"
const WANTED = "Wanted"
const TAKEN = "Taken"
const RECEIVED = "Received"

const COLLECTION_APPROVED = "Approved"
const COLLECTION_PENDING = "Pending"
const COLLECTION_SPAM = "Spam"

const EMAIL_REGEXP = "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}\b"
const PHONE_REGEXP = "[0-9]{4,}"
const TN_REGEXP = "^([\\s\\S]+?)-g[0-9]+$"

const OPEN_AGE = 90
const CHAT_ACTIVE_LIMIT = 31
const CHAT_REPLY_GRACE = 30

const SUPPORTER_PERIOD = 360

const RATINGS_PERIOD = 182
const RATING_UP = "Up"
const RATING_DOWN = "Down"

const BLUR_NONE = 0
const BLUR_USER = 400
const BLUR_1K = 1000

const SRID = 3857

const CHAT_TYPE_USER2USER = "User2User"
const CHAT_TYPE_USER2MOD = "User2Mod"
const CHAT_STATUS_CLOSED = "Closed"
const CHAT_STATUS_BLOCKED = "Blocked"

const CHAT_MESSAGE_DEFAULT = "Default"

const CHAT_MESSAGE_MODMAIL = "ModMail"

const CHAT_MESSAGE_SYSTEM = "System"

const CHAT_MESSAGE_INTERESTED = "Interested"

const CHAT_MESSAGE_PROMISED = "Promised"

const CHAT_MESSAGE_RENEGED = "Reneged"

const CHAT_MESSAGE_REPORTEDUSER = "ReportedUser"

const CHAT_MESSAGE_COMPLETED = "Completed"

const CHAT_MESSAGE_IMAGE = "Image"

const CHAT_MESSAGE_ADDRESS = "Address"

const CHAT_MESSAGE_NUDGE = "Nudge"

func Blur(lat float64, lng float64, dist float64) (float64, float64) {
	var dlat, dlng float64
	var dir = (float64)(((int)(lat*1000) + (int)(lng*1000)) % 360)
	geodesic.WGS84.Direct(lat, lng, dir, dist, &dlat, &dlng, nil)

	// Don"t return pointless precision.
	return math.Round(dlat*1000) / 1000, math.Round(dlng*1000) / 1000
}

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
	tnre := regexp.MustCompile(TN_REGEXP)
	name = tnre.ReplaceAllString(name, "$1")

	return name
}
