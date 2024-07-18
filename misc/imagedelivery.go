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
		UPLOADS = "https://uploads.ilovefreegle.org/"
	}

	url := DELIVERY + UPLOADS + uid + "/"

	if len(mods) > 0 {
		// Add the stored mods to the URL.  Currently only rotate is stored.
		var modsMap map[string]string
		err := json.Unmarshal([]byte(mods), &modsMap)

		if err != nil {
			if len(modsMap) > 0 {
				for mod, val := range modsMap {
					if mod == "rotate" {
						url += fmt.Sprintf("?ro=", mod, val)
					}
				}
			}
		}
	}

	return url
}
