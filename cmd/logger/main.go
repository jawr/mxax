package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jawr/mxax/internal/account"
	"github.com/jawr/mxax/internal/cache"
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
	db, err := pgxpool.Connect(ctx, os.Getenv("MXAX_ADMIN_DB_URL"))
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

	cache, err := cache.NewCache()
	if err != nil {
		return errors.WithMessage(err, "NewCache")
	}

	var logLevel account.LogLevel

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

			item, ok := cache.Get("loglevel", fmt.Sprintf("%s", e.AccountID))
			if ok {
				logLevel = *item.(*account.LogLevel)

			} else {
				err := db.QueryRow(
					ctx,
					"SELECT log_level FROM accounts WHERE id = $1",
					e.AccountID,
				).Scan(&logLevel)
				if err != nil {
					return err
				}

				cache.Set("loglevel", fmt.Sprintf("%s", e.AccountID), &logLevel)
			}

			log.Printf("LOGGER RECV %+v", e)

			// depending on log level decide on logging

			if logLevel == account.LogLevelNone {
				msg.Ack(false)
				continue
			}

			if logLevel == account.LogLevelBounce && e.Etype != logger.EntryTypeBounce {
				msg.Ack(false)
				continue
			}

			if logLevel == account.LogLevelReject && e.Etype != logger.EntryTypeReject {
				msg.Ack(false)
				continue
			}

			if logLevel == account.LogLevelBounceAndReject && (e.Etype == logger.EntryTypeSend) {
				msg.Ack(false)
				continue
			}

			_, err := db.Exec(
				ctx,
				`
				INSERT INTO logs 
					(
						time,
						id,
						account_id,
						domain_id,
						alias_id,
						destination_id,
						from_email,
						via_email,
						to_email,
						etype,
						status,
						message,
						queue_level
					)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
				e.Time,
				e.ID,
				e.AccountID,
				e.DomainID,
				e.AliasID,
				e.DestinationID,
				e.FromEmail,
				e.ViaEmail,
				e.ToEmail,
				e.Etype,
				e.Status,
				e.Message,
				e.QueueLevel,
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

	if err := ch.Qos(1, 0, false); err != nil {
		return nil, nil, errors.WithMessage(err, "Qos")
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
