package webhook

import (
	"encoding/json"
	"time"

	"fx-settlement-lab/go-backend/internal/domain"
)

type InboxMessage struct {
	ID              string
	Provider        string
	Topic           string
	ExternalEventID string
	ConversionID    string
	ExternalRef     *string
	Payload         json.RawMessage
	ReceivedAt      time.Time
	ProcessedAt     *time.Time
}

func (m InboxMessage) Validate() error {
	if m.ID == "" || m.Provider == "" || m.Topic == "" || m.ExternalEventID == "" || m.ConversionID == "" {
		return domain.Validation("Webhook inbox message is incomplete", map[string]any{
			"id":              m.ID,
			"provider":        m.Provider,
			"topic":           m.Topic,
			"externalEventId": m.ExternalEventID,
			"conversionId":    m.ConversionID,
		})
	}

	return nil
}
