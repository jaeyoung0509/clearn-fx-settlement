package frankfurter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"fx-settlement-lab/go-backend/internal/domain"
	"fx-settlement-lab/go-backend/internal/domain/shared"
	"fx-settlement-lab/go-backend/internal/port"
)

type Client struct {
	baseURL        string
	providerFilter string
	httpClient     *http.Client
}

var _ port.RateProvider = (*Client)(nil)

type ratesResponseItem struct {
	Date  string          `json:"date"`
	Base  string          `json:"base"`
	Quote string          `json:"quote"`
	Rate  decimal.Decimal `json:"rate"`
}

type errorResponse struct {
	Message string `json:"message"`
}

func NewClient(baseURL string, providerFilter string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		providerFilter: strings.TrimSpace(providerFilter),
		httpClient:     httpClient,
	}
}

func (c *Client) GetReferenceRates(ctx context.Context, base shared.Currency, quotes []shared.Currency) ([]shared.ExchangeRate, error) {
	if len(quotes) == 0 {
		return nil, nil
	}

	requestURL, err := url.Parse(c.baseURL + "/v2/rates")
	if err != nil {
		return nil, fmt.Errorf("parse frankfurter base url: %w", err)
	}

	query := requestURL.Query()
	query.Set("base", base.String())

	quoteValues := make([]string, 0, len(quotes))
	for _, quoteCurrency := range quotes {
		quoteValues = append(quoteValues, quoteCurrency.String())
	}
	query.Set("quotes", strings.Join(quoteValues, ","))
	if c.providerFilter != "" {
		query.Set("providers", c.providerFilter)
	}
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create frankfurter request: %w", err)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, domain.Internal("Failed to fetch reference rates", map[string]any{
			"provider": shared.ProviderFrankfurter,
		}).WithCause(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		var providerError errorResponse
		_ = json.NewDecoder(response.Body).Decode(&providerError)

		if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusUnprocessableEntity {
			return nil, domain.Validation("Reference rate request was rejected by provider", map[string]any{
				"provider": shared.ProviderFrankfurter,
				"message":  providerError.Message,
				"status":   response.StatusCode,
			})
		}

		return nil, domain.Internal("Reference rate provider returned an unexpected response", map[string]any{
			"provider": shared.ProviderFrankfurter,
			"status":   response.StatusCode,
			"message":  providerError.Message,
		})
	}

	var payload []ratesResponseItem
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&payload); err != nil {
		return nil, domain.Internal("Failed to decode reference rates", map[string]any{
			"provider": shared.ProviderFrankfurter,
		}).WithCause(err)
	}

	fetchedAt := time.Now().UTC()
	rates := make([]shared.ExchangeRate, 0, len(payload))
	for _, item := range payload {
		observedAt, err := time.Parse(time.DateOnly, item.Date)
		if err != nil {
			return nil, domain.Internal("Provider returned an invalid reference rate date", map[string]any{
				"provider": shared.ProviderFrankfurter,
				"value":    item.Date,
			}).WithCause(err)
		}

		baseCurrency, err := shared.ParseCurrency(item.Base)
		if err != nil {
			return nil, err
		}
		quoteCurrency, err := shared.ParseCurrency(item.Quote)
		if err != nil {
			return nil, err
		}

		rates = append(rates, shared.ExchangeRate{
			Base:       baseCurrency,
			Quote:      quoteCurrency,
			Provider:   shared.ProviderFrankfurter,
			Rate:       item.Rate,
			ObservedAt: observedAt.UTC(),
			FetchedAt:  fetchedAt,
		})
	}

	return rates, nil
}
