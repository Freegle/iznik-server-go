package message

import "time"

func (MessageOutcome) TableName() string {
	return "messages_outcomes"
}

type MessageOutcome struct {
	ID        uint64    `json:"id" gorm:"primary_key"`
	Msgid     uint64    `json:"msgid"`
	Timestamp time.Time `json:"timestamp"`
	Outcome   string    `json:"outcome"`
}
