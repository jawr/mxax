package smtp

import (
	"context"
	"log"
	"strings"

	"github.com/emersion/go-smtp"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	if !strings.Contains(state.LocalAddr.String(), ":587") {
		return nil, smtp.ErrAuthRequired
	}

	session, err := s.newSubmissionSession(s.submissionServer.Domain, state)
	if err != nil {
		log.Printf("Login; unable to create new SubmissionSession: %s", err)
		return nil, errors.New("temporary error, please try again later")
	}

	log.Printf("OB - %s - try auth with %s / %s", session, username, password)

	// filter out bad user/pass
	if _, ok := s.cacheGet("login", username); ok {
		return nil, smtp.ErrAuthUnsupported
	}

	// buffer pool?
	var cachedPassword []byte
	cachedGet, ok := s.cacheGet("login", password)

	if !ok {
		// look for good user
		err = s.db.QueryRow(
			context.Background(),
			`
			SELECT password FROM accounts WHERE username = $1
			`,
			username,
		).Scan(&cachedPassword)
		if err != nil {
			s.cacheSet("login", username, struct{}{})
			return nil, errors.New("Not authorized")
		}

		s.cacheSet("login", username, cachedPassword)
	} else {
		cachedPassword = cachedGet.([]byte)
	}

	if err := bcrypt.CompareHashAndPassword(cachedPassword, []byte(password)); err != nil {
		// TODO
		// fail2ban type
		return nil, errors.New("Not authorized")
	}

	log.Printf("OB - %s - init", session)

	return session, nil
}

func (s *Server) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	if !strings.Contains(state.LocalAddr.String(), ":25") {
		return nil, smtp.ErrAuthRequired
	}

	session, err := s.newRelaySession(s.relayServer.Domain, state)
	if err != nil {
		log.Printf("AnonymousLogin; unable to create new RelaySession: %s", err)
		return nil, errors.New("temporary error, please try again later")
	}

	log.Printf("IB - %s - init", session)

	return session, nil
}
