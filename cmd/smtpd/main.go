package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jawr/mxax/internal/smtp"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	// setup a cancel context and work out what we want to do
	// in the event of a rabbitmq failure or such
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// setup  database connection
	db, err := pgxpool.Connect(ctx, os.Getenv("MXAX_ADMIN_DB_URL"))
	if err != nil {
		return errors.WithMessage(err, "pgx.Connect")
	}
	defer db.Close()

	log.Println("Connected to the Database")

	// setup rabbitmq connection
	rabbitConn, err := rabbitmq.Dial(os.Getenv("MXAX_MQ_URL"))
	if err != nil {
		return errors.WithMessage(err, "rabbitmq.Dial")
	}
	defer rabbitConn.Close()

	// setup logs publisher
	logPublisher, err := createPublisher(rabbitConn, "logs")
	if err != nil {
		return errors.WithMessage(err, "createPublisher logs")
	}
	defer logPublisher.Close()

	// setup email publisher
	emailPublisher, err := createPublisher(rabbitConn, "")
	if err != nil {
		return errors.WithMessage(err, "createPublisher")
	}
	defer emailPublisher.Close()

	log.Println("Connected to the MQ")

	// server will eventually handle inbound and outbound
	server, err := smtp.NewServer(db, logPublisher, emailPublisher)
	if err != nil {
		return errors.WithMessage(err, "NewServer")
	}

	log.Println("Starting SMTP Server")

	if err := server.Run(os.Getenv("MXAX_DOMAIN")); err != nil {
		return errors.WithMessage(err, "server.Run")
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
