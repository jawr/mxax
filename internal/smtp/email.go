package smtp

import (
	"github.com/google/uuid"
)

type Email struct {
	ID      uuid.UUID
	From    string
	To      string
	Message []byte
}
