package user

type UserProfile struct {
	ID        uint64 `json:"id" gorm:"primary_key"`
	Userid    uint64 `json:"-"`
	Path      string `json:"path"`
	Paththumb string `json:"paththumb"`
}
