package smtp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type InboundSession struct {
	ctx context.Context

	id uuid.UUID

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
func (s *Server) newInboundSession(serverName string, state *smtp.ConnectionState) (*InboundSession, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	// try use a pool with a self reference to the server, is Logout guaranteed to be called?

	session := InboundSession{
		id:           id,
		ctx:          context.TODO(),
		ServerName:   serverName,
		State:        state,
		aliasHandler: s.aliasHandler,
		relayHandler: s.relayHandler,
	}

	return &session, nil
}

func (s *InboundSession) String() string {
	return fmt.Sprintf("is-%s", s.id)
}

func (s *InboundSession) Mail(from string, opts smtp.MailOptions) error {
	tcpAddr, ok := s.State.RemoteAddr.(*net.TCPAddr)
	if !ok {
		log.Printf("%s - Mail - Unable to case RemoteAddr: %+v", s.State.RemoteAddr)
		return errors.Errorf("network error (%s)", s)
	}

	// spf check
	result, _ := spf.CheckHostWithSender(
		tcpAddr.IP,
		s.State.Hostname,
		from,
	)

	if result == spf.Fail {
		log.Printf(
			"%s - Mail - CheckHostWithSender spf.Fail: ip: %s hostname: %s from: %s",
			tcpAddr.IP,
			s.State.Hostname,
			from,
		)
		return errors.Errorf("spf check failed (%s)", s)
	}

	// do we want to provide dbl checks here, i.e. spamhaus?

	s.From = from

	return nil
}

func (s *InboundSession) Rcpt(to string) error {
	aliasID, err := s.aliasHandler(s.ctx, to)
	if err != nil {
		log.Printf("%s - Rcpt - %s", err)
		return errors.Errorf("unknown recipient (%s)", s)
	}

	s.AliasID = aliasID
	s.To = to

	return nil
}

func (s *InboundSession) Data(r io.Reader) error {
	if _, err := s.Message.ReadFrom(r); err != nil {
		log.Printf("%s - Data - ReadFrom: %s", err)
		return errors.Errorf("can not read message (%s)", s)
	}

	if err := s.relayHandler(s); err != nil {
		log.Printf("%s - Data - relayHandler: %s", err)
		return errors.Errorf("unable to relay this message (%s)", s)
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
