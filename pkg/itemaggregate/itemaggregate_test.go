package itemaggregate

import (
	"context"
	"errors"
	"testing"
)

func TestAggregateKeepsAllAvailableItems(t *testing.T) {
	items := []Item{{ID: "a"}, {ID: "b"}}
	result := Aggregate(context.Background(), items, func(ctx context.Context, item Item) (string, error) {
		return "value-" + item.ID, nil
	})
	values := result.Available()
	if len(values) != 2 || values[0] != "value-a" || values[1] != "value-b" {
		t.Fatalf("Available() = %#v, want both values", values)
	}
	if len(result.Unavailable()) != 0 {
		t.Fatalf("Unavailable() = %#v, want none", result.Unavailable())
	}
}

func TestAggregateIsolatesSingleFailure(t *testing.T) {
	items := []Item{{ID: "a"}, {ID: "b"}}
	result := Aggregate(context.Background(), items, func(ctx context.Context, item Item) (string, error) {
		if item.ID == "b" {
			return "", errors.New("boom")
		}
		return "value-" + item.ID, nil
	})
	values := result.Available()
	if len(values) != 1 || values[0] != "value-a" {
		t.Fatalf("Available() = %#v, want only value-a", values)
	}
	unavailable := result.Unavailable()
	if len(unavailable) != 1 {
		t.Fatalf("Unavailable() = %#v, want one", unavailable)
	}
	if unavailable[0].Item.ID != "b" || unavailable[0].Reason != "boom" {
		t.Fatalf("unavailable outcome = %#v, want b/boom", unavailable[0])
	}
}

func TestAggregateRecordsPerItemAvailability(t *testing.T) {
	items := []Item{{ID: "a"}, {ID: "b"}}
	result := Aggregate(context.Background(), items, func(ctx context.Context, item Item) (string, error) {
		if item.ID == "b" {
			return "", errors.New("boom")
		}
		return "value-a", nil
	})
	if len(result.Outcomes) != 2 {
		t.Fatalf("Outcomes = %#v, want two", result.Outcomes)
	}
	if result.Outcomes[0].Availability != AvailabilityAvailable || !result.Outcomes[0].IsAvailable() {
		t.Fatalf("outcome[0] = %#v, want available", result.Outcomes[0])
	}
	if result.Outcomes[1].Availability != AvailabilityUnavailable || result.Outcomes[1].IsAvailable() {
		t.Fatalf("outcome[1] = %#v, want unavailable", result.Outcomes[1])
	}
}

func TestAggregateWithEmptyItems(t *testing.T) {
	result := Aggregate(context.Background(), nil, func(ctx context.Context, item Item) (string, error) {
		t.Fatalf("processor must not run for empty items")
		return "", nil
	})
	if len(result.Outcomes) != 0 || len(result.Available()) != 0 {
		t.Fatalf("result = %#v, want empty", result)
	}
}
