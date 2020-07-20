package smtp

import (
	"context"
	"os"

	"github.com/emersion/go-smtp"
)

// AliasHandler checks to see if the domain is valid
// and if the domain has any aliases attached that
// match this email address
type AliasHandler func(ctx context.Context, email string) (int, error)

// On a successful relay, pass to the handler
type RelayHandler func(session *InboundSession) error

// Server will listen for smtp connections
// and check them against various rules in the
// database. Expected to have a load balancer
// in front, i.e. HaProxy
type Server struct {
	aliasHandler AliasHandler
	relayHandler RelayHandler

	s *smtp.Server
}

// Create a new Server, currently only handles inbound
// connections
func NewServer(aliasHandler AliasHandler, relayHandler RelayHandler) *Server {
	server := &Server{
		aliasHandler: aliasHandler,
		relayHandler: relayHandler,
	}

	server.s = smtp.NewServer(server)
	server.s.Debug = os.Stdout

	return server
}

func (s *Server) Run(domain string) error {
	s.s.Domain = domain
	return s.s.ListenAndServe()
}

func (s *Server) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	// return an OutboundSession
	return nil, smtp.ErrAuthUnsupported
}

func (s *Server) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return s.newInboundSession(s.s.Domain, state), nil
}
