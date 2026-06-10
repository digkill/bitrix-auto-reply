package bitrix

import (
	"encoding/json"
	"testing"
)

func TestRecentItemIDAcceptsNumber(t *testing.T) {
	body := []byte(`{"id":160591,"type":"user"}`)

	var item RecentItem
	if err := json.Unmarshal(body, &item); err != nil {
		t.Fatalf("unmarshal recent item: %v", err)
	}

	if item.ID.String() != "160591" {
		t.Fatalf("expected id 160591, got %q", item.ID.String())
	}
}

func TestRecentItemIDAcceptsString(t *testing.T) {
	body := []byte(`{"id":"chat42","type":"chat"}`)

	var item RecentItem
	if err := json.Unmarshal(body, &item); err != nil {
		t.Fatalf("unmarshal recent item: %v", err)
	}

	if item.ID.String() != "chat42" {
		t.Fatalf("expected id chat42, got %q", item.ID.String())
	}
}
