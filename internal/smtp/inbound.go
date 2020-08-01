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
	"github.com/jawr/mxax/internal/logger"
	"github.com/pkg/errors"
)

type InboundSession struct {
	data *SessionData
}

// initialise a new inbound session
func (s *Server) newInboundSession(serverName string, state *smtp.ConnectionState) (*InboundSession, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	// try use a pool with a self reference to the server, is Logout guaranteed to be called?

	session := InboundSession{
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

func (s *InboundSession) String() string {
	return fmt.Sprintf("%s", s.data.ID)
}

func (s *InboundSession) Mail(from string, opts smtp.MailOptions) error {
	log.Printf("IB - %s - Mail - From '%s'", s, from)

	tcpAddr, ok := s.data.State.RemoteAddr.(*net.TCPAddr)
	if !ok {
		log.Printf("IB - %s - Mail - Unable to case RemoteAddr: %+v", s, s.data.State)
		return errors.Errorf("network error (%s)", s)
	}

	// spf check
	result, _ := spf.CheckHostWithSender(
		tcpAddr.IP,
		s.data.State.Hostname,
		from,
	)

	// TODO
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

func (s *InboundSession) Rcpt(to string) error {
	// if no domain id then just drop
	accountID, domainID, err := s.data.server.domainHandler(to)
	if err != nil {
		log.Printf("IB - %s - Rcpt - To: '%s' - domainHandler error: %s", s, to, err)
		return errors.Errorf("unknown recipient (%s)", s)
	}

	// check for return path first as we might
	// have some sort of catch all
	oID, returnPath, err := s.data.server.returnPathHandler(to)
	if err != nil {
		log.Printf("IB - %s - Rcpt - To: '%s' - returnPathHandler error: %s", s, to, err)
	}

	if len(returnPath) > 0 {
		log.Printf("IB - %s - Rcpt - To: %s Found return path: %s reset id to %s", s, to, returnPath, oID)

		// overwrite to with returnPath and set returnPath flag
		s.data.Via = to
		to = returnPath
		s.data.returnPath = true
		s.data.ID = oID

	} else {

		// otherwise check alias
		aliasID, err := s.data.server.aliasHandler(to)
		if err != nil {
			log.Printf("IB - %s - Rcpt - To: '%s' - aliasHandler error: %s", s, to, err)

			// inc reject metric
			s.data.server.publishLogEntry(logger.Entry{
				AccountID: accountID,
				DomainID:  domainID,
				AliasID:   aliasID,
				FromEmail: s.data.From,
				ViaEmail:  to,
				Etype:     logger.EntryTypeReject,
			})
			return &smtp.SMTPError{
				Code:    550,
				Message: fmt.Sprintf("unknown recipient (%s)", s),
			}
		}

		s.data.AliasID = aliasID
	}

	s.data.AccountID = accountID
	s.data.DomainID = domainID
	s.data.To = to

	log.Printf("IB - %s - Mail - To: '%s' - AliasID: %d", s, to, s.data.AliasID)

	return nil
}

func (s *InboundSession) Data(r io.Reader) error {
	start := time.Now()

	n, err := s.data.Message.ReadFrom(r)
	if err != nil {
		log.Printf("IB - %s - Data - ReadFrom: %s", s, err)
		return errors.Errorf("can not read message (%s)", s)
	}

	if s.data.returnPath {
		if err := s.data.server.queueEmailHandler(Email{
			ID:        s.data.ID,
			From:      s.data.From,
			Via:       s.data.Via,
			To:        s.data.To,
			Message:   s.data.Message.Bytes(),
			AccountID: s.data.AccountID,
			DomainID:  s.data.DomainID,
			AliasID:   s.data.AliasID,
			Bounce:    "Returned",
		}); err != nil {
			log.Printf("IB - %s - Data - queueEmailHandler: %s", s, err)
			return errors.Errorf("unable to forward this message (%s)", s)
		}
	} else {
		if err := s.data.server.forwardHandler(s.data); err != nil {
			log.Printf("IB - %s - Data - forwardHandler: %s", s, err)
			return errors.Errorf("unable to forward this message (%s)", s)
		}
	}

	log.Printf("IB - %s - Data - read %d bytes in %s", s, n, time.Since(start))

	return nil
}

func (s *InboundSession) Reset() {
	log.Printf("IB - %s - Reset - after %s", s, time.Since(s.data.start))
	s.data.From = ""
	s.data.To = ""
	s.data.Message.Reset()
	s.data.AliasID = 0
	s.data.returnPath = false
}

func (s *InboundSession) Logout() error {
	if len(s.data.From) > 0 {
		s.Reset()
	}
	log.Printf("IB - %s - Logout", s)
	return nil
}
