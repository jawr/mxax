package transactional

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

type Publisher struct {
	ch   *rabbitmq.Channel
	pool sync.Pool
}

func NewPublisher(ch *rabbitmq.Channel) *Publisher {
	p := Publisher{
		ch: ch,
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
	return &p
}

// TODO
// potentially want to wrap this with an internal channel to prevent latency on requests
func (p *Publisher) Add(email Email) error {
	b := p.pool.Get().(*bytes.Buffer)
	defer p.pool.Put(b)
	b.Reset()

	if err := json.NewEncoder(b).Encode(email); err != nil {
		return errors.WithMessage(err, "Encode")
	}

	msg := amqp.Publishing{
		Timestamp:   time.Now(),
		ContentType: "application/json",
		Body:        b.Bytes(),
	}

	err := s.logPublisher.Publish(
		"",
		"transactional",
		false, // mandatory
		false, // immediate
		msg,
	)
	if err != nil {
		return errors.WithMessage(err, "Publish")
	}

	return nil
}
