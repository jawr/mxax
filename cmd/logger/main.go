package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4"
	"github.com/jawr/mxax/internal/logger"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
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

	hostname, err := os.Hostname()
	if err != nil {
		return errors.WithMessage(err, "Hostname")
	}

	logsSubscriberCh, logsSubscriber, err := createSubscriber(rabbitConn, "logs", hostname+"logs")
	if err != nil {
		return errors.WithMessage(err, "createSubscriber logs")
	}
	defer logsSubscriberCh.Close()

	log.Println("Connected to the MQ")

	// listen for interrupt
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	for {
		select {
		case <-ctx.Done():
			return errors.New("done")
		case <-quit:
			return nil
		case msg := <-logsSubscriber:
			var e logger.Entry
			if err := json.Unmarshal(msg.Body, &e); err != nil {
				return err
			}

			_, err := db.Exec(
				ctx,
				`
				INSERT INTO logs 
					(
						time,
						id,
						domain_id,
						from_email,
						via_email,
						to_email,
						etype,
						status,
						message
					)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
				e.Time,
				e.ID,
				e.DomainID,
				e.FromEmail,
				e.ViaEmail,
				e.ToEmail,
				e.Etype,
				e.Status,
				e.Message,
			)
			if err != nil {
				return err
			}

			msg.Ack(false)
		}
	}

	return nil
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