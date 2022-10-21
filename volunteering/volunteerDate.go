package volunteering

import "time"

func (VolunteeringDate) TableName() string {
	return "volunteering_dates"
}

type VolunteeringDate struct {
	ID      uint64    `json:"id" gorm:"primary_key"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Applyby time.Time `json:"applyby"`
}
