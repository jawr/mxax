package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jess/mxax/internal/account"
	"github.com/jess/mxax/internal/smtp"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	db, err := pgx.Connect(ctx, os.Getenv("MXAX_DATABASE_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgx.Connect")
	}
	defer db.Close(ctx)

	log.Println("DB Connected")

	server := smtp.NewServer(makeAliasHandler(db), makeRelayHandler(db))

	log.Println("Starting SMTP Server...")

	if err := server.Run(os.Getenv("MXAX_DOMAIN")); err != nil {
		return errors.WithMessage(err, "server.Run")
	}

	return nil
}

func makeAliasHandler(db *pgx.Conn) smtp.AliasHandler {
	// replace with real concurrent safe caches that allows
	// for ttl

	nxdomain := make(map[string]struct{}, 0)
	nxmatch := make(map[string]struct{}, 0)
	matches := make(map[string]int, 0)
	aliases := make(map[string][]account.Alias, 0)

	return func(ctx context.Context, email string) (int, error) {
		if _, ok := nxmatch[email]; ok {
			return 0, errors.New("No match")
		}

		if aliasID, ok := matches[email]; ok {
			return aliasID, nil
		}

		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			return 0, errors.New("Malformed email address")
		}

		user := parts[0]
		domain := parts[1]

		// check if this is a bad domain we have checked already
		if _, ok := nxdomain[domain]; ok {
			return 0, errors.New("Domain not accepted")
		}

		// search for domain in the database
		all, ok := aliases[domain]
		if !ok {
			if err := pgxscan.Select(ctx, db, &all, "SELECT a.* FROM aliases AS a JOIN domains AS d ON a.domain_id = d.id WHERE d.name = $1 AND d.deleted_at IS NULL AND d.verified_at IS NOT NULL", domain); err != nil {
				nxdomain[domain] = struct{}{}

				return 0, errors.New("Domain not accepted")
			}
			aliases[domain] = all
		}

		// check for matches
		for _, i := range all {
			ok, err := i.Check(user)
			if err != nil {
				continue
			}
			if ok {
				matches[email] = i.ID
				return i.ID, nil
			}
		}

		// no matches found, update nxmatch and return
		nxmatch[email] = struct{}{}

		return 0, errors.New("No match")
	}
}

func makeRelayHandler(db *pgx.Conn) smtp.RelayHandler {
	return func(session *smtp.InboundSession) {
		log.Printf("Success: %+v", session)
	}
}
