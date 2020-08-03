package smtp

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net"
	stdsmtp "net/smtp"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jawr/mxax/internal/logger"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

func (s *Server) handleEmails(emailSubscriber <-chan amqp.Delivery) error {
	// TODO
	// temp pace our deliveries, this will need much better logic
	// to handle bounces
	tick := time.Tick(time.Minute / 1)

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

		if err := s.sendEmail(email); err != nil {
			log.Printf("%s - Bounced (%s -> %s -> %s) [%s]: %s", email.ID, email.From, email.Via, email.To, time.Since(start), err)
			email.Bounce = err.Error()
		}

		if len(email.Bounce) > 0 {
			s.publishLogEntry(logger.Entry{
				ID:            email.ID,
				AccountID:     email.AccountID,
				DomainID:      email.DomainID,
				AliasID:       email.AliasID,
				DestinationID: email.DestinationID,
				FromEmail:     email.From,
				ViaEmail:      email.Via,
				ToEmail:       email.To,
				Etype:         logger.EntryTypeBounce,
				Status:        email.Bounce,
				Message:       email.Message,
			})

		} else {
			log.Printf("%s - Sent (%s -> %s -> %s) [%s]", email.ID, email.From, email.Via, email.To, time.Since(start))
			s.publishLogEntry(logger.Entry{
				ID:            email.ID,
				AccountID:     email.AccountID,
				DomainID:      email.DomainID,
				AliasID:       email.AliasID,
				DestinationID: email.DestinationID,
				FromEmail:     email.From,
				ViaEmail:      email.Via,
				ToEmail:       email.To,
				Etype:         logger.EntryTypeSend,
			})
		}

	END:
		msg.Ack(false)
		pool.Put(email)

		<-tick
	}

	log.Println("Shutting down handleEmails")

	return nil
}

func (s *Server) sendEmail(email *Email) error {
	parts := strings.Split(email.To, "@")
	if len(parts) != 2 {
		return errors.Errorf("bad destination: '%s'", email.To)
	}

	destinationMXs, err := s.getDestinationMXs(parts[1])
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

		returnPath := email.ReturnPath
		if len(returnPath) == 0 {
			returnPath = email.From
		}

		if err := client.Mail(returnPath); err != nil {
			return errors.WithMessage(err, "Mail")
		}

		if err := client.Rcpt(email.To); err != nil {
			return errors.WithMessage(err, "Rcpt")
		}

		wc, err := client.Data()
		if err != nil {
			return errors.WithMessage(err, "Data")
		}

		if _, err := wc.Write(email.Message); err != nil {
			return errors.WithMessage(err, "Write")
		}

		if err := wc.Close(); err != nil {
			return err
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
}

func (s *Server) getDestinationMXs(domain string) ([]*net.MX, error) {
	if mxs, ok := s.cacheGet("mx", domain); ok {
		return mxs.([]*net.MX), nil
	}

	mxs, err := net.LookupMX(domain)
	if err != nil {
		return nil, errors.WithMessage(err, "LookupMX")
	}

	if len(mxs) == 0 {
		return nil, errors.Errorf("Found no MX domains for %s", domain)
	}

	sort.Slice(mxs, func(i, j int) bool {
		return mxs[i].Pref < mxs[j].Pref
	})

	s.cacheSet("mx", domain, mxs)

	return mxs, nil
}
