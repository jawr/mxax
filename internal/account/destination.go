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

func GetDestinations(ctx context.Context, db pgx.Tx, destinations []Destination) error {
	return pgxscan.Select(
		ctx,
		db,
		&destinations,
		`
			SELECT * 
			FROM destinations 
			WHERE deleted_at IS NULL
			ORDER BY address
			`,
	)
}

func GetDestination(ctx context.Context, db pgx.Tx, destination *Destination, name string) error {
	return pgxscan.Get(
		ctx,
		db,
		destination,
		`
			SELECT * 
			FROM destinations 
			WHERE 
				name = $1
				AND deleted_at IS NULL
			`,
		name,
	)
}

func GetDestinationByID(ctx context.Context, db pgx.Tx, destination *Destination, destinationID int) error {
	return pgxscan.Get(
		ctx,
		db,
		destination,
		`
			SELECT * 
			FROM destinations 
			WHERE 
				id = $1
				AND deleted_at IS NULL
			`,
		destinationID,
	)
}
