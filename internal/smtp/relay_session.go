package smtp

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/account"
	"github.com/jawr/mxax/internal/logger"
	"github.com/pkg/errors"
)

type RelaySession struct {
	data *SessionData
}

// initialise a new inbound session
func (s *Server) newRelaySession(serverName string, state *smtp.ConnectionState) (*RelaySession, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	session := RelaySession{
		data: &SessionData{
			ID:         id,
			start:      time.Now(),
			ServerName: serverName,
			State:      state,
			server:     s,
		},
	}

	return &session, nil
}

func (s *RelaySession) String() string {
	return fmt.Sprintf("RLY - %s", s.data.ID)
}

func (s *RelaySession) Mail(from string, opts smtp.MailOptions) error {
	log.Printf("%s - Mail - From '%s'", s, from)

	tcpAddr, ok := s.data.State.RemoteAddr.(*net.TCPAddr)
	if !ok {
		log.Printf("%s - Mail - Unable to case RemoteAddr: %+v", s, s.data.State)
		return errors.Errorf("network error (%s)", s)
	}

	// spf check
	result, _ := spf.CheckHostWithSender(
		tcpAddr.IP,
		s.data.State.Hostname,
		from,
	)

	// TODO
	// use https://github.com/emersion/go-msgauth/ to check dmarc/dkim/spf
	// do we want to make these things configurable per domain? or system wide

	if result == spf.Fail {
		// inc reject metric
		s.data.server.publishLogEntry(logger.Entry{
			ID:        s.data.ID,
			FromEmail: from,
			Etype:     logger.EntryTypeReject,
			Status:    "SPF Fail",
		})

		log.Printf(
			"%s - Mail - CheckHostWithSender spf.Fail: ip: %s hostname: %s from: %s",
			s,
			tcpAddr.IP,
			s.data.State.Hostname,
			from,
		)
		return errors.Errorf("spf check failed (%s)", s)
	}

	// do we want to provide dbl checks here, i.e. spamhaus?

	s.data.From = from

	return nil
}

func (s *RelaySession) Rcpt(to string) error {
	// if no domain id then just drop
	domain, err := s.data.server.detectDomain(to)
	if err != nil {
		log.Printf("%s - Rcpt - To: '%s' - detectDomain error: %s", s, to, err)
		return errors.Errorf("unknown recipient (%s)", s)
	}

	// check for return path first as we might
	// have some sort of catch all
	oID, returnPath, err := s.data.server.detectReturnPath(to)
	if err != nil {
		log.Printf("%s - Rcpt - To: '%s' - detectReturnPath error: %s", s, to, err)
	}

	if len(returnPath) > 0 {
		log.Printf("%s - Rcpt - To: %s Found return path: %s reset id to %s", s, to, returnPath, oID)

		// overwrite to with returnPath and set returnPath flag
		s.data.Via = to
		to = returnPath
		s.data.returnPath = true
		s.data.ID = oID

	} else {

		// otherwise check alias
		alias, err := s.data.server.detectAlias(to)
		if err != nil {
			log.Printf("%s - Rcpt - To: '%s' - detectAlias error: %s", s, to, err)

			// inc reject metric
			s.data.server.publishLogEntry(logger.Entry{
				AccountID: domain.AccountID,
				DomainID:  domain.ID,
				AliasID:   alias.ID,
				FromEmail: s.data.From,
				ViaEmail:  to,
				Etype:     logger.EntryTypeReject,
			})
			return &smtp.SMTPError{
				Code:    550,
				Message: fmt.Sprintf("unknown recipient (%s)", s),
			}
		}

		s.data.Alias = alias
	}

	s.data.Domain = domain
	s.data.To = to

	log.Printf(
		"%s - Mail - To: '%s' - AccountID: %d DomainID: %d AliasID: %d",
		s,
		to,
		s.data.Domain.AccountID,
		s.data.Domain.ID,
		s.data.Alias.ID,
	)

	return nil
}

func (s *RelaySession) Data(r io.Reader) error {
	start := time.Now()

	n, err := s.data.Message.ReadFrom(r)
	if err != nil {
		log.Printf("%s - Data - ReadFrom: %s", s, err)
		return errors.Errorf("can not read message (%s)", s)
	}

	log.Printf("%s - Data - read %d bytes in %s", s, n, time.Since(start))

	if s.data.returnPath {
		if err := s.data.server.queueEmail(Email{
			ID:        s.data.ID,
			From:      s.data.From,
			Via:       s.data.Via,
			To:        s.data.To,
			Message:   s.data.Message.Bytes(),
			AccountID: s.data.Domain.AccountID,
			DomainID:  s.data.Domain.ID,
			AliasID:   s.data.Alias.ID,
			Bounce:    "Returned",
		}); err != nil {
			log.Printf("%s - Data - queueEmail: %s", s, err)
			return errors.Errorf("unable to relay this message (%s)", s)
		}

	} else {
		if err := s.data.server.relay(s.data); err != nil {
			log.Printf("%s - Data - relay: %s", s, err)
			return errors.Errorf("unable to relay this message (%s)", s)
		}
	}

	return nil
}

func (s *RelaySession) Reset() {
	log.Printf("%s - Reset - after %s", s, time.Since(s.data.start))
	s.data.From = ""
	s.data.To = ""
	s.data.Message.Reset()
	s.data.Alias = account.Alias{}
	s.data.Domain = account.Domain{}
	s.data.returnPath = false
}

func (s *RelaySession) Logout() error {
	if len(s.data.From) > 0 {
		s.Reset()
	}
	log.Printf("%s - Logout", s)
	return nil
}
