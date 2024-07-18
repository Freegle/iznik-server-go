package user

import (
	"encoding/json"
	"github.com/freegle/iznik-server-go/misc"
	"os"
	"strconv"
	"strings"
)

type UserProfile struct {
	ID           uint64          `json:"id" gorm:"primary_key"`
	Userid       uint64          `json:"-"`
	Path         string          `json:"path"`
	Paththumb    string          `json:"paththumb"`
	Ours         bool            `json:"ours"`
	Externaluid  string          `json:"externaluid"`
	Ouruid       string          `json:"ouruid"` // Temp until Uploadcare retired.
	Externalmods json.RawMessage `json:"externalmods"`
}

func ProfileSetPath(profileid uint64, url string, externaluid string, externalmods json.RawMessage, archived int, profile *UserProfile) {
	profile.ID = profileid

	if len(url) > 0 {
		// External.
		profile.Path = url
		profile.Paththumb = url
		profile.Ours = false
	} else if len(externaluid) > 0 {
		// Until Uploadcare is retired we need to return different variants to allow for client code
		// which doesn't yet know about our own image hosting.
		if strings.Contains(externaluid, "freegletusd-") {
			profile.Ouruid = externaluid
			profile.Externalmods = externalmods
			profile.Path = misc.GetImageDeliveryUrl(externaluid, string(externalmods))
			profile.Paththumb = misc.GetImageDeliveryUrl(externaluid, string(externalmods))
		} else {
			profile.Externaluid = externaluid
			profile.Externalmods = externalmods
			profile.Path = misc.GetUploadcareUrl(externaluid, string(externalmods))
			profile.Paththumb = misc.GetUploadcareUrl(externaluid, string(externalmods))
		}
	} else if archived > 0 {
		// Archived.
		profile.Path = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/uimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Paththumb = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tuimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Ours = true
	} else {
		// Still in DB.
		profile.Path = "https://" + os.Getenv("IMAGE_DOMAIN") + "/uimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Paththumb = "https://" + os.Getenv("IMAGE_DOMAIN") + "/tuimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Ours = true
	}
}
