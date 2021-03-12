package smtp

import (
	"bytes"
	"crypto/tls"
	"os"
	"sync"

	"github.com/emersion/go-smtp"
	"github.com/isayme/go-amqp-reconnect/rabbitmq"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jawr/mxax/internal/cache"
	"github.com/pkg/errors"
)

// Server will listen for smtp connections
// and check them against various rules in the
// database. Expected to have a load balancer
// in front, i.e. HaProxy
type Server struct {
	db *pgxpool.Pool

	// underlying smtp servers, one for :smtp (aka relay)
	// one for :submission
	relayServer      *smtp.Server
	submissionServer *smtp.Server

	// publishers
	logPublisher   *rabbitmq.Channel
	emailPublisher *rabbitmq.Channel

	// bytes pool
	bufferPool sync.Pool

	// multi purpose cache, strings are prefixed with namespace
	cache *cache.Cache
}

// Create a new Server, currently only handles inbound
// connections
func NewServer(db *pgxpool.Pool, logPublisher, emailPublisher *rabbitmq.Channel) (*Server, error) {
	cache, err := cache.NewCache()
	if err != nil {
		return nil, errors.WithMessage(err, "NewCache")
	}

	// load our certificate for TLS
	cert, err := tls.LoadX509KeyPair(
		"/etc/letsencrypt/live/ehlo.mx.ax/fullchain.pem",
		"/etc/letsencrypt/live/ehlo.mx.ax/privkey.pem",
	)
	if err != nil {
		return nil, errors.WithMessage(err, "tls.LoadX509KeyPair")
	}

	tlsConfig := &tls.Config{
		ServerName:   "ehlo.mx.ax",
		Certificates: []tls.Certificate{cert},
	}

	server := &Server{
		db:             db,
		logPublisher:   logPublisher,
		emailPublisher: emailPublisher,
		cache:          cache,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}

	// setup the underlying smtp servers
	server.relayServer = smtp.NewServer(server)
	server.relayServer.Addr = ":smtp"

	server.submissionServer = smtp.NewServer(server)
	server.submissionServer.Addr = ":submission"

	server.relayServer.TLSConfig = tlsConfig
	server.submissionServer.TLSConfig = tlsConfig

	if len(os.Getenv("MXAX_DEBUG")) > 0 {
		server.relayServer.Debug = os.Stdout
		server.submissionServer.Debug = os.Stdout
	}

	return server, nil
}
