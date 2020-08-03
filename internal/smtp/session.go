package smtp

import (
	"bytes"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/account"
)

type SessionData struct {
	start time.Time

	ID uuid.UUID

	// references server
	server *Server

	// connection meta data
	State *smtp.ConnectionState

	ServerName string

	// email
	From    string
	Via     string
	To      string
	Message bytes.Buffer

	// account structs
	Domain account.Domain
	Alias  account.Alias

	// internal flags
	returnPath bool
}
