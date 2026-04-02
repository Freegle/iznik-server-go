package misc

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BuildChatImageUrl constructs the full and thumbnail URLs for a chat image.
// It uses external delivery if imageuid is set, the archived domain if archived > 0,
// or the live domain otherwise.
func BuildChatImageUrl(imageid uint64, imageuid string, imagemods string, archived int) (path string, paththumb string) {
	if imageuid != "" {
		url := GetImageDeliveryUrl(imageuid, imagemods)
		return url, url
	}
	idStr := fmt.Sprintf("%d", imageid)
	if archived > 0 {
		domain := os.Getenv("IMAGE_ARCHIVED_DOMAIN")
		return "https://" + domain + "/mimg_" + idStr + ".jpg",
			"https://" + domain + "/tmimg_" + idStr + ".jpg"
	}
	domain := os.Getenv("IMAGE_DOMAIN")
	return "https://" + domain + "/mimg_" + idStr + ".jpg",
		"https://" + domain + "/tmimg_" + idStr + ".jpg"
}

func GetImageDeliveryUrl(uid string, mods string) string {
	// We construct a wsrv.nl-compatible URL which points at our caching proxy.
	DELIVERY := os.Getenv("IMAGE_DELIVERY")
	UPLOADS := os.Getenv("UPLOADS")

	if len(DELIVERY) == 0 {
		DELIVERY = "https://delivery.ilovefreegle.org"
	}

	if len(UPLOADS) == 0 {
		UPLOADS = "https://uploads.ilovefreegle.org:8080/"
	}

	// Ensure DELIVERY doesn't already have ?url= appended (backward compat).
	DELIVERY = strings.TrimSuffix(DELIVERY, "?url=")

	// Strip "freegletusd-" prefix from the UID if present.
	if len(uid) > 12 {
		uid = uid[12:]
	}
	url := DELIVERY + "?url=" + UPLOADS + uid

	if len(mods) > 0 {
		// Add the stored mods to the URL.  Currently only rotate is stored.
		var modifiers = struct {
			Rotate int `json:"rotate"`
		}{}

		err := json.Unmarshal([]byte(mods), &modifiers)

		if err == nil {
			url += fmt.Sprintf("&ro=%d", modifiers.Rotate)
		}
	}

	return url
}
