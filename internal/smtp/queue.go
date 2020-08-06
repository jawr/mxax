package smtp

import (
	"bytes"
	"encoding/json"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

func (s *Server) queueEmail(email Email) error {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
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

	log.Printf("=== - %s - Queued", email.ID)

	return nil
}
