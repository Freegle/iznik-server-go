package misc

import (
	"encoding/json"
	"fmt"
	"os"
)

func GetUploadcareUrl(uid string, mods string) string {
	CDN := os.Getenv("UPLOADCARE_CDN")

	if len(CDN) == 0 {
		CDN = "https://ucarecdn.com/"
	}

	url := CDN + uid + "/"

	var modsMap map[string]string
	err := json.Unmarshal([]byte(mods), &modsMap)

	if err != nil {
		return url
	}

	if len(modsMap) > 0 {
		url += "-/"

		for mod, val := range modsMap {
			url += fmt.Sprintf("%s/%s/", mod, val)
		}
	}

	return url
}
