package group

func (GroupSponsor) TableName() string {
	return "groups_sponsorship"
}

type GroupSponsor struct {
	ID       uint64 `json:"id" gorm:"primary_key"`
	Groupid  uint64 `json:"-"`
	Name     string `json:"name"`
	Linkurl  string `json:"linkurl"`
	Imageurl string `json:"imageurl"`
	Tagline  string `json:"tagline"`
}
