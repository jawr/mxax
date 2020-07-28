package account

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgtype"
)

// ReturnPath represents a route back for bounces
type ReturnPath struct {
	ID uuid.UUID

	AliasID  int
	ReturnTo string

	CreatedAt  time.Time
	ReturnedAt pgtype.Timestamp
}

// Alias represents an rule created by an Account
// for matching and forwarding to destinations
// there is a join table between these two
// structs
type Alias struct {
	ID int

	DomainID int

	Rule string

	// internal use
	rule         *regexp.Regexp
	destinations []int

	MetaData
}

// Take an email local part and check it against the
// Alias' rule. regexp is compiled lazily
func (a *Alias) Check(user string) (bool, error) {
	if a.rule == nil {
		rule := strings.ToLower(a.Rule)
		rule = strings.TrimPrefix(rule, "^")
		rule = strings.TrimSuffix(rule, "$")
		rule = "^" + rule + "$"
		r, err := regexp.Compile(rule)
		if err != nil {
			return false, err
		}
		a.rule = r
	}

	return a.rule.MatchString(user), nil
}

// Destination represents an email address
// that an alias will send to. It's split out
// in to it's own struct so it is normalised
type Destination struct {
	ID int

	AccountID int

	Address string

	MetaData
}
