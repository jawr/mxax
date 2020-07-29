package smtp

import (
	"bytes"
	"log"
	"os"
	"sync"

	"github.com/emersion/go-smtp"
	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
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
}

// Create a new Server, currently only handles inbound
// connections
func NewServer(db *pgx.Conn, logPublisher, emailPublisher *rabbitmq.Channel) (*Server, error) {

	server := &Server{
		db:             db,
		logPublisher:   logPublisher,
		emailPublisher: emailPublisher,
		bufferPool: sync.Pool{
			New: func() interface{} { return new(bytes.Buffer) },
		},
	}

	// setup handlers using closures to keep from polluting the server struct
	var err error

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
	// return an OutboundSession
	return nil, smtp.ErrAuthUnsupported
}

func (s *Server) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	session, err := s.newInboundSession(s.s.Domain, state)
	if err != nil {
		log.Printf("AnonymousLogin; unable to create new InboundSession: %s", err)
		return nil, errors.New("temporary error, please try again later")
	}

	log.Printf("%s - init", session)

	return session, nil
}
