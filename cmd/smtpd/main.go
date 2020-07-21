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

	// for inbound sessions, handles the RCPT TO hook
	aliasHandler, err := smtp.MakeAliasHandler(db)
	if err != nil {
		return errors.WithMessage(err, "makeAliasHandler")
	}

	// for inbound sessions, handles the DATA hook
	relayHandler, err := smtp.MakeRelayHandler(db)
	if err != nil {
		return errors.WithMessage(err, "makeRelayHandler")
	}

	// for inbound sessions, handles return path at RCPT TO (post/at failed aliasHandler)
	returnPathHandler, err := smtp.MakeReturnPathHandler(db)
	if err != nil {
		return errors.WithMessage(err, "makeReturnPathHandler")
	}

	// messages to sent get queued here
	queueEnvelopeHandler, err := smtp.MakeQueueEnvelopeHandler()
	if err != nil {
		return errors.WithMessage(err, "makeQueueEnvelopeHandler")
	}

	// server will eventually handle inbound and outbound
	server := smtp.NewServer(
		aliasHandler,
		returnPathHandler,
		relayHandler,
		queueEnvelopeHandler,
	)

	log.Println("Starting SMTP Server")

	if err := server.Run(os.Getenv("MXAX_DOMAIN")); err != nil {
		return errors.WithMessage(err, "server.Run")
	}

	return nil
}
