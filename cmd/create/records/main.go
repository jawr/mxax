package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/account"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	var domainName string

	flag.StringVar(&domainName, "domain", "", "Domain to create")

	flag.Parse()

	if len(domainName) == 0 {
		return errors.New("No Domain specified")
	}

	ctx := context.Background()

	db, err := pgx.Connect(ctx, os.Getenv("MXAX_DATABASE_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgx.Connect")
	}
	defer db.Close(ctx)

	// get domain
	var domain account.Domain
	if err := pgxscan.Get(ctx, db, &domain, "SELECT * FROM domains WHERE name = $1", domainName); err != nil {
		return errors.WithMessage(err, "Get domain")
	}

	var dkimKey account.DkimKey
	if err := pgxscan.Get(ctx, db, &dkimKey, "SELECT * FROM dkim_keys WHERE domain_id = $1", domain.ID); err != nil {
		return errors.WithMessage(err, "Get dkimKey")
	}

	fmt.Fprintln(os.Stdout, "@ 3600 IN MX 10 mx.pageup.uk.")
	fmt.Fprintln(os.Stdout, `@ 3600 IN TXT "v=spf1 include:spf.pageup.uk -all"`)
	fmt.Fprintf(os.Stdout, "default._domainkey 3600 IN TXT %s\n", dkimKey)

	return nil
}
