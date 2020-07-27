package smtp

import (
	"bytes"
	"encoding/json"
	"log"
	"time"

	"github.com/streadway/amqp"
)

func (s *Server) publishMetric(metric interface{}) {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
	b.Reset()

	if err := json.NewEncoder(b).Encode(metric); err != nil {
		log.Printf("Error publish metric encode: %s", err)
		return
	}

	msg := amqp.Publishing{
		Timestamp:   time.Now(),
		ContentType: "application/json",
		Body:        b.Bytes(),
	}

	err := s.metricPublisher.Publish(
		"",
		"metrics",
		false, // mandatory
		false, // immediate
		msg,
	)
	if err != nil {
		log.Printf("Error publish metric: %s", err)
		return
	}
}
