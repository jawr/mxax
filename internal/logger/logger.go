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

func (e EntryType) String() string {
	switch e {
	case EntryTypeSend:
		return "SND"
	case EntryTypeReject:
		return "REJ"
	case EntryTypeBounce:
		return "BNC"
	default:
		return "Unknown"
	}
}

type Entry struct {
	Time time.Time

	ID uuid.UUID

	// for charting and deleting
	AccountID     int
	DomainID      int
	AliasID       int
	DestinationID int

	// meta data
	FromEmail string
	ViaEmail  string
	ToEmail   string

	Etype EntryType

	Status string
	Bounce string

	QueueLevel int

	// actual email message
	Message []byte
}

func (e Entry) DateTime() string {
	return e.Time.Format("15:04 01/02/06")
}

func (e Entry) GetMessage() string {
	return string(e.Message)
}

func (e Entry) EncodeTime() string {
	return e.Time.Format("20060102150405.999999999-0700")
}

func (e Entry) DecodeTime(t string) (time.Time, error) {
	return time.Parse("20060102150405.999999999-0700", t)
}
