package volunteering

func (VolunteeringImage) TableName() string {
	return "volunteering_images"
}

type VolunteeringImage struct {
	ID             uint64 `json:"id" gorm:"primary_key"`
	Archived       int    `json:"-"`
	Volunteeringid uint64 `json:"-"`
	Path           string `json:"path"`
	Paththumb      string `json:"paththumb"`
}
