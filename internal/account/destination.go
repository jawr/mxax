package account

import (
	"context"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
)

// Destination represents an email address
// that an alias will send to. It's split out
// in to it's own struct so it is normalised
type Destination struct {
	ID int

	AccountID int

	Address string

	MetaData
}

func GetDestinations(ctx context.Context, db *pgx.Conn, destinations *[]Destination, accountID int) error {
	return pgxscan.Select(
		ctx,
		db,
		&destinations,
		`
			SELECT * 
			FROM destinations 
			WHERE 
				account_id = $1 
				AND deleted_at IS NULL 
			ORDER BY address
			`,
		accountID,
	)
}

func GetDestination(ctx context.Context, db *pgx.Conn, destination *Destination, accountID int, name string) error {
	return pgxscan.Get(
		ctx,
		db,
		destination,
		`
			SELECT * 
			FROM destinations 
			WHERE 
				account_id = $1 
				AND name = $2
				AND deleted_at IS NULL
			`,
		accountID,
		name,
	)
}

func GetDestinationByID(ctx context.Context, db *pgx.Conn, destination *Destination, accountID, destinationID int) error {
	return pgxscan.Get(
		ctx,
		db,
		destination,
		`
			SELECT * 
			FROM destinations 
			WHERE 
				account_id = $1 
				AND id = $2
				AND deleted_at IS NULL
			`,
		accountID,
		destinationID,
	)
}
