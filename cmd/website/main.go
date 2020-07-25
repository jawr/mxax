package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v4"
	"github.com/jess/mxax/internal/site"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
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

	server, err := site.NewSite(db)
	if err != nil {
		return errors.WithMessage(err, "NewSite")
	}

	listenAddr := os.Getenv("MXAX_LISTEN_ADDR")
	log.Printf("Listening on http://%s", listenAddr)
	if err := server.Run(listenAddr); err != nil {
		return errors.WithMessage(err, "Run")
	}

	return nil
}
