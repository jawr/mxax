package smtp

import (
	"bytes"
	"context"
	"log"
	"os"
	"sync"

	"github.com/dgraph-io/ristretto"
	"github.com/emersion/go-smtp"
	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

// Server will listen for smtp connections
// and check them against various rules in the
// database. Expected to have a load balancer
// in front, i.e. HaProxy
type Server struct {
	db *pgx.Conn
	s  *smtp.Server

	// handlers
	aliasHandler      aliasHandlerFn
	domainHandler     domainHandlerFn
	queueEmailHandler queueEmailHandlerFn
	forwardHandler    forwardHandlerFn
	returnPathHandler returnPathHandlerFn

	// publishers
	logPublisher   *rabbitmq.Channel
	emailPublisher *rabbitmq.Channel

	// bytes pool
	bufferPool sync.Pool

	// caches
	usernameCache   *ristretto.Cache
	nxusernameCache *ristretto.Cache
}

// Create a new Server, currently only handles inbound
// connections
func NewServer(db *pgx.Conn, logPublisher, emailPublisher *rabbitmq.Channel) (*Server, error) {
	usernameCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache usernameCache")
	}

	nxusernameCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache nxusernameCache")
	}

	server := &Server{
		db:             db,
		logPublisher:   logPublisher,
		emailPublisher: emailPublisher,
		bufferPool: sync.Pool{
			New: func() interface{} { return new(bytes.Buffer) },
		},
		usernameCache:   usernameCache,
		nxusernameCache: nxusernameCache,
	}

	// setup handlers using closures to keep from polluting the server struct

	server.aliasHandler, err = server.makeAliasHandler(db)
	if err != nil {
		return nil, errors.WithMessage(err, "makeAliasHandler")
	}

	server.domainHandler, err = server.makeDomainHandler(db)
	if err != nil {
		return nil, errors.WithMessage(err, "makeDomainHandler")
	}

	server.queueEmailHandler, err = server.makeQueueEmailHandler(db)
	if err != nil {
		return nil, errors.WithMessage(err, "makeQueueEmailHandler")
	}

	server.forwardHandler, err = server.makeForwardHandler(db)
	if err != nil {
		return nil, errors.WithMessage(err, "server.makeForwardHandler")
	}

	server.returnPathHandler, err = server.makeReturnPathHandler(db)
	if err != nil {
		return nil, errors.WithMessage(err, "server.makeReturnPathHandler")
	}

	// setup the underlying smtp server
	server.s = smtp.NewServer(server)

	if len(os.Getenv("MXAX_DEBUG")) > 0 {
		server.s.Debug = os.Stdout
	}

	return server, nil
}

func (s *Server) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	session, err := s.newOutboundSession(s.s.Domain, state)
	if err != nil {
		log.Printf("Login; unable to create new OutboundSession: %s", err)
		return nil, errors.New("temporary error, please try again later")
	}

	log.Printf("OB - %s - try auth with %s / %s", session, username, password)

	// filter out bad user/pass
	if _, ok := s.nxusernameCache.Get(username); ok {
		return nil, smtp.ErrAuthUnsupported
	}

	// buffer pool?
	var cachedPassword []byte
	cacheGet, ok := s.usernameCache.Get(password)

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
			s.nxusernameCache.Set(username, struct{}{}, 1)
			return nil, errors.New("Not authorized")
		}

		s.usernameCache.Set(username, cachedPassword, 1)
	} else {
		cachedPassword = cacheGet.([]byte)
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
	session, err := s.newInboundSession(s.s.Domain, state)
	if err != nil {
		log.Printf("AnonymousLogin; unable to create new InboundSession: %s", err)
		return nil, errors.New("temporary error, please try again later")
	}

	log.Printf("IB - %s - init", session)

	return session, nil
}
