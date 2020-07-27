package smtp

import (
	"log"
	"os"

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
	queueEmailHandler queueEmailHandlerFn
	forwardHandler    forwardHandlerFn
	returnPathHandler returnPathHandlerFn

	// publishers
	metricPublisher *rabbitmq.Channel
	emailPublisher  *rabbitmq.Channel
}

// Create a new Server, currently only handles inbound
// connections
func NewServer(db *pgx.Conn, metricPublisher, emailPublisher *rabbitmq.Channel) (*Server, error) {

	server := &Server{
		db:              db,
		metricPublisher: metricPublisher,
		emailPublisher:  emailPublisher,
	}

	// setup handlers closures to keep logic close
	var err error

	server.aliasHandler, err = server.makeAliasHandler(db)
	if err != nil {
		return nil, errors.WithMessage(err, "makeAliasHandler")
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

	server.s = smtp.NewServer(server)

	if len(os.Getenv("MXAX_DEBUG")) > 0 {
		server.s.Debug = os.Stdout
	}

	return server, nil
}

func (s *Server) Run(domain string) error {
	s.s.Domain = domain
	return s.s.ListenAndServe()
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
