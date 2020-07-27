package metrics

import "time"

type MetricType int

const (
	MetricTypeInboundReject = iota
	MetricTypeInboundForward
	MetricTypeInboundBounce
)

// wrap our metric for transport
type Metric struct {
	T MetricType
	M []byte
}

// when we do not find an alias or return path
// for an inbound email
type InboundReject struct {
	Time      time.Time
	FromEmail string
	ToEmail   string
	DomainID  int
}

func (i InboundReject) Type() MetricType { return MetricTypeInboundReject }

func NewInboundReject(from, to string, domainID int) *InboundReject {
	return &InboundReject{
		Time:      time.Now(),
		FromEmail: from,
		ToEmail:   to,
		DomainID:  domainID,
	}
}

// when we successfully forward a message on
type InboundForward struct {
	Time          time.Time
	FromEmail     string
	DomainID      int
	AliasID       int
	DestinationID int
}

func (i InboundForward) Type() MetricType { return MetricTypeInboundForward }

func NewInboundForward(from, to string, domainID, aliasID, destinationID int) *InboundForward {
	return &InboundForward{
		Time:          time.Now(),
		FromEmail:     from,
		DomainID:      domainID,
		AliasID:       aliasID,
		DestinationID: destinationID,
	}
}

// when we get a bounce attempting to forward
// a message on
type InboundBounce struct {
	Time          time.Time
	FromEmail     string
	DomainID      int
	AliasID       int
	DestinationID int
	Reason        string
	Message       []byte
}

func (i InboundBounce) Type() MetricType { return MetricTypeInboundBounce }

func NewInboundBounce(from, to, reason string, domainID, aliasID, destinationID int, message []byte) *InboundBounce {
	return &InboundBounce{
		Time:          time.Now(),
		FromEmail:     from,
		DomainID:      domainID,
		AliasID:       aliasID,
		DestinationID: destinationID,
		Reason:        reason,
		Message:       message,
	}
}
