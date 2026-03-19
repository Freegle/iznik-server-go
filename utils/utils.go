package utils

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/geodesic"
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
const COLLECTION_REJECTED = "Rejected"
const COLLECTION_BANNED = "Banned"
const COLLECTION_DRAFT = "Draft"

const POSTING_STATUS_MODERATED = "MODERATED"
const POSTING_STATUS_PROHIBITED = "PROHIBITED"
const POSTING_STATUS_DEFAULT = "DEFAULT"

const EMAIL_REGEXP = "[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}\b"
const PHONE_REGEXP = "[0-9]{4,}"
const TN_REGEXP = "^([\\s\\S]+?)-g[0-9]+$"

const OPEN_AGE = 90
const OPEN_AGE_CHITCHAT = 365
const CHAT_ACTIVE_LIMIT = 31
const CHAT_REPLY_GRACE = 30
const NOTIFICATION_AGE = 90

const SUPPORTER_PERIOD = 360
const ADFREE_PERIOD = 31
const ADFREE_GRACE_PERIOD = 10 // Extra days for external (bank transfer) donations

const RATINGS_PERIOD = 182
const RATING_UP = "Up"
const RATING_DOWN = "Down"

const BLUR_NONE = 0
const BLUR_USER = 400
const BLUR_1K = 1000

const SRID = 3857

const CHAT_TYPE_USER2USER = "User2User"
const CHAT_TYPE_USER2MOD = "User2Mod"
const CHAT_TYPE_GROUP = "Group"
const CHAT_TYPE_MOD2MOD = "Mod2Mod"

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
const CHAT_MESSAGE_REFER_TO_SUPPORT = "ReferToSupport"

const NEWSFEED_TYPE_ALERT = "Alert"

const NEARBY = 50
const QUITE_NEARBY = 50

const OUTCOME_TAKEN = "Taken"
const OUTCOME_RECEIVED = "Received"
const OUTCOME_WITHDRAWN = "Withdrawn"
const OUTCOME_REPOST = "Repost"
const OUTCOME_EXPIRED = "Expired"
const OUTCOME_PARTIAL = "Partial"

const CHAT_STATUS_ONLINE = "Online"
const CHAT_STATUS_OFFLINE = "Offline"
const CHAT_STATUS_AWAY = "Away"
const CHAT_STATUS_CLOSED = "Closed"
const CHAT_STATUS_BLOCKED = "Blocked"

const ROLE_MEMBER = "Member"
const ROLE_MODERATOR = "Moderator"
const ROLE_OWNER = "Owner"

const SYSTEMROLE_USER = "User"
const SYSTEMROLE_MODERATOR = "Moderator"
const SYSTEMROLE_SUPPORT = "Support"
const SYSTEMROLE_ADMIN = "Admin"

const FREQUENCY_NEVER = 0
const FREQUENCY_IMMEDIATE = -1
const FREQUENCY_HOUR1 = 1
const FREQUENCY_HOUR2 = 2
const FREQUENCY_HOUR4 = 4
const FREQUENCY_HOUR8 = 8
const FREQUENCY_DAILY = 24

