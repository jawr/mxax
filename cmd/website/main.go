package main

import (
	"fmt"
	"os"

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
	server, err := site.NewSite()
	if err != nil {
		return errors.WithMessage(err, "NewSite")
	}

	if err := server.Run("localhost:8888"); err != nil {
		return errors.WithMessage(err, "Run")
	}

	return nil
}
