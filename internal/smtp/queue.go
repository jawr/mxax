package smtp

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"log"
	stdsmtp "net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

type queueEmailHandlerFn func(Email) error

func (s *Server) makeQueueEmailHandler(db *pgx.Conn) (queueEmailHandlerFn, error) {
	pool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	return func(email Email) error {
		b := pool.Get().(*bytes.Buffer)
		defer pool.Put(b)
		b.Reset()

		if err := json.NewEncoder(b).Encode(&email); err != nil {
			return errors.WithMessage(err, "Encode")
		}

		msg := amqp.Publishing{
			Timestamp:   time.Now(),
			ContentType: "application/json",
			Body:        b.Bytes(),
		}

		err := s.emailPublisher.Publish(
			"",
			"emails",
			false, // mandatory
			false, // immediate
			msg,
		)
		if err != nil {
			return errors.WithMessage(err, "Publish")
		}

		log.Printf("%s - Queued", email.ID)

		return nil
	}, nil
}

func (s *Server) handleEmails(emailSubscriber <-chan amqp.Delivery) error {
	// TODO
	// temp pace our deliveries, this will need much better logic
	// to handle bounces
	tick := time.Tick(time.Minute / 1)

	// make our actual sender
	sendEmail, err := s.makeSendEmail()
	if err != nil {
		return err
	}

	pool := sync.Pool{
		New: func() interface{} { return new(Email) },
	}

	for msg := range emailSubscriber {
		start := time.Now()

		email := pool.Get().(*Email)
		email.Reset()

		if err := json.Unmarshal(msg.Body, email); err != nil {
			log.Printf("Failed to unmarshal msg: %s", err)
			goto END
		}

		if err := sendEmail(email); err != nil {
			log.Printf("Failed to send %s (%s -> %s) [%s]: %s", email.ID, email.From, email.To, time.Since(start))
		} else {
			log.Printf("Sent %s (%s -> %s) [%s]", email.ID, email.From, email.To, time.Since(start))
		}

	END:
		msg.Ack(false)
		pool.Put(email)

		<-tick
	}

	log.Println("Shutting down handleEmails")

	return nil
}

type sendEmailFn func(*Email) error

func (s *Server) makeSendEmail() (sendEmailFn, error) {
	destinationMXsCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	return func(email *Email) error {
		parts := strings.Split(email.To, "@")
		if len(parts) != 2 {
			return errors.Errorf("bad destination: '%s'", email.To)
		}

		destinationMXs, err := getDestinationMXs(destinationMXsCache, parts[1])
		if err != nil {
			return errors.WithMessagef(err, "getDestinationMXs for '%s'", parts[1])
		}

		// TODO
		// try until we hit an mx successfully
		var dialErr error
		for _, mx := range destinationMXs {
			// reset err, if we hit a dial error, iterate to the next
			dialErr = nil
			client, dialErr := stdsmtp.Dial(mx.Host + ":25")
			if dialErr != nil {
				dialErr = errors.WithMessagef(err, "dial %s'", mx.Host)
				continue
			}

			if err := client.Hello(os.Getenv("MXAX_DOMAIN")); err != nil {
				return errors.WithMessage(err, "Hello")
			}

			tlsConfig := &tls.Config{
				ServerName: mx.Host,
			}

			if ok, _ := client.Extension("STARTTLS"); ok {
				if err := client.StartTLS(tlsConfig); err != nil {
					return errors.WithMessage(err, "StartTLS")
				}
			}

			if err := client.Mail(email.From); err != nil {
				return errors.WithMessage(err, "Mail")
			}

			if err := client.Rcpt(email.To); err != nil {
				return errors.WithMessage(err, "Rcpt")
			}

			wc, err := client.Data()
			if err != nil {
				return errors.WithMessage(err, "Data")
			}

			n, err := wc.Write(email.Message)
			if err != nil {
				return errors.WithMessage(err, "Write")
			}
			log.Printf("%s - wrote %d bytes", email.ID, n)

			if err := wc.Close(); err != nil {
				return errors.WithMessage(err, "Close")
			}

			if err := client.Quit(); err != nil {
				return errors.WithMessage(err, "Quit")
			}

			break
		}

		// check for any dial errors
		if dialErr != nil {
			return err
		}

		return nil
	}, nil
}
