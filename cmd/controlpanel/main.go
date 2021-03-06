package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jawr/mxax/internal/controlpanel"
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

	db, err := pgxpool.Connect(ctx, os.Getenv("MXAX_DB_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgxpool.Connect")
	}
	defer db.Close()

	log.Println("Connected to the Accounts Database")

	adminDB, err := pgxpool.Connect(ctx, os.Getenv("MXAX_ADMIN_DB_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgxpool.Connect")
	}
	defer adminDB.Close()

	log.Println("Connected to the Admin Database")

	server, err := controlpanel.NewSite(db, adminDB)
	if err != nil {
		return errors.WithMessage(err, "NewSite")
	}

	listenAddr := os.Getenv("MXAX_CONTROLPANEL_LISTEN_ADDR")
	log.Printf("Listening on http://%s", listenAddr)
	if err := server.Run(listenAddr); err != nil {
		return errors.WithMessage(err, "Run")
	}

	return nil
}
