package smtp

import (
	"bytes"
	"encoding/json"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

type QueueLevel int

const (
	QueueLevelStraw = iota
	QueueLevelSticks
	QueueLevelBricks
)

func (l QueueLevel) String() string {
	switch l {
	case QueueLevelStraw:
		return "emails.straw"
	case QueueLevelSticks:
		return "emails.sticks"
	case QueueLevelBricks:
		return "emails.bricks"
	default:
		return "emails.failover"
	}
}

var Queues = map[string]QueueLevel{
	"emails.straw":  QueueLevelStraw,
	"emails.sticks": QueueLevelSticks,
	"emails.bricks": QueueLevelBricks,
}

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
		email.QueueLevel.String(),
		false, // mandatory
		false, // immediate
		msg,
	)
	if err != nil {
		return errors.WithMessage(err, "Publish")
	}

	log.Printf("=== - %s - Queued to %s", email.ID, email.QueueLevel.String())

	return nil
}
