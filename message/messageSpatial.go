package message

import "time"

type MessagesSpatial struct {
	ID         uint64    `json:"id" gorm:"primary_key"`
	Successful bool      `json:"successful"`
	Promised   bool      `json:"promised"`
	Groupid    uint64    `json:"groupid"`
	Type       string    `json:"type"`
	Arrival    time.Time `json:"arrival"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
}
