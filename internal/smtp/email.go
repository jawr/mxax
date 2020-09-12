package smtp

import (
	"github.com/google/uuid"
	"github.com/jawr/mxax/internal/logger"
)

type Email struct {
	ID         uuid.UUID
	From       string
	ReturnPath string
	Via        string
	To         string
	Message    []byte

	QueueLevel QueueLevel

	// for metrics
	AccountID     int
	DomainID      int
	AliasID       int
	DestinationID int

	// internals for sender
	Etype  logger.EntryType
	Error  error
	Status string
	Bounce string
}

func (e *Email) Reset() {
	e.ID = uuid.Nil
	e.From = ""
	e.ReturnPath = ""
	e.Via = ""
	e.To = ""
	e.Message = nil
	e.AccountID = 0
	e.DomainID = 0
	e.AliasID = 0
	e.DestinationID = 0
	e.Bounce = ""
	e.Status = ""
	e.Error = nil
	e.QueueLevel = QueueLevelStraw
	e.Etype = logger.EntryTypeSend
}
