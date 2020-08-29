package sender

import (
	"bytes"
	"encoding/json"
	"log"
	"time"

	"github.com/jawr/mxax/internal/smtp"
	"github.com/streadway/amqp"
)

func (s *Sender) publishBounce(email *smtp.Email) {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
	b.Reset()

	if err := json.NewEncoder(b).Encode(email); err != nil {
		log.Printf("Error publish email encode: %s", err)
		return
	}

	msg := amqp.Publishing{
		Timestamp:   time.Now(),
		ContentType: "application/json",
		Body:        b.Bytes(),
	}

	err := s.publisher.Publish(
		"",
		"bounces",
		false, // mandatory
		false, // immediate
		msg,
	)

	if err != nil {
		log.Printf("Error publish bounce: %s", err)
		return
	}
}
