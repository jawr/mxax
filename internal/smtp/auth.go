package smtp

import (
	"context"
	"log"
	"strings"

	"github.com/emersion/go-smtp"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) Login(state *smtp.ConnectionState, email, password string) (smtp.Session, error) {
	if !strings.Contains(state.LocalAddr.String(), ":587") {
		return nil, smtp.ErrAuthRequired
	}

	session, err := s.newSubmissionSession(s.submissionServer.Domain, state)
	if err != nil {
		log.Printf("Login; unable to create new SubmissionSession: %s", err)
		return nil, errors.New("temporary error, please try again later")
	}

	log.Printf("%s - try auth with %s / %s", session, email, password)

	// filter out bad user/pass
	if _, ok := s.cache.Get("login", email); ok {
		log.Printf("%s - auth failed with %s / %s", session, email, password)
		return nil, smtp.ErrAuthUnsupported
	}

	// buffer pool?
	var cachedPassword []byte
	cachedGet, ok := s.cache.Get("login", password)

	if !ok {
		// look for good user
		err = s.db.QueryRow(
			context.Background(),
			`
			SELECT password FROM accounts WHERE email = $1
			`,
			email,
		).Scan(&cachedPassword)
		if err != nil {
			s.cache.Set("login", email, struct{}{})
			log.Printf("%s - auth failed with %s / %s (%s)", session, email, password, err)
			return nil, errors.New("Not authorized")
		}

		s.cache.Set("login", email, cachedPassword)
	} else {
		cachedPassword = cachedGet.([]byte)
	}

	if err := bcrypt.CompareHashAndPassword(cachedPassword, []byte(password)); err != nil {
		// TODO
		// fail2ban type
		log.Printf("%s - auth failed with %s / %s (%s)", session, email, password, err)
		return nil, errors.New("Not authorized")
	}

	log.Printf("%s - init", session)

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

	log.Printf("%s - init", session)

	return session, nil
}
