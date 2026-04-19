package keyset

import (
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"fx-settlement-lab/go-backend/internal/domain"
)

const (
	sortName      = "name"
	sortAisle     = "aisle"
	sortCreatedAt = "created_at"
)

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	payload := Cursor{
		Sort:      sortName,
		Order:     OrderAsc,
		LastValue: "apple",
		LastID:    ulid.Make().String(),
	}

	cursor, err := EncodeCursor(payload)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	decoded, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}

	if decoded.Sort != payload.Sort || decoded.Order != payload.Order || decoded.LastValue != payload.LastValue || decoded.LastID != payload.LastID {
		t.Fatalf("decoded payload mismatch: %#v", decoded)
	}
}

func TestNewPlanWithRFC3339Primary(t *testing.T) {
	t.Parallel()

	cursor, err := EncodeCursor(Cursor{
		Sort:      sortCreatedAt,
		Order:     OrderDesc,
		LastValue: "2026-02-06T17:00:00Z",
		LastID:    ulid.Make().String(),
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	plan, err := NewPlan(Query{
		Sort:   sortCreatedAt,
		Order:  OrderDesc,
		Limit:  20,
		Cursor: cursor,
	}, map[string]SortDefinition{
		sortCreatedAt: {
			PrimaryColumn: "created_at",
			PrimaryCodec:  RFC3339TimeCodec{},
		},
	})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	clause, args, err := plan.WhereClause()
	if err != nil {
		t.Fatalf("where clause: %v", err)
	}

	if clause == "" || len(args) != 3 {
		t.Fatalf("unexpected where clause output: clause=%q args=%#v", clause, args)
	}
	if _, ok := args[0].(time.Time); !ok {
		t.Fatalf("expected time.Time primary value, got %T", args[0])
	}
}

func TestNewPlanRejectsSortOrderMismatch(t *testing.T) {
	t.Parallel()

	secondary := "apple"
	cursor, err := EncodeCursor(Cursor{
		Sort:          sortAisle,
		Order:         OrderAsc,
		LastValue:     "produce",
		LastSecondary: &secondary,
		LastID:        ulid.Make().String(),
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	_, err = NewPlan(Query{
		Sort:   sortName,
		Order:  OrderAsc,
		Limit:  20,
		Cursor: cursor,
	}, sortableDefinitions())
	if err == nil {
		t.Fatal("expected mismatch error")
	}

	appErr := domain.AsAppError(err)
	if appErr.Code != domain.ErrorCodeValidation || appErr.Message != "Cursor sort/order mismatch" {
		t.Fatalf("unexpected error: %#v", appErr)
	}
}

func TestNewPlanRequiresSecondaryValue(t *testing.T) {
	t.Parallel()

	cursor, err := EncodeCursor(Cursor{
		Sort:      sortName,
		Order:     OrderAsc,
		LastValue: "apple",
		LastID:    ulid.Make().String(),
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	_, err = NewPlan(Query{
		Sort:   sortName,
		Order:  OrderAsc,
		Limit:  20,
		Cursor: cursor,
	}, sortableDefinitions())
	if err == nil {
		t.Fatal("expected secondary value error")
	}

	appErr := domain.AsAppError(err)
	if appErr.Code != domain.ErrorCodeValidation || appErr.Message != "Invalid cursor format for this sort" {
		t.Fatalf("unexpected error: %#v", appErr)
	}
}

func TestPlanEncodesNextCursor(t *testing.T) {
	t.Parallel()

	plan, err := NewPlan(Query{
		Sort:  sortAisle,
		Order: OrderDesc,
		Limit: 20,
	}, sortableDefinitions())
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	lastID := ulid.Make().String()
	cursor, err := plan.EncodeNextCursor("produce", lastID, "apple")
	if err != nil {
		t.Fatalf("encode next cursor: %v", err)
	}

	decoded, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}

	if decoded.Sort != sortAisle || decoded.Order != OrderDesc || decoded.LastValue != "produce" || decoded.LastID != lastID {
		t.Fatalf("unexpected decoded cursor: %#v", decoded)
	}
	if decoded.LastSecondary == nil || *decoded.LastSecondary != "apple" {
		t.Fatalf("unexpected secondary: %#v", decoded.LastSecondary)
	}
}

func sortableDefinitions() map[string]SortDefinition {
	return map[string]SortDefinition{
		sortName: {
			PrimaryColumn:   "name",
			SecondaryColumn: "aisle",
		},
		sortAisle: {
			PrimaryColumn:   "aisle",
			SecondaryColumn: "name",
		},
		sortCreatedAt: {
			PrimaryColumn: "created_at",
			PrimaryCodec:  RFC3339TimeCodec{},
		},
	}
}
