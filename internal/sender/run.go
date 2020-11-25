package sender

import (
	"context"
	"encoding/json"
	"fmt"
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

	printf := func(format string, args ...interface{}) {
		format = fmt.Sprintf("%s :: %s", rdns, format)
		log.Printf(format, args...)
	}

	printf("Start")

	// TODO
	// temp pace our deliveries, this will need much better logic
	// to handle bounces
	tick := time.Tick(time.Minute)

	// reused variables that allow us to goto
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg := <-s.emailSubscriber:
			start := time.Now()

			email := s.emailPool.Get().(*smtp.Email)
			email.Reset()

			if err := json.Unmarshal(msg.Body, email); err != nil {
				printf("Failed to unmarshal msg: %s", err)
				// probably better to deadletter this
				msg.Ack(false)
				continue
			}

			printf(
				"TRY :: %s (%s -> %s -> %s)",
				email.ID,
				email.From,
				email.Via,
				email.To,
			)

			email.Status, email.Error = s.sendEmail(rdns, dialer, email)
			if email.Error != nil {
				email.Status = email.Error.Error()
				email.Etype = logger.EntryTypeBounce

				s.publishBounce(email)
			}

			printf(
				"%s :: %s (%s -> %s -> %s) [%s] [status: %s] [bounce: %s]",
				email.Etype.String(),
				email.ID,
				email.From,
				email.Via,
				email.To,
				time.Since(start),
				email.Status,
				email.Bounce,
			)

			entry := logger.Entry{
				ID:            email.ID,
				AccountID:     email.AccountID,
				DomainID:      email.DomainID,
				AliasID:       email.AliasID,
				DestinationID: email.DestinationID,
				FromEmail:     email.From,
				ViaEmail:      email.Via,
				ToEmail:       email.To,
				Status:        email.Status,
				Etype:         email.Etype,
				QueueLevel:    int(email.QueueLevel),
			}

			if entry.Etype != logger.EntryTypeSend {
				entry.Message = email.Message
			}

			s.publishLogEntry(entry)

			if err := msg.Ack(false); err != nil {
				printf("ERR :: %s :: ACK ERROR: %s", email.ID, err)
			}

			s.emailPool.Put(email)

			select {
			case <-ctx.Done():
				return ctx.Err()

			case <-tick:
			}
		}
	}

	return nil
}
