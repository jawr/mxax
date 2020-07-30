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

// Account represents a user of the service
type Account struct {
	ID int

	Username string
	Password []byte

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
