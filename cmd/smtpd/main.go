package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v4"
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

	log.Println("Connected to the Database")

	aliasHandler, err := makeAliasHandler(db)
	if err != nil {
		return errors.WithMessage(err, "makeAliasHandler")
	}

	relayHandler, err := makeRelayHandler(db)
	if err != nil {
		return errors.WithMessage(err, "makeRelayHandler")
	}

	server := smtp.NewServer(aliasHandler, relayHandler)

	log.Println("Starting SMTP Server")

	if err := server.Run(os.Getenv("MXAX_DOMAIN")); err != nil {
		return errors.WithMessage(err, "server.Run")
	}

	return nil
}