// RandomHex generates a random hex string of n bytes (2n hex chars).
func RandomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NilIfEmpty returns nil if the string is empty, for use in SQL NULL inserts.
func NilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// NilIfZero returns nil if the value is zero, for use in SQL NULL inserts.
func NilIfZero(v uint64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func Blur(lat float64, lng float64, dist float64) (float64, float64) {
	var dlat, dlng float64

	// Some old posts have invalid lat/lng values, which would result in us returning NaN.
	if lat > 90 || lat < -90 || lng > 180 || lng < -180 {
		// Use the center of Britain.  Dunsop Bridge has lovely ducks.
		lat = 53.945
		lng = -2.5209
	}

	var dir = (float64)(((int)(lat*1000) + (int)(lng*1000)) % 360)
	geodesic.WGS84.Direct(lat, lng, dir, dist, &dlat, &dlng, nil)

	// Don"t return pointless precision.
	return math.Round(dlat*1000) / 1000, math.Round(dlng*1000) / 1000
}

// Haversine returns the great-circle distance in miles between two lat/lng points.
func Haversine(lat1, lng1, lat2, lng2 float64) float64 {
	var dist float64
	geodesic.WGS84.Inverse(lat1, lng1, lat2, lng2, &dist, nil, nil)
	return dist / 1609.344 // metres to miles
}

type LatLng struct {
	Lat float32
	Lng float32
}

// CountryName converts a 2-letter ISO 3166-1 alpha-2 country code to a full
// English country name.  Returns the name and true if found, or "" and false
// if the code is unknown.
func CountryName(code string) (string, bool) {
	name, ok := isoCountries[strings.ToUpper(code)]
	return name, ok
}

// isoCountries maps ISO 3166-1 alpha-2 codes to English country names.
// Only European + common codes are included; extend as needed.
var isoCountries = map[string]string{
	"AD": "Andorra", "AE": "United Arab Emirates", "AF": "Afghanistan",
	"AG": "Antigua and Barbuda", "AL": "Albania", "AM": "Armenia",
	"AO": "Angola", "AR": "Argentina", "AT": "Austria", "AU": "Australia",
	"AZ": "Azerbaijan", "BA": "Bosnia and Herzegovina", "BB": "Barbados",
	"BD": "Bangladesh", "BE": "Belgium", "BG": "Bulgaria", "BH": "Bahrain",
	"BN": "Brunei", "BO": "Bolivia", "BR": "Brazil", "BS": "Bahamas",
	"BW": "Botswana", "BY": "Belarus", "BZ": "Belize", "CA": "Canada",
	"CH": "Switzerland", "CL": "Chile", "CM": "Cameroon", "CN": "China",
	"CO": "Colombia", "CR": "Costa Rica", "CU": "Cuba", "CY": "Cyprus",
	"CZ": "Czech Republic", "DE": "Germany", "DK": "Denmark", "DO": "Dominican Republic",
	"DZ": "Algeria", "EC": "Ecuador", "EE": "Estonia", "EG": "Egypt",
	"ES": "Spain", "ET": "Ethiopia", "FI": "Finland", "FJ": "Fiji",
	"FR": "France", "GB": "United Kingdom", "GE": "Georgia", "GH": "Ghana",
	"GR": "Greece", "GT": "Guatemala", "HK": "Hong Kong", "HN": "Honduras",
	"HR": "Croatia", "HU": "Hungary", "ID": "Indonesia", "IE": "Ireland",
	"IL": "Israel", "IN": "India", "IQ": "Iraq", "IR": "Iran",
	"IS": "Iceland", "IT": "Italy", "JM": "Jamaica", "JO": "Jordan",
	"JP": "Japan", "KE": "Kenya", "KG": "Kyrgyzstan", "KH": "Cambodia",
	"KR": "South Korea", "KW": "Kuwait", "KZ": "Kazakhstan", "LA": "Laos",
	"LB": "Lebanon", "LI": "Liechtenstein", "LK": "Sri Lanka", "LT": "Lithuania",
	"LU": "Luxembourg", "LV": "Latvia", "LY": "Libya", "MA": "Morocco",
	"MC": "Monaco", "MD": "Moldova", "ME": "Montenegro", "MG": "Madagascar",
	"MK": "North Macedonia", "ML": "Mali", "MM": "Myanmar", "MN": "Mongolia",
	"MT": "Malta", "MU": "Mauritius", "MV": "Maldives", "MW": "Malawi",
	"MX": "Mexico", "MY": "Malaysia", "MZ": "Mozambique", "NA": "Namibia",
	"NG": "Nigeria", "NI": "Nicaragua", "NL": "Netherlands", "NO": "Norway",
	"NP": "Nepal", "NZ": "New Zealand", "OM": "Oman", "PA": "Panama",
	"PE": "Peru", "PG": "Papua New Guinea", "PH": "Philippines", "PK": "Pakistan",
	"PL": "Poland", "PR": "Puerto Rico", "PS": "Palestine", "PT": "Portugal",
	"PY": "Paraguay", "QA": "Qatar", "RO": "Romania", "RS": "Serbia",
	"RU": "Russia", "RW": "Rwanda", "SA": "Saudi Arabia", "SD": "Sudan",
	"SE": "Sweden", "SG": "Singapore", "SI": "Slovenia", "SK": "Slovakia",
	"SN": "Senegal", "SO": "Somalia", "SV": "El Salvador", "SY": "Syria",
	"TH": "Thailand", "TN": "Tunisia", "TR": "Turkey", "TT": "Trinidad and Tobago",
	"TW": "Taiwan", "TZ": "Tanzania", "UA": "Ukraine", "UG": "Uganda",
	"US": "United States", "UY": "Uruguay", "UZ": "Uzbekistan", "VE": "Venezuela",
	"VN": "Vietnam", "YE": "Yemen", "ZA": "South Africa", "ZM": "Zambia",
	"ZW": "Zimbabwe",
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
		// Fallback display name when no name can be derived.
		name = "A freegler"
	}

	// We hide the "-gxxx" part of names, which will almost always be for TN members.
	tnre := regexp.MustCompile(TN_REGEXP)
	name = tnre.ReplaceAllString(name, "$1")

	return name
}
