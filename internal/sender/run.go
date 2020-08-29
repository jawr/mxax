package sender

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/jawr/mxax/internal/logger"
	"github.com/jawr/mxax/internal/smtp"
)

func (s *Sender) Run(ctx context.Context, dialer net.Dialer, rdns string) error {

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.wait:
	}

	// TODO
	// temp pace our deliveries, this will need much better logic
	// to handle bounces
	tick := time.Tick(time.Minute)

	// reused variables that allow us to goto
	for {
		select {
		case <-ctx.Done():
			log.Printf("context done, closing: %s", ctx.Err())
			return ctx.Err()

		case msg := <-s.emailSubscriber:
			log.Println("=== - msg")

			start := time.Now()

			email := s.emailPool.Get().(*smtp.Email)
			email.Reset()

			if err := json.Unmarshal(msg.Body, email); err != nil {
				log.Printf("Failed to unmarshal msg: %s", err)
				goto END
			}

			log.Printf(
				"=== - %s - Unmarshalled %s -> %s -> %s",
				email.ID,
				email.From,
				email.Via,
				email.To,
			)

			email.Status, email.Error = s.sendEmail(rdns, dialer, email)
			if email.Error != nil {
				email.Bounce = email.Error.Error()
				email.Etype = logger.EntryTypeBounce

				s.publishBounce(email)
			}

			log.Printf(
				"=== - %s - %s (%s -> %s -> %s) [%s] [status: %s] [bounce: %s]",
				email.Etype.String(),
				email.ID,
				email.From,
				email.Via,
				email.To,
				time.Since(start),
				email.Status,
				email.Bounce,
			)

		END:

			log.Printf("=== - DBG - %s - pre s.publishLogEntry", email.ID)

			s.publishLogEntry(logger.Entry{
				ID:            email.ID,
				AccountID:     email.AccountID,
				DomainID:      email.DomainID,
				AliasID:       email.AliasID,
				DestinationID: email.DestinationID,
				FromEmail:     email.From,
				ViaEmail:      email.Via,
				ToEmail:       email.To,
				Status:        email.Status,
				Bounce:        email.Bounce,
				Etype:         email.Etype,
				QueueLevel:    int(email.QueueLevel),
			})

			log.Printf("=== - DBG - %s - post s.publishLogEntry", email.ID)

			if err := msg.Ack(false); err != nil {
				log.Printf("=== - DBG - %s - ACK ERROR: %s", email.ID, err)
			}

			log.Println("=== - DBG - ack done")

			s.emailPool.Put(email)

			log.Println("=== - DBG - pool put done")

			select {
			case <-ctx.Done():
				log.Printf("inner ctx done: %s", ctx.Err())
				return ctx.Err()

			case <-tick:
			}

			log.Println("=== - DBG - tick done")
		}
	}

	log.Println("Shutting down Run")

	return nil
}
