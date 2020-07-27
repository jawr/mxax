package smtp

import (
	"github.com/google/uuid"
)

type Email struct {
	ID      uuid.UUID
	From    string
	To      string
	Message []byte

	// for metrics
	DomainID      int
	AliasID       int
	DestinationID int
}

func (e *Email) Reset() {
	e.ID = uuid.Nil
	e.From = ""
	e.To = ""
	e.Message = nil
}
