package smtp

type OutboundSession struct {
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
}

func (s *Server) newOutboundSession(serverName string, state *smtp.ConnectionState) (*OutboundSession, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	session := OutboundSession{
		ID:         id,
		start:      time.Now(),
		ServerName: serverName,
		State:      state,
		server:     s,
	}

	return &session, nil
}

func (s *OutboundSession) String() string {
	return fmt.Sprintf("%s", s.ID)
}

func (s *OutboundSession) Mail(from string, opts smtp.MailOptions) error {
	// if no domain id then just drop
	accountID, domainID, err := s.server.domainHandler(from)
	if err != nil {
		log.Printf("OB - %s - Rcpt - From: '%s' - domainHandler error: %s", s, from, err)
		return errors.Errorf("unknown recipient (%s)", s)
	}

	aliasID, err := s.server.aliasHandler(from)
	if err != nil {
		log.Printf("OB - %s - Rcpt - From: '%s' - aliasHandler error: %s", s, from, err)

		// inc reject metric
		s.server.publishLogEntry(logger.Entry{
			AccountID: accountID,
			DomainID:  domainID,
			AliasID:   aliasID,
			FromEmail: from,
			Etype:     logger.EntryTypeReject,
		})
		return &smtp.SMTPError{
			Code:    550,
			Message: fmt.Sprintf("unknown sender (%s)", s),
		}
	}

	s.AliasID = aliasID
	s.AccountID = accountID
	s.DomainID = domainID
	s.From = from

	log.Printf("OB - %s - Mail - From: '%s' - AliasID: %d", s, from, s.AliasID)

	return nil
}

func (s *OutboundSession) Rcpt(to string) error {
	log.Printf("OB - %s - Mail - To '%s'", s, to)

	s.To = to

	return nil
}

func (s *OutboundSession) Data(r io.Reader) error {
	start := time.Now()

	n, err := s.Message.ReadFrom(r)
	if err != nil {
		log.Printf("OB - %s - Data - ReadFrom: %s", s, err)
		return errors.Errorf("can not read message (%s)", s)
	}

	if err := s.server.forwardHandler(s); err != nil {
		log.Printf("OB - %s - Data - forwardHandler: %s", s, err)
		return errors.Errorf("unable to forward this message (%s)", s)
	}

	log.Printf("OB - %s - Data - read %d bytes in %s", s, n, time.Since(start))

	return nil
}

func (s *OutboundSession) Reset() {
	log.Printf("OB - %s - Reset - after %s", s, time.Since(s.start))
	s.From = ""
	s.To = ""
	s.Message.Reset()
	s.AliasID = 0
}

func (s *OutboundSession) Logout() error {
	if len(s.From) > 0 {
		s.Reset()
	}
	log.Printf("OB - %s - Logout", s)
	return nil
}

