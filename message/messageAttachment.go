package message

import (
	"encoding/json"
)

func (MessageAttachment) TableName() string {
	return "messages_attachments"
}

type MessageAttachment struct {
	ID           uint64          `json:"id" gorm:"primary_key"`
	Msgid        uint64          `json:"-"`
	Path         string          `json:"path"`
	Paththumb    string          `json:"paththumb"`
	Archived     int             `json:"archived"`
	Externaluid  string          `json:"externaluid"`
	Externalurl  string          `json:"externalurl"`
	Externalmods json.RawMessage `json:"externalmods"`
}
