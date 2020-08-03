package smtp

import (
	"github.com/streadway/amqp"
)

func (s *Server) Run(domain string, emailSubscriber <-chan amqp.Delivery) error {
	// TODO
	// add cancellation

	s.relayServer.Domain = domain
	s.submissionServer.Domain = domain

	errCh := make(chan error, 0)

	go func() {
		errCh <- s.relayServer.ListenAndServe()
	}()

	go func() {
		errCh <- s.submissionServer.ListenAndServe()
	}()

	go func() {
		errCh <- s.handleEmails(emailSubscriber)
	}()

	return <-errCh
}
