package keyset

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"fx-settlement-lab/go-backend/internal/domain"
)

const (
	OrderAsc  = "asc"
	OrderDesc = "desc"
)

type Query struct {
	Sort   string
	Order  string
	Limit  int
	Cursor string
}

type Page[T any] struct {
	Items      []T
	NextCursor *string
}

type Cursor struct {
	Sort          string  `json:"sort"`
	Order         string  `json:"order"`
	LastValue     string  `json:"last_value"`
	LastSecondary *string `json:"last_secondary"`
	LastID        string  `json:"last_id"`
}

type Codec interface {
	Parse(raw string) (any, error)
	Serialize(value any) (string, error)
}

type StringCodec struct{}

func (StringCodec) Parse(raw string) (any, error) {
	return raw, nil
}

func (StringCodec) Serialize(value any) (string, error) {
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("expected string cursor value, got %T", value)
	}

	return stringValue, nil
}

type RFC3339TimeCodec struct{}

func (RFC3339TimeCodec) Parse(raw string) (any, error) {
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (RFC3339TimeCodec) Serialize(value any) (string, error) {
	timeValue, ok := value.(time.Time)
	if !ok {
		return "", fmt.Errorf("expected time.Time cursor value, got %T", value)
	}

	return timeValue.UTC().Format(time.RFC3339Nano), nil
}

type SortDefinition struct {
	PrimaryColumn   string
	SecondaryColumn string
	PrimaryCodec    Codec
	SecondaryCodec  Codec
}

type Plan struct {
	Sort       string
	Order      string
	definition SortDefinition
	Cursor     *Cursor
}

func EncodeCursor(cursor Cursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(raw), nil
}

func DecodeCursor(raw string) (Cursor, error) {
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, domain.Validation("Invalid cursor format", nil).WithCause(err)
	}

	var cursor Cursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return Cursor{}, domain.Validation("Invalid cursor format", nil).WithCause(err)
	}

	return cursor, nil
}

func NewPlan(query Query, definitions map[string]SortDefinition) (Plan, error) {
	definition, ok := definitions[query.Sort]
	if !ok {
		return Plan{}, domain.Validation("Invalid sort or order value", nil)
	}

	if query.Order != OrderAsc && query.Order != OrderDesc {
		return Plan{}, domain.Validation("Invalid sort or order value", nil)
	}

	plan := Plan{
		Sort:       query.Sort,
		Order:      query.Order,
		definition: normalizeDefinition(definition),
	}

	if query.Cursor == "" {
		return plan, nil
	}

	cursor, err := DecodeCursor(query.Cursor)
	if err != nil {
		return Plan{}, err
	}

	if cursor.Sort != plan.Sort || cursor.Order != plan.Order {
		return Plan{}, domain.Validation("Cursor sort/order mismatch", map[string]any{
			"cursor_sort":  cursor.Sort,
			"cursor_order": cursor.Order,
			"query_sort":   plan.Sort,
			"query_order":  plan.Order,
		})
	}

	plan.Cursor = &cursor

	if _, err := plan.primaryValue(); err != nil {
		return Plan{}, domain.Validation("Invalid cursor format", nil).WithCause(err)
	}

	if plan.hasSecondary() {
		if plan.Cursor.LastSecondary == nil {
			return Plan{}, domain.Validation("Invalid cursor format for this sort", nil)
		}
		if _, err := plan.secondaryValue(); err != nil {
			return Plan{}, domain.Validation("Invalid cursor format for this sort", nil).WithCause(err)
		}
	}

	return plan, nil
}

func (p Plan) OrderBy() []string {
	direction := OrderAsc
	if p.Order == OrderDesc {
		direction = OrderDesc
	}

	orderBy := []string{p.definition.PrimaryColumn + " " + direction}
	if p.hasSecondary() {
		orderBy = append(orderBy, p.definition.SecondaryColumn+" "+direction)
	}
	orderBy = append(orderBy, "id "+direction)

	return orderBy
}

func (p Plan) WhereClause() (string, []any, error) {
	if p.Cursor == nil {
		return "", nil, nil
	}

	operator := ">"
	if p.Order == OrderDesc {
		operator = "<"
	}

	primaryValue, err := p.primaryValue()
	if err != nil {
		return "", nil, domain.Validation("Invalid cursor format", nil).WithCause(err)
	}

	if !p.hasSecondary() {
		return fmt.Sprintf(
				"(%s %s ?) OR (%s = ? AND id %s ?)",
				p.definition.PrimaryColumn,
				operator,
				p.definition.PrimaryColumn,
				operator,
			), []any{
				primaryValue,
				primaryValue,
				p.Cursor.LastID,
			}, nil
	}

	secondaryValue, err := p.secondaryValue()
	if err != nil {
		return "", nil, domain.Validation("Invalid cursor format for this sort", nil).WithCause(err)
	}

	return fmt.Sprintf(
			"(%s %s ?) OR (%s = ? AND %s %s ?) OR (%s = ? AND %s = ? AND id %s ?)",
			p.definition.PrimaryColumn,
			operator,
			p.definition.PrimaryColumn,
			p.definition.SecondaryColumn,
			operator,
			p.definition.PrimaryColumn,
			p.definition.SecondaryColumn,
			operator,
		), []any{
			primaryValue,
			primaryValue,
			secondaryValue,
			primaryValue,
			secondaryValue,
			p.Cursor.LastID,
		}, nil
}

func (p Plan) EncodeNextCursor(primaryValue any, lastID string, secondaryValue any) (string, error) {
	primaryRaw, err := p.definition.PrimaryCodec.Serialize(primaryValue)
	if err != nil {
		return "", domain.Validation("Invalid cursor format", nil).WithCause(err)
	}

	var secondaryRaw *string
	if p.hasSecondary() {
		if secondaryValue == nil {
			return "", domain.Validation("Invalid cursor format for this sort", nil)
		}

		serializedSecondary, err := p.definition.SecondaryCodec.Serialize(secondaryValue)
		if err != nil {
			return "", domain.Validation("Invalid cursor format for this sort", nil).WithCause(err)
		}
		secondaryRaw = &serializedSecondary
	}

	return EncodeCursor(Cursor{
		Sort:          p.Sort,
		Order:         p.Order,
		LastValue:     primaryRaw,
		LastSecondary: secondaryRaw,
		LastID:        lastID,
	})
}

func (p Plan) primaryValue() (any, error) {
	if p.Cursor == nil {
		return nil, nil
	}

	return p.definition.PrimaryCodec.Parse(p.Cursor.LastValue)
}

func (p Plan) secondaryValue() (any, error) {
	if p.Cursor == nil || !p.hasSecondary() || p.Cursor.LastSecondary == nil {
		return nil, nil
	}

	return p.definition.SecondaryCodec.Parse(*p.Cursor.LastSecondary)
}

func (p Plan) hasSecondary() bool {
	return p.definition.SecondaryColumn != ""
}

func normalizeDefinition(definition SortDefinition) SortDefinition {
	if definition.PrimaryCodec == nil {
		definition.PrimaryCodec = StringCodec{}
	}

	if definition.SecondaryColumn != "" && definition.SecondaryCodec == nil {
		definition.SecondaryCodec = StringCodec{}
	}

	return definition
}
