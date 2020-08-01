package smtp

import (
	"bytes"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
)

type SessionData struct {
	start time.Time

	ID uuid.UUID

	// connection meta data
	State *smtp.ConnectionState

	ServerName string

	// email
	From    string
	Via     string
	To      string
	Message bytes.Buffer

	// account details
	AccountID int
	DomainID  int
	AliasID   int

	// reference to the server
	server *Server

	// internal flags
	returnPath bool
}
