package account

import (
	"time"

	"github.com/jackc/pgtype"
)

type MetaData struct {
	CreatedAt time.Time
	UpdatedAt pgtype.Time
	DeletedAt pgtype.Time
}

// Account represents a user of the service
type Account struct {
	ID int

	Username string
	Password []byte

	MetaData
	LastLoginAt pgtype.Time
}

// Domain is attached to an Account and must
// be verified before being used. Alias' are
// attached to the Domain for forwarding
type Domain struct {
	ID int

	AccountID int

	Name       string
	VerifiedAt pgtype.Time

	MetaData
}
