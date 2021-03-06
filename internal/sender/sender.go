package sender

import (
	"bytes"
	"sync"

	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jawr/mxax/internal/cache"
	"github.com/jawr/mxax/internal/smtp"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

type Sender struct {
	wait chan struct{}

	publisher *rabbitmq.Channel

	emailSubscriber  <-chan amqp.Delivery
	bounceSubscriber <-chan amqp.Delivery

	// pools
	emailPool  sync.Pool
	bufferPool sync.Pool

	// multi purpose cache, strings are prefixed with namespace
	cache *cache.Cache
}

func NewSender(publisher *rabbitmq.Channel, emailSubscriber, bounceSubscriber <-chan amqp.Delivery) (*Sender, error) {
	cache, err := cache.NewCache()
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	sender := &Sender{
		wait:             make(chan struct{}, 0),
		publisher:        publisher,
		emailSubscriber:  emailSubscriber,
		bounceSubscriber: bounceSubscriber,
		cache:            cache,
		emailPool: sync.Pool{
			New: func() interface{} {
				return new(smtp.Email)
			},
		},
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}

	return sender, nil
}

func (s *Sender) Start() {
	close(s.wait)
}
