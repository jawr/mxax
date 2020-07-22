package smtp

import (
	"crypto/tls"
	"log"
	stdsmtp "net/smtp"
	"os"
	"strings"

	"github.com/dgraph-io/ristretto"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type Envelope struct {
	ID      uuid.UUID
	From    string
	To      string
	Message []byte
}

type QueueEnvelopeHandler func(Envelope) error

func MakeQueueEnvelopeHandler() (QueueEnvelopeHandler, error) {
	destinationMXsCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	return func(env Envelope) error {
		log.Printf("%s - Queued", env.ID)

		parts := strings.Split(env.To, "@")
		if len(parts) != 2 {
			return errors.Errorf("bad destination: '%s'", env.To)
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

			if err := client.Mail(env.From); err != nil {
				return errors.WithMessage(err, "Mail")
			}

			if err := client.Rcpt(env.To); err != nil {
				return errors.WithMessage(err, "Rcpt")
			}

			wc, err := client.Data()
			if err != nil {
				return errors.WithMessage(err, "Data")
			}

			n, err := wc.Write(env.Message)
			if err != nil {
				return errors.WithMessage(err, "Write")
			}
			log.Printf("%s - wrote %d bytes", env.ID, n)

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
