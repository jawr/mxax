package smtp

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type InboundSession struct {
	start time.Time

	ID uuid.UUID

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
	aliasHandler      AliasHandler
	relayHandler      RelayHandler
	returnPathHandler ReturnPathHandler

	// internal flags
	returnPath bool
}

// initialise a new inbound session
func (s *Server) newInboundSession(serverName string, state *smtp.ConnectionState) (*InboundSession, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	// try use a pool with a self reference to the server, is Logout guaranteed to be called?

	session := InboundSession{
		ID:                id,
		start:             time.Now(),
		ServerName:        serverName,
		State:             state,
		aliasHandler:      s.aliasHandler,
		relayHandler:      s.relayHandler,
		returnPathHandler: s.returnPathHandler,
	}

	return &session, nil
}

func (s *InboundSession) String() string {
	return fmt.Sprintf("is-%s", s.ID)
}

func (s *InboundSession) Mail(from string, opts smtp.MailOptions) error {
	log.Printf("%s - Mail - From '%s'", s, from)

	tcpAddr, ok := s.State.RemoteAddr.(*net.TCPAddr)
	if !ok {
		log.Printf("%s - Mail - Unable to case RemoteAddr: %+v", s, s.State)
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
			s,
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

	aliasID, err := s.aliasHandler(to)
	if err != nil {

		// check and see if we are a bounce relay
		returnPath, err := s.returnPathHandler(to)
		if err != nil {
			log.Printf("%s - Rcpt - To: '%s' - returnPathHandler error: %s", s, to, err)
			return errors.Errorf("unknown recipient (%s)", s)
		}

		if len(returnPath) == 0 {
			// no returnPath address found, return aliasHandler error
			log.Printf("%s - Rcpt - To: '%s' - aliasHandler error: %s", s, to, err)
			return errors.Errorf("unknown recipient (%s)", s)
		}

		// overwrite to with returnPath and set returnPath flag
		to = returnPath
		s.returnPath = true
	}

	s.AliasID = aliasID
	s.To = to

	log.Printf("%s - Mail - To: '%s' - AliasID: %d", s, to, aliasID)

	return nil
}

func (s *InboundSession) Data(r io.Reader) error {
	start := time.Now()

	n, err := s.Message.ReadFrom(r)
	if err != nil {
		log.Printf("%s - Data - ReadFrom: %s", s, err)
		return errors.Errorf("can not read message (%s)", s)
	}

	if err := s.relayHandler(s); err != nil {
		log.Printf("%s - Data - relayHandler: %s", s, err)
		return errors.Errorf("unable to relay this message (%s)", s)
	}

	log.Printf("%s - Data - read %d bytes in %s", s, n, time.Since(start))

	return nil
}

func (s *InboundSession) Reset() {
	log.Printf("%s - Reset - after %s", s, time.Since(s.start))
	s.State = nil
	s.From = ""
	s.To = ""
	s.Message.Reset()
	s.AliasID = 0
	s.aliasHandler = nil
	s.relayHandler = nil
	s.returnPathHandler = nil
	s.start = time.Time{}
	s.returnPath = false
}

func (s *InboundSession) Logout() error {
	if s.State != nil {
		s.Reset()
	}
	log.Printf("%s - Logout", s)
	return nil
}
