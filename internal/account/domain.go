package account

import (
	"time"

	"github.com/jackc/pgtype"
	whoisParser "github.com/jawr/whois-parser-go"
	"github.com/likexian/whois-go"
)

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
	Rtype          string
	Value          string
	LastVerifiedAt pgtype.Timestamp
}

func (rr Record) IsComplete() bool {
	return !rr.LastVerifiedAt.Time.IsZero() && time.Since(rr.LastVerifiedAt.Time) > time.Hour*24

}

func GetDomainExpirationDate(name string) (time.Time, error) {
	whoisResult, err := whois.Whois(name)
	if err != nil {
		return time.Time{}, err
	}

	// parse and extract expiresAt
	whoisParsed, err := whoisParser.Parse(whoisResult)
	if err != nil {
		return time.Time{}, err
	}

	return whoisParsed.Domain.ExpirationDate, nil
}
