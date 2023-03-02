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

	// There's a slight privacy issue in returning the approval id.  Potentially we might not want users to know that
	// their messages are moderated, and we might not want to reveal the id of the moderator.  However it's a useful
	// thing to be able to show mods themselves.
	Approvedby uint64 `json:"approvedby"`
}
