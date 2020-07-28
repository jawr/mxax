package smtp

import (
	"github.com/google/uuid"
)

type Email struct {
	ID         uuid.UUID
	From       string
	ReturnPath string
	To         string
	Message    []byte

	// for metrics
	DomainID      int
	AliasID       int
	DestinationID int

	Bounce string
}

func (e *Email) Reset() {
	e.ID = uuid.Nil
	e.From = ""
	e.To = ""
	e.Message = nil
	e.DomainID = 0
	e.AliasID = 0
	e.DestinationID = 0
	e.Bounce = ""
}
