package message

import "time"

func (MessageReply) TableName() string {
	return "chat_messages"
}

type MessageReply struct {
	ID       uint64    `json:"id" gorm:"primary_key"`
	Refmsgid uint64    `json:"refmsgid"`
	Date     time.Time `json:"date"`
}
