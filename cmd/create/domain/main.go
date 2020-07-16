package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v4"
	"github.com/jess/mxax/internal/account"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	var accountID int
	var domain string

	flag.IntVar(&accountID, "accountID", 0, "Account ID to attach Domain to")
	flag.StringVar(&domain, "domain", "", "Domain to create")

	flag.Parse()

	if accountID == 0 {
		return errors.New("No Account ID specified")
	}

	if len(domain) == 0 {
		return errors.New("No Domain specified")
	}

	ctx := context.Background()

	db, err := pgx.Connect(ctx, os.Getenv("MXAX_DATABASE_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgx.Connect")
	}
	defer db.Close(ctx)

	// try insert domain
	var domainID int
	err = db.QueryRow(
		ctx,
		`INSERT INTO domains (account_id, name) VALUES ($1, $2) RETURNING id`,
		accountID,
		domain,
	).Scan(&domainID)
	if err != nil {
		return errors.WithMessage(err, "Inserting domain")
	}

	// create a new dkim key
	dkimKey, err := account.NewDkimKey(domainID)
	if err != nil {
		return errors.WithMessage(err, "account.NewDkimKey")
	}

	_, err = db.Exec(
		ctx,
		"INSERT INTO dkim_keys (domain_id, private_key, public_key) VALUES ($1, $2, $3)",
		dkimKey.DomainID,
		dkimKey.PrivateKey,
		dkimKey.PublicKey,
	)
	if err != nil {
		return errors.WithMessage(err, "Insert DkimKey")
	}

	return nil
}
