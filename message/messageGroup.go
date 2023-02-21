package message

import "time"

type Tabler interface {
	TableName() string
}

func (MessageGroup) TableName() string {
	return "messages_groups"
}

type MessageGroup struct {
	Groupid     uint64    `json:"groupid"`
	Msgid       uint64    `json:"msgid"`
	Arrival     time.Time `json:"arrival"`
	Collection  string    `json:"collection"`
	Autoreposts uint      `json:"autoreposts"`
}
