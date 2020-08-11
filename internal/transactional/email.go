package transactional

import "net/mail"

type Email struct {
	To mail.Address

	Subject string
	HTML    []byte
}
