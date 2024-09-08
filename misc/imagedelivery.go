package misc

import (
	"encoding/json"
	"fmt"
	"os"
)

func GetImageDeliveryUrl(uid string, mods string) string {
	// We construct a wsrv.nl-compatible URL which points at our caching proxy.
	DELIVERY := os.Getenv("IMAGE_DELIVERY")
	UPLOADS := os.Getenv("UPLOADS")

	if len(DELIVERY) == 0 {
		DELIVERY = "https://delivery.ilovefreegle.org?url="
	}

	if len(UPLOADS) == 0 {
		UPLOADS = "https://uploads.ilovefreegle.org:8080/"
	}

	// Strip freegletusd- from the UID.
	uid = uid[12:]
	url := DELIVERY + UPLOADS + uid

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
