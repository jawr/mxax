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

func GetAlias(ctx context.Context, db *pgx.Conn, alias *Alias, accountID, aliasID int) error {
	return pgxscan.Get(
		ctx,
		db,
		alias,
		`
		SELECT a.* 
		FROM aliases AS a
			JOIN domains AS d ON a.domain_id = d.id
		WHERE
			a.id = $1
			AND d.account_id = $2
			AND d.deleted_at IS NULL
			AND a.deleted_at IS NULL
		`,
		aliasID,
		accountID,
	)
}

func CreateAlias(ctx context.Context, db *pgx.Conn, rule string, accountID, domainID, destinationID int) error {
	// get domain
	var domain Domain
	err := GetDomainByID(ctx, db, &domain, accountID, domainID)
	if err != nil {
		return errors.WithMessage(err, "GetDomainByID")
	}

	// get destination
	var destination Destination
	err = GetDestinationByID(ctx, db, &destination, accountID, destinationID)
	if err != nil {
		return errors.WithMessage(err, "GetDestinationByID")
	}

	// create alias
	var aliasID int
	err = db.QueryRow(
		ctx,
		`
			WITH e AS (
				INSERT INTO aliases (domain_id, rule) 
				VALUES ($1, $2) 
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

func CreateAliasDestination(ctx context.Context, db *pgx.Conn, aliasID, destinationID int) error {
	_, err := db.Exec(
		ctx,
		`
		INSERT INTO alias_destinations (alias_id, destination_id) 
			VALUES ($1, $2)
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
