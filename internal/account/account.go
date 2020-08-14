package account

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgtype"
)

type MetaData struct {
	CreatedAt time.Time
	UpdatedAt pgtype.Timestamp
	DeletedAt pgtype.Timestamp
}

type AccountType int

const (
	AccountTypeFree AccountType = iota
	AccountTypeSubscription
)

func (at AccountType) String() string {
	switch at {
	case AccountTypeSubscription:
		return "Subscription"
	case AccountTypeFree:
		fallthrough
	default:
		return "Free"
	}
}

type LogLevel int

const (
	LogLevelAll LogLevel = iota
	LogLevelBounceAndReject
	LogLevelBounce
	LogLevelReject
	LogLevelNone
)

func (ll LogLevel) String() string {
	switch ll {
	case LogLevelBounceAndReject:
		return "Bounce & Reject"
	case LogLevelBounce:
		return "Bounce"
	case LogLevelReject:
		return "Reject"
	case LogLevelNone:
		return "None"
	case LogLevelAll:
		fallthrough
	default:
		return "All"
	}
}

// Account represents a user of the service
type Account struct {
	ID int

	Email    string
	Password []byte

	SMTPPassword []byte

	AccountType AccountType
	LogLevel    LogLevel

	VerifyCode uuid.UUID
	VerifiedAt time.Time

	MetaData
	LastLoginAt pgtype.Timestamp
}

type Security struct {
	ID int

	AccountID int
	Password  []byte

	MetaData
}
