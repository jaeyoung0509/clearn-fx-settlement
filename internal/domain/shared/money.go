package shared

import (
	"fmt"

	"github.com/shopspring/decimal"

	"fx-settlement-lab/go-backend/internal/domain"
)

type Money struct {
	Currency   Currency
	MinorUnits int64
}

func NewMoney(currency Currency, minorUnits int64) (Money, error) {
	if _, ok := supportedCurrencies[currency]; !ok {
		return Money{}, domain.Validation("Unsupported currency", map[string]any{
			"currency": currency,
		})
	}

	return Money{
		Currency:   currency,
		MinorUnits: minorUnits,
	}, nil
}

func MustMoney(currency Currency, minorUnits int64) Money {
	money, err := NewMoney(currency, minorUnits)
	if err != nil {
		panic(err)
	}

	return money
}

func (m Money) Scale() int32 {
	return m.Currency.Scale()
}

func (m Money) DecimalAmount() decimal.Decimal {
	return decimal.NewFromInt(m.MinorUnits).Shift(-m.Scale())
}

func (m Money) AmountString() string {
	return m.DecimalAmount().StringFixedBank(m.Scale())
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, domain.Validation("Currency mismatch", map[string]any{
			"left":  m.Currency,
			"right": other.Currency,
		})
	}

	return Money{
		Currency:   m.Currency,
		MinorUnits: m.MinorUnits + other.MinorUnits,
	}, nil
}

func (m Money) ValidatePositive(field string) error {
	if m.MinorUnits <= 0 {
		return domain.Validation("Money amount must be positive", map[string]any{
			"field":      field,
			"currency":   m.Currency,
			"minorUnits": m.MinorUnits,
		})
	}

	return nil
}

func Convert(base Money, target Currency, rate decimal.Decimal) (Money, error) {
	if err := base.ValidatePositive("baseAmount"); err != nil {
		return Money{}, err
	}
	if rate.LessThanOrEqual(decimal.Zero) {
		return Money{}, domain.Validation("Exchange rate must be positive", map[string]any{
			"rate": rate.String(),
		})
	}

	targetAmount := base.DecimalAmount().Mul(rate)
	minorUnits := targetAmount.Shift(target.Scale()).RoundBank(0).IntPart()

	return NewMoney(target, minorUnits)
}

func FeeFromBPS(base Money, feeBPS int64, minimum Money) (Money, error) {
	if base.Currency != minimum.Currency {
		return Money{}, domain.Validation("Fee currency mismatch", map[string]any{
			"baseCurrency":    base.Currency,
			"minimumCurrency": minimum.Currency,
		})
	}
	if feeBPS < 0 {
		return Money{}, domain.Validation("Fee basis points must be zero or positive", map[string]any{
			"feeBPS": feeBPS,
		})
	}

	calculated := (base.MinorUnits * feeBPS) / 10000
	if (base.MinorUnits*feeBPS)%10000 != 0 {
		calculated++
	}
	if calculated < minimum.MinorUnits {
		calculated = minimum.MinorUnits
	}

	return NewMoney(base.Currency, calculated)
}

func (m Money) String() string {
	return fmt.Sprintf("%s %s", m.Currency, m.AmountString())
}
