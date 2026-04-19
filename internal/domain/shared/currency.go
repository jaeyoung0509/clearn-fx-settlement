package shared

import (
	"strings"

	"fx-settlement-lab/go-backend/internal/domain"
)

type Currency string

const (
	CurrencyKRW Currency = "KRW"
	CurrencyUSD Currency = "USD"
	CurrencyJPY Currency = "JPY"
	CurrencyEUR Currency = "EUR"
)

var supportedCurrencies = map[Currency]int32{
	CurrencyKRW: 0,
	CurrencyUSD: 2,
	CurrencyJPY: 0,
	CurrencyEUR: 2,
}

func ParseCurrency(raw string) (Currency, error) {
	currency := Currency(strings.ToUpper(strings.TrimSpace(raw)))
	if _, ok := supportedCurrencies[currency]; !ok {
		return "", domain.Validation("Unsupported currency", map[string]any{
			"currency": raw,
		})
	}

	return currency, nil
}

func MustCurrency(raw string) Currency {
	currency, err := ParseCurrency(raw)
	if err != nil {
		panic(err)
	}

	return currency
}

func (c Currency) Scale() int32 {
	return supportedCurrencies[c]
}

func (c Currency) String() string {
	return string(c)
}
