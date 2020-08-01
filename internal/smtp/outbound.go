package smtp

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/logger"
	"github.com/pkg/errors"
)

type OutboundSession struct {
	data *SessionData
}

func (s *Server) newOutboundSession(serverName string, state *smtp.ConnectionState) (*OutboundSession, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	session := OutboundSession{
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

func (s *OutboundSession) String() string {
	return fmt.Sprintf("%s", s.data.ID)
}

func (s *OutboundSession) Mail(from string, opts smtp.MailOptions) error {
	// if no domain id then just drop
	accountID, domainID, err := s.data.server.domainHandler(from)
	if err != nil {
		log.Printf("OB - %s - Rcpt - From: '%s' - domainHandler error: %s", s, from, err)
		return errors.Errorf("unknown recipient (%s)", s)
	}

	aliasID, err := s.data.server.aliasHandler(from)
	if err != nil {
		log.Printf("OB - %s - Rcpt - From: '%s' - aliasHandler error: %s", s, from, err)

		// inc reject metric
		s.data.server.publishLogEntry(logger.Entry{
			AccountID: accountID,
			DomainID:  domainID,
			AliasID:   aliasID,
			FromEmail: from,
			Etype:     logger.EntryTypeReject,
		})
		return &smtp.SMTPError{
			Code:    550,
			Message: fmt.Sprintf("unknown sender (%s)", s),
		}
	}

	s.data.AliasID = aliasID
	s.data.AccountID = accountID
	s.data.DomainID = domainID
	s.data.From = from

	log.Printf("OB - %s - Mail - From: '%s' - AliasID: %d", s, from, s.data.AliasID)

	return nil
}

func (s *OutboundSession) Rcpt(to string) error {
	log.Printf("OB - %s - Mail - To '%s'", s, to)

	s.data.To = to

	return nil
}

func (s *OutboundSession) Data(r io.Reader) error {
	start := time.Now()

	n, err := s.data.Message.ReadFrom(r)
	if err != nil {
		log.Printf("OB - %s - Data - ReadFrom: %s", s, err)
		return errors.Errorf("can not read message (%s)", s)
	}

	if err := s.data.server.forwardHandler(s.data); err != nil {
		log.Printf("OB - %s - Data - forwardHandler: %s", s, err)
		return errors.Errorf("unable to forward this message (%s)", s)
	}

	log.Printf("OB - %s - Data - read %d bytes in %s", s, n, time.Since(start))

	return nil
}

func (s *OutboundSession) Reset() {
	log.Printf("OB - %s - Reset - after %s", s, time.Since(s.data.start))
	s.data.From = ""
	s.data.To = ""
	s.data.Message.Reset()
	s.data.AliasID = 0
	s.data.AccountID = 0
	s.data.DomainID = 0
}

func (s *OutboundSession) Logout() error {
	if len(s.data.From) > 0 {
		s.Reset()
	}
	log.Printf("OB - %s - Logout", s)
	return nil
}
