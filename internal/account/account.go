package account

import (
	"time"

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

	MetaData
	LastLoginAt pgtype.Timestamp
}
