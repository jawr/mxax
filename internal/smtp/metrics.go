package smtp

import (
	"bytes"
	"encoding/json"
	"log"
	"time"

	"github.com/jawr/mxax/internal/logger"
	"github.com/streadway/amqp"
)

func (s *Server) publishLogEntry(entry logger.Entry) {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
	b.Reset()

	entry.Time = time.Now()

	if err := json.NewEncoder(b).Encode(entry); err != nil {
		log.Printf("Error publish entry encode: %s", err)
		return
	}

	msg := amqp.Publishing{
		Timestamp:   time.Now(),
		ContentType: "application/json",
		Body:        b.Bytes(),
	}

	err := s.logPublisher.Publish(
		"",
		"logs",
		false, // mandatory
		false, // immediate
		msg,
	)
	if err != nil {
		log.Printf("Error publish entry: %s", err)
		return
	}
}
