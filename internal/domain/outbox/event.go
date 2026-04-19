package outbox

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID              string
	AggregateType   string
	AggregateID     string
	EventType       string
	Payload         json.RawMessage
	CreatedAt       time.Time
	PublishedAt     *time.Time
	PublishAttempts int
	LastError       *string
}

func (e Event) IsPublished() bool {
	return e.PublishedAt != nil
}
