package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4"
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
	ctx := context.Background()

	db, err := pgx.Connect(ctx, os.Getenv("MXAX_ADMIN_DB_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgx.Connect")
	}
	defer db.Close(ctx)

	log.Println("Connected to the Database")

	// setup rabbitmq connection
	rabbitConn, err := rabbitmq.Dial(os.Getenv("MXAX_MQ_URL"))
	if err != nil {
		return errors.WithMessage(err, "rabbitmq.Dial")
	}
	defer rabbitConn.Close()

	// setup logs publisher
	emailPublisher, err := createPublisher(rabbitConn, "emails")
	if err != nil {
		return errors.WithMessage(err, "createPublisher emails")
	}
	defer emailPublisher.Close()

	log.Println("Connected to the MQ")

	server, err := site.NewSite(db, emailPublisher)
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

func createPublisher(conn *rabbitmq.Connection, queueName string) (*rabbitmq.Channel, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, errors.WithMessage(err, "subscriber.Channel")
	}

	if len(queueName) > 0 {
		_, err = ch.QueueDeclare(
			queueName,
			true,  // durable
			false, // autoDelete
			false, // exclusive
			false, // noWait
			nil,
		)
		if err != nil {
			return nil, errors.WithMessage(err, "QueueDeclare")
		}
	}

	return ch, nil
}
