package smtp

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/account"
	"github.com/pkg/errors"
)

type SubmissionSession struct {
	data *SessionData
}

func (s *Server) newSubmissionSession(serverName string, state *smtp.ConnectionState) (*SubmissionSession, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	session := SubmissionSession{
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

func (s *SubmissionSession) String() string {
	return fmt.Sprintf("%s", s.data.ID)
}

func (s *SubmissionSession) Mail(from string, opts smtp.MailOptions) error {
	// if no domain id then just drop
	domain, err := s.data.server.detectDomain(from)
	if err != nil {
		log.Printf("%s - Rcpt - From: '%s' - detectDomain error: %s", s, from, err)
		return errors.Errorf("unknown recipient (%s)", s)
	}

	/*
		TODO
		if we want to limit outbound accounts to those that match our aliases,
		uncomment
		alias, err := s.data.server.detectAlias(from)
		if err != nil {
			log.Printf("%s - Rcpt - From: '%s' - detect error: %s", s, from, err)

			// inc reject metric
			s.data.server.publishLogEntry(logger.Entry{
				AccountID: domain.AccountID,
				DomainID:  domain.ID,
				FromEmail: from,
				Etype:     logger.EntryTypeReject,
			})
			return &smtp.SMTPError{
				Code:    550,
				Message: fmt.Sprintf("unknown sender (%s)", s),
			}
		}

		s.data.Alias = alias
	*/

	s.data.Domain = domain
	s.data.From = from

	log.Printf(
		"%s - Mail - From: '%s' - AccountID: %d DomainID: %d AliasID: %d",
		s,
		from,
		s.data.Domain.AccountID,
		s.data.Domain.ID,
		s.data.Alias.ID,
	)

	return nil
}

func (s *SubmissionSession) Rcpt(to string) error {
	log.Printf("%s - Mail - To '%s'", s, to)

	s.data.To = to

	return nil
}

func (s *SubmissionSession) Data(r io.Reader) error {
	start := time.Now()

	n, err := s.data.Message.ReadFrom(r)
	if err != nil {
		log.Printf("%s - Data - ReadFrom: %s", s, err)
		return errors.Errorf("can not read message (%s)", s)
	}

	// TODO
	// do we need to add a return path?

	signed := s.data.server.bufferPool.Get().(*bytes.Buffer)
	signed.Reset()
	defer s.data.server.bufferPool.Put(signed)

	if err := s.data.server.dkimSignHandler(s.data, &s.data.Message, signed); err != nil {
		return errors.WithMessage(err, "dkimSignHandler")
	}

	err = s.data.server.queueEmail(Email{
		ID:        s.data.ID,
		From:      s.data.From,
		To:        s.data.To,
		Message:   signed.Bytes(),
		AccountID: s.data.Domain.AccountID,
		DomainID:  s.data.Domain.ID,
	})
	if err != nil {
		return errors.Wrap(err, "queueEmailHandler")
	}

	log.Printf("%s - Data - read %d bytes in %s", s, n, time.Since(start))

	return nil
}

func (s *SubmissionSession) Reset() {
	log.Printf("%s - Reset - after %s", s, time.Since(s.data.start))
	s.data.From = ""
	s.data.To = ""
	s.data.Message.Reset()
	s.data.Alias = account.Alias{}
	s.data.Domain = account.Domain{}
}

func (s *SubmissionSession) Logout() error {
	if len(s.data.From) > 0 {
		s.Reset()
	}
	log.Printf("%s - Logout", s)
	return nil
}
