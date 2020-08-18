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
	tick := time.Tick(time.Minute / 1)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg := <-s.emailSubscriber:
			start := time.Now()

			email := s.emailPool.Get().(*smtp.Email)
			email.Reset()

			var err error
			var reply string

			if err := json.Unmarshal(msg.Body, email); err != nil {
				log.Printf("Failed to unmarshal msg: %s", err)
				goto END
			}

			reply, err = s.sendEmail(rdns, dialer, email)
			if err != nil {
				log.Printf("=== - %s - Bounced (%s -> %s -> %s) [%s] [%s]: %s", email.ID, email.From, email.Via, email.To, time.Since(start), reply, err)
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
				log.Printf("=== - %s - Sent (%s -> %s -> %s) [%s]: %s", email.ID, email.From, email.Via, email.To, time.Since(start), reply)
				s.publishLogEntry(logger.Entry{
					ID:            email.ID,
					AccountID:     email.AccountID,
					DomainID:      email.DomainID,
					AliasID:       email.AliasID,
					DestinationID: email.DestinationID,
					FromEmail:     email.From,
					ViaEmail:      email.Via,
					ToEmail:       email.To,
					Status:        reply,
					Etype:         logger.EntryTypeSend,
				})
			}

		END:
			// msg.Ack(false)
			s.emailPool.Put(email)

			<-tick
		}
	}

	log.Println("Shutting down Run")

	return nil
}
