package smtp

func (s *Server) Run(domain string) error {
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

	return <-errCh
}
