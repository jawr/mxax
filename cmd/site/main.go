package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jawr/mxax/internal/site"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	server, err := site.NewSite()
	if err != nil {
		return errors.WithMessage(err, "NewSite")
	}

	listenAddr := os.Getenv("MXAX_SITE_LISTEN_ADDR")
	log.Printf("Listening on http://%s", listenAddr)
	if err := server.Run(listenAddr); err != nil {
		return errors.WithMessage(err, "Run")
	}

	return nil
}
