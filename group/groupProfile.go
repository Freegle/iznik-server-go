package group

func (GroupProfile) TableName() string {
	return "groups_images"
}

type GroupProfile struct {
	ID        uint64 `json:"id" gorm:"primary_key"`
	Groupid   uint64 `json:"-"`
	Path      string `json:"path"`
	Paththumb string `json:"paththumb"`
}
