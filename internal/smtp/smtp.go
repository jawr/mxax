package smtp

import (
	"os"

	"github.com/emersion/go-smtp"
)

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
	return s.newInboundSession(s.s.Domain, state)
}
