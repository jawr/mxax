package smtp

import (
	"bytes"
	"encoding/json"
	"log"
	"time"

	"github.com/jawr/mxax/internal/metrics"
	"github.com/streadway/amqp"
)

func (s *Server) publishMetric(metric metrics.Metric) {
	b := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b)
	b.Reset()

	if err := json.NewEncoder(b).Encode(metric); err != nil {
		log.Printf("Error publish metric encode: %s", err)
		return
	}

	b2 := s.bufferPool.Get().(*bytes.Buffer)
	defer s.bufferPool.Put(b2)
	b2.Reset()

	wrapper := metrics.Wrapper{
		Type: metric.Type(),
		Data: b.Bytes(),
	}

	if err := json.NewEncoder(b2).Encode(wrapper); err != nil {
		log.Printf("Error publish metric wrapper encode: %s", err)
		return
	}

	msg := amqp.Publishing{
		Timestamp:   time.Now(),
		ContentType: "application/json",
		Body:        b2.Bytes(),
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
