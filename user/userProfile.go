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
	Ours      bool   `json:"ours"`
}

func ProfileSetPath(profileid uint64, url string, archived int, profile *UserProfile) {
	profile.ID = profileid

	if len(url) > 0 {
		// External.
		profile.Path = url
		profile.Paththumb = url
		profile.Ours = false
	} else if archived > 0 {
		// Archived.
		profile.Path = os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/uimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Paththumb = os.Getenv("IMAGE_ARCHIVED_DOMAIN") + "/tuimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Ours = true
	} else {
		// Still in DB.
		profile.Path = os.Getenv("IMAGE_DOMAIN") + "/uimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Paththumb = os.Getenv("IMAGE_DOMAIN") + "/tuimg_" + strconv.FormatUint(profileid, 10) + ".jpg"
		profile.Ours = true
	}
}
