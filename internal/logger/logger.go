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
	DomainID      int
	AliasID       int
	DestinationID int

	// meta data
	FromEmail string
	ViaEmail  string
	ToEmail   string

	Etype EntryType

	Status string

	// actual email message
	Message []byte
}

func (e Entry) DateTime() string {
	return e.Time.Format("2006/01/02 15:04")
}

func (e Entry) GetMessage() string {
	return string(e.Message)
}

func (e Entry) EncodeTime() string {
	return e.Time.Format("20060102150405.000000")
}

func (e Entry) DecodeTime(t string) (time.Time, error) {
	return time.Parse("20060102150405.000000", t)
}
