package smtp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
)

type InboundSession struct {
	ctx context.Context

	// connection meta data
	State *smtp.ConnectionState

	ServerName string

	// email
	From    string
	To      string
	Message bytes.Buffer

	// account details
	AliasID int

	// internal interfaces
	aliasHandler AliasHandler
	relayHandler RelayHandler
}

// initialise a new inbound session
func (s *Server) newInboundSession(serverName string, state *smtp.ConnectionState) *InboundSession {
	return &InboundSession{
		ctx:          context.TODO(),
		ServerName:   serverName,
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
	if _, err := s.Message.ReadFrom(r); err != nil {
		return err
	}

	if err := s.relayHandler(s); err != nil {
		return err
	}

	return nil
}

func (s *InboundSession) Reset() {
	s.State = nil
	s.From = ""
	s.To = ""
	s.Message.Reset()
	s.AliasID = 0
	s.aliasHandler = nil
	s.relayHandler = nil
}

func (s *InboundSession) Logout() error {
	s.Reset()
	return nil
}
