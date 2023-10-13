package log

import (
	"github.com/freegle/iznik-server-go/database"
	"time"
)

// Log types must match the enumeration in the logs table.
const LOG_TYPE_GROUP = "Group"

const LOG_TYPE_USER = "User"

const LOG_TYPE_MESSAGE = "Message"

const LOG_TYPE_CONFIG = "Config"

const LOG_TYPE_STDMSG = "StdMsg"

const LOG_TYPE_BULKOP = "BulkOp"

const LOG_TYPE_LOCATION = "Location"

const LOG_TYPE_CHAT = "Chat"

const LOG_SUBTYPE_CREATED = "Created"

const LOG_SUBTYPE_DELETED = "Deleted"

const LOG_SUBTYPE_EDIT = "Edit"

const LOG_SUBTYPE_APPROVED = "Approved"

const LOG_SUBTYPE_REJECTED = "Rejected"

const LOG_SUBTYPE_RECEIVED = "Received"

const LOG_SUBTYPE_NOTSPAM = "NotSpam"

const LOG_SUBTYPE_HOLD = "Hold"

const LOG_SUBTYPE_RELEASE = "Release"

const LOG_SUBTYPE_FAILURE = "Failure"

const LOG_SUBTYPE_JOINED = "Joined"

const LOG_SUBTYPE_APPLIED = "Applied"

const LOG_SUBTYPE_LEFT = "Left"

const LOG_SUBTYPE_REPLIED = "Replied"

const LOG_SUBTYPE_MAILED = "Mailed"

const LOG_SUBTYPE_LOGIN = "Login"

const LOG_SUBTYPE_LOGOUT = "Logout"

const LOG_SUBTYPE_CLASSIFIED_SPAM = "ClassifiedSpam"

const LOG_SUBTYPE_SUSPECT = "Suspect"

const LOG_SUBTYPE_SENT = "Sent"

const LOG_SUBTYPE_OUR_POSTING_STATUS = "OurPostingStatus"

const LOG_SUBTYPE_OUR_EMAIL_FREQUENCY = "OurEmailFrequency"

const LOG_SUBTYPE_ROLE_CHANGE = "RoleChange"

const LOG_SUBTYPE_MERGED = "Merged"

const LOG_SUBTYPE_SPLIT = "Split"

const LOG_SUBTYPE_MAILOFF = "MailOff"

const LOG_SUBTYPE_EVENTSOFF = "EventsOff"

const LOG_SUBTYPE_NEWSLETTERSOFF = "NewslettersOff"

const LOG_SUBTYPE_RELEVANTOFF = "RelevantOff"

const LOG_SUBTYPE_VOLUNTEERSOFF = "VolunteersOff"

const LOG_SUBTYPE_BOUNCE = "Bounce"

const LOG_SUBTYPE_SUSPEND_MAIL = "SuspendMail"

const LOG_SUBTYPE_AUTO_REPOSTED = "Autoreposted"

const LOG_SUBTYPE_OUTCOME = "Outcome"

const LOG_SUBTYPE_NOTIFICATIONOFF = "NotificationOff"

const LOG_SUBTYPE_AUTO_APPROVED = "Autoapproved"

const LOG_SUBTYPE_UNBOUNCE = "Unbounce"

const LOG_SUBTYPE_WORRYWORDS = "WorryWords"

const LOG_SUBTYPE_POSTCODECHANGE = "PostcodeChange"

const LOG_SUBTYPE_REPOST = "Repost"

type LogEntry struct {
	ID        uint64
	Timestamp time.Time
	Byuser    *uint64
	Type      string
	Subtype   string
	Groupid   *uint64
	User      *uint64
	Msgid     *uint64
	Configid  *uint64
	Stdmsgid  *uint64
	Bulkopid  *uint64
	Text      *string
}

func (LogEntry) TableName() string {
	return "logs"
}

func Log(entry LogEntry) {
	db := database.DBConn
	entry.Timestamp = time.Now()
	db.Create(&entry)
}
