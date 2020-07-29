package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/smtp"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
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
	db, err := pgx.Connect(ctx, os.Getenv("MXAX_DB_URL"))
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
	metricPublisher, err := createPublisher(rabbitConn, "logs")
	if err != nil {
		return errors.WithMessage(err, "createPublisher logs")
	}
	defer metricPublisher.Close()

	// setup email publisher
	emailPublisher, err := createPublisher(rabbitConn, "emails")
	if err != nil {
		return errors.WithMessage(err, "createPublisher emails")
	}
	defer emailPublisher.Close()

	// setup email subscriber
	hostname, err := os.Hostname()
	if err != nil {
		return errors.WithMessage(err, "Hostname")
	}

	emailSubscriberCh, emailSubscriber, err := createSubscriber(rabbitConn, "emails", hostname+"smtpd")
	if err != nil {
		return errors.WithMessage(err, "createSubscriber emails")
	}
	defer emailSubscriberCh.Close()

	log.Println("Connected to the MQ")

	// server will eventually handle inbound and outbound
	server, err := smtp.NewServer(db, metricPublisher, emailPublisher)
	if err != nil {
		return errors.WithMessage(err, "NewServer")
	}

	log.Println("Starting SMTP Server")

	if err := server.Run(os.Getenv("MXAX_DOMAIN"), emailSubscriber); err != nil {
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

func createSubscriber(conn *rabbitmq.Connection, queueName, name string) (*rabbitmq.Channel, <-chan amqp.Delivery, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, errors.WithMessage(err, "subscriber.Channel")
	}

	msgs, err := ch.Consume(
		queueName,
		name,
		false, // autoack
		false, // exclusive
		false, // nolocal
		false, // nowait
		nil,
	)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "ch.Consume")
	}

	return ch, msgs, nil
}
