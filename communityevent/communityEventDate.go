package communityevent

import "time"

func (CommunityEventDate) TableName() string {
	return "communityevents_dates"
}

type CommunityEventDate struct {
	ID    uint64    `json:"id" gorm:"primary_key"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
