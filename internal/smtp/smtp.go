package smtp

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
)

// AliasHandler checks to see if the domain is valid
// and if the domain has any aliases attached that
// match this email address
type AliasHandler func(ctx context.Context, email string) (int, error)

// On a successful relay, pass to the handler
type RelayHandler func(session *InboundSession)

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
	return s.newInboundSession(state), nil
}

type InboundSession struct {
	ctx context.Context

	// connection meta data
	State *smtp.ConnectionState

	// email
	From    string
	To      string
	Message []byte

	// account details
	AliasID int

	// internal interfaces
	aliasHandler AliasHandler
	relayHandler RelayHandler
}

// initialise a new inbound session
func (s *Server) newInboundSession(state *smtp.ConnectionState) *InboundSession {
	return &InboundSession{
		ctx:          context.TODO(),
		State:        state,
		aliasHandler: s.aliasHandler,
		relayHandler: s.relayHandler,
	}
}

func (s *InboundSession) Mail(from string, opts smtp.MailOptions) error {
	tcpAddr, ok := s.State.RemoteAddr.(*net.TCPAddr)
	if !ok {
		return errors.New("Unknown remoteAddr type")
	}

	// spf check
	result, _ := spf.CheckHostWithSender(
		tcpAddr.IP,
		s.State.Hostname,
		from,
	)

	if result == spf.Fail {
		return errors.New("Not allowed to send using this domain")
	}

	// do we want to provide dbl checks here, i.e. spamhaus?

	s.From = from
	return nil
}

func (s *InboundSession) Rcpt(to string) error {
	aliasID, err := s.aliasHandler(s.ctx, to)
	if err != nil {
		return err
	}

	s.AliasID = aliasID
	s.To = to

	return nil
}

func (s *InboundSession) Data(r io.Reader) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	s.Message = b

	s.relayHandler(s)

	return nil
}

func (s *InboundSession) Reset() {
	s.State = nil
	s.From = ""
	s.To = ""
	s.Message = []byte("")
	s.AliasID = 0
	s.aliasHandler = nil
	s.relayHandler = nil
}

func (s *InboundSession) Logout() error {
	s.Reset()
	return nil
}
