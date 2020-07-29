package logger

import (
	"time"

	"github.com/google/uuid"
)

type EntryType int

const (
	EntryTypeSend EntryType = iota
	EntryTypeReject
	EntryTypeBounce
)

type Entry struct {
	Time time.Time

	ID uuid.UUID

	// for charting and deleting
	DomainID int

	// meta data
	FromEmail string
	ViaEmail  string
	ToEmail   string

	Etype EntryType

	Status string

	// actual email message
	Message []byte
}
