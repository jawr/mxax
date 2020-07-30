package account

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

// ReturnPath represents a route back for bounces
type ReturnPath struct {
	ID uuid.UUID

	AccountID int
	AliasID   int
	ReturnTo  string

	CreatedAt  time.Time
	ReturnedAt pgtype.Timestamp
}

// Alias represents an rule created by an Account
// for matching and forwarding to destinations
// there is a join table between these two
// structs
type Alias struct {
	ID int

	AccountID int
	DomainID  int

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

func GetAlias(ctx context.Context, db pgx.Tx, alias *Alias, aliasID int) error {
	return pgxscan.Get(
		ctx,
		db,
		alias,
		`
		SELECT * 
		FROM aliases
		WHERE
			id = $1
			AND deleted_at IS NULL
		`,
		aliasID,
	)
}

func CreateAlias(ctx context.Context, db pgx.Tx, rule string, domainID, destinationID int) error {
	// get domain
	var domain Domain
	err := GetDomainByID(ctx, db, &domain, domainID)
	if err != nil {
		return errors.WithMessage(err, "GetDomainByID")
	}

	// get destination
	var destination Destination
	err = GetDestinationByID(ctx, db, &destination, destinationID)
	if err != nil {
		return errors.WithMessage(err, "GetDestinationByID")
	}

	// create alias
	var aliasID int
	err = db.QueryRow(
		ctx,
		`
			WITH e AS (
				INSERT INTO aliases (account_id, domain_id, rule) 
				VALUES (current_setting('mxax.current_account_id')::INT, $1, $2) 
				ON CONFLICT (domain_id, rule) DO UPDATE SET deleted_at = NULL RETURNING id
			)
			SELECT * FROM e UNION SELECT id FROM aliases WHERE domain_id = $1 AND rule = $2
			`,
		domainID,
		rule,
	).Scan(&aliasID)
	if err != nil {
		return errors.WithMessage(err, "INSERT aliases")
	}

	return CreateAliasDestination(ctx, db, aliasID, destinationID)
}

func CreateAliasDestination(ctx context.Context, db pgx.Tx, aliasID, destinationID int) error {
	_, err := db.Exec(
		ctx,
		`
		INSERT INTO alias_destinations (account_id, alias_id, destination_id) 
			VALUES (current_setting('mxax.current_account_id')::INT, $1, $2)
			ON CONFLICT (alias_id, destination_id) DO UPDATE SET deleted_at = NULL 
		`,
		aliasID,
		destinationID,
	)
	if err != nil {
		return errors.WithMessage(err, "INSERT alias_destinations")
	}

	return nil
}
