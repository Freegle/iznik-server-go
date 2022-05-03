package user

import (
	"os"
	"strconv"
)

type UserProfile struct {
	ID        uint64 `json:"id" gorm:"primary_key"`
	Userid    uint64 `json:"-"`
	Path      string `json:"path"`
	Paththumb string `json:"paththumb"`
}

func ProfileSetPath(profileid uint64, url string, archived int, profile *UserProfile) {
	profile.ID = profileid

	if len(url) > 0 {
		// External.
		profile.Path = url
		profile.Paththumb = url
	} else if archived > 0 {
		// Archived.
		profile.Path = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/uimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Paththumb = "https://" + os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tuimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
	} else {
		// Still in DB.
		profile.Path = "https://" + os.Getenv("USER_SITE") + "/uimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Paththumb = "https://" + os.Getenv("USER_SITE") + "/tuimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
	}
}
