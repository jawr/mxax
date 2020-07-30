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
	"github.com/jawr/mxax/internal/logger"
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
	Via     string
	To      string
	Message bytes.Buffer

	// account details
	AccountID int
	DomainID  int
	AliasID   int

	// reference to the server
	server *Server

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
		ID:         id,
		start:      time.Now(),
		ServerName: serverName,
		State:      state,
		server:     s,
	}

	return &session, nil
}

func (s *InboundSession) String() string {
	return fmt.Sprintf("%s", s.ID)
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

	// TODO
	// do we want to make these things configurable per domain? or system wide

	if result == spf.Fail {
		// inc reject metric
		s.server.publishLogEntry(logger.Entry{
			ID:        s.ID,
			FromEmail: from,
			Etype:     logger.EntryTypeReject,
			Status:    "SPF Fail",
		})

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
	// if no domain id then just drop
	accountID, domainID, err := s.server.domainHandler(to)
	if err != nil {
		log.Printf("%s - Rcpt - To: '%s' - domainHandler error: %s", s, to, err)
		return errors.Errorf("unknown recipient (%s)", s)
	}

	// check for return path first as we might
	// have some sort of catch all
	oID, returnPath, err := s.server.returnPathHandler(to)
	if err != nil {
		log.Printf("%s - Rcpt - To: '%s' - returnPathHandler error: %s", s, to, err)
	}

	if len(returnPath) > 0 {
		log.Printf("%s - Rcpt - To: %s Found return path: %s reset id to %s", s, to, returnPath, oID)

		// overwrite to with returnPath and set returnPath flag
		s.Via = to
		to = returnPath
		s.returnPath = true
		s.ID = oID

	} else {

		// otherwise check alias
		aliasID, err := s.server.aliasHandler(to)
		if err != nil {
			log.Printf("%s - Rcpt - To: '%s' - aliasHandler error: %s", s, to, err)

			// inc reject metric
			s.server.publishLogEntry(logger.Entry{
				AccountID: accountID,
				DomainID:  domainID,
				AliasID:   aliasID,
				FromEmail: s.From,
				ViaEmail:  to,
				Etype:     logger.EntryTypeReject,
			})
			return &smtp.SMTPError{
				Code:    550,
				Message: fmt.Sprintf("unknown recipient (%s)", s),
			}
		}

		s.AliasID = aliasID
	}

	s.AccountID = accountID
	s.DomainID = domainID
	s.To = to

	log.Printf("%s - Mail - To: '%s' - AliasID: %d", s, to, s.AliasID)

	return nil
}

func (s *InboundSession) Data(r io.Reader) error {
	start := time.Now()

	n, err := s.Message.ReadFrom(r)
	if err != nil {
		log.Printf("%s - Data - ReadFrom: %s", s, err)
		return errors.Errorf("can not read message (%s)", s)
	}

	if s.returnPath {
		if err := s.server.queueEmailHandler(Email{
			ID:        s.ID,
			From:      s.From,
			Via:       s.Via,
			To:        s.To,
			Message:   s.Message.Bytes(),
			AccountID: s.AccountID,
			DomainID:  s.DomainID,
			AliasID:   s.AliasID,
			Bounce:    "Returned",
		}); err != nil {
			log.Printf("%s - Data - queueEmailHandler: %s", s, err)
			return errors.Errorf("unable to forward this message (%s)", s)
		}
	} else {
		if err := s.server.forwardHandler(s); err != nil {
			log.Printf("%s - Data - forwardHandler: %s", s, err)
			return errors.Errorf("unable to forward this message (%s)", s)
		}
	}

	log.Printf("%s - Data - read %d bytes in %s", s, n, time.Since(start))

	return nil
}

func (s *InboundSession) Reset() {
	log.Printf("%s - Reset - after %s", s, time.Since(s.start))
	s.From = ""
	s.To = ""
	s.Message.Reset()
	s.AliasID = 0
	s.returnPath = false
}

func (s *InboundSession) Logout() error {
	if len(s.From) > 0 {
		s.Reset()
	}
	log.Printf("%s - Logout", s)
	return nil
}
