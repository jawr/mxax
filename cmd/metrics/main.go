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
	"github.com/jawr/mxax/internal/metrics"
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

	metricsSubscriberCh, metricsSubscriber, err := createSubscriber(rabbitConn, "metrics", hostname+"metrics")
	if err != nil {
		return errors.WithMessage(err, "createSubscriber metrics")
	}
	defer metricsSubscriberCh.Close()

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
		case msg := <-metricsSubscriber:
			var w metrics.Wrapper
			if err := json.Unmarshal(msg.Body, &w); err != nil {
				// temp
				return err
			}

			switch w.Type {
			case metrics.MetricTypeInboundReject:
				var m metrics.InboundReject
				if err := json.Unmarshal(w.Data, &m); err != nil {
					return err
				}

				_, err := db.Exec(
					ctx,
					`INSERT INTO metrics__inbound_rejects (time,from_email,to_email,domain_id)
					VALUES ($1,$2,$3,$4)`,
					m.Time,
					m.FromEmail,
					m.ToEmail,
					m.DomainID,
				)
				if err != nil {
					return err
				}
			case metrics.MetricTypeInboundForward:
				var m metrics.InboundForward
				if err := json.Unmarshal(w.Data, &m); err != nil {
					return err
				}

				_, err := db.Exec(
					ctx,
					`INSERT INTO metrics__inbound_forwards (time,from_email,domain_id,alias_id,destination_id)
					VALUES ($1,$2,$3,$4,$5)`,
					m.Time,
					m.FromEmail,
					m.DomainID,
					m.AliasID,
					m.DestinationID,
				)
				if err != nil {
					return err
				}
			case metrics.MetricTypeInboundBounce:
				var m metrics.InboundBounce
				if err := json.Unmarshal(w.Data, &m); err != nil {
					return err
				}

				_, err := db.Exec(
					ctx,
					`INSERT INTO metrics__inbound_bounces (time,from_email,domain_id,alias_id,destination_id,message,reason)
					VALUES ($1,$2,$3,$4,$5,$6,$7)`,
					m.Time,
					m.FromEmail,
					m.DomainID,
					m.AliasID,
					m.DestinationID,
					m.Message,
					m.Reason,
				)
				if err != nil {
					return err
				}
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
