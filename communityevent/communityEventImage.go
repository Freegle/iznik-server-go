package communityevent

func (CommunityEventImage) TableName() string {
	return "communityevents_images"
}

type CommunityEventImage struct {
	ID             uint64 `json:"id" gorm:"primary_key"`
	Archived       int    `json:"-"`
	Volunteeringid uint64 `json:"-"`
	Path           string `json:"path"`
	Paththumb      string `json:"paththumb"`
}
