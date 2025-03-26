package user

type UserBanned struct {
	Userid  uint64 `json:"userid" gorm:"primaryKey;autoIncrement:false"`
	Groupid uint64 `json:"groupid" gorm:"primaryKey;autoIncrement:false"`
	Date    string `json:"date"`
	Byuser  uint64 `json:"byuser"`
}

func (UserBanned) TableName() string {
	return "users_banned"
}
