package shared

import (
	"encoding/json"
	"time"
)

const (
	ProviderFrankfurter = "frankfurter"
	ProviderToss        = "toss"
	ProviderStripe      = "stripe"
	ProviderPlaid       = "plaid"
	ProviderWise        = "wise"
	ProviderDemo        = "demo"
)

func IsSupportedPaymentProvider(provider string) bool {
	switch provider {
	case ProviderToss, ProviderStripe, ProviderDemo:
		return true
	default:
		return false
	}
}

func IsSupportedTransferProvider(provider string) bool {
	switch provider {
	case ProviderWise, ProviderPlaid, ProviderDemo:
		return true
	default:
		return false
	}
}

type ProviderEvent struct {
	Provider          string
	Topic             string
	ExternalEventID   string
	ConversionID      string
	ExternalReference *string
	OccurredAt        time.Time
	Payload           json.RawMessage
}
