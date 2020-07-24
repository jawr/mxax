package account

import "github.com/jackc/pgtype"

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
