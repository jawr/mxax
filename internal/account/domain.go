package account

import "github.com/jackc/pgtype"

// Domain is attached to an Account and must
// be verified before being used. Alias' are
// attached to the Domain for forwarding
type Domain struct {
	ID int

	AccountID int

	Name string

	// Verification
	VerifyCode string
	VerifiedAt pgtype.Timestamp

	// when the domain expires
	ExpiresAt pgtype.Date

	MetaData
}

type Record struct {
	ID             int
	DomainID       int
	Host           string
	RType          string
	Value          string
	LastVerifiedAt pgtype.Timestamp
}
