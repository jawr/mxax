package smtp

import (
	"github.com/streadway/amqp"
)

func (s *Server) Run(domain string, emailSubscriber <-chan amqp.Delivery) error {
	// TODO
	// add cancellation

	s.submission.Domain = domain
	s.relay.Domain = domain

	errCh := make(chan error, 0)

	go func() {
		errCh <- s.relay.ListenAndServe()
	}()

	go func() {
		errCh <- s.submission.ListenAndServe()
	}()

	go func() {
		errCh <- s.handleEmails(emailSubscriber)
	}()

	return <-errCh
}
