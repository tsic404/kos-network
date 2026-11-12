package support

import (
	"encoding/json"
	"testing"

	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
)

// Item returns one rendered page item by identity.
func Item(t *testing.T, page map[string]any, id string) map[string]any {
	t.Helper()
	for _, item := range harness.Items(t, page) {
		if harness.String(item, "id") == id {
			return item
		}
	}
	t.Fatalf("the page declares no item %q; items = %v", id, harness.Items(t, page))
	return nil
}

// Value returns one item's current value.
func Value(t *testing.T, page map[string]any, id string) any {
	t.Helper()
	item := Item(t, page, id)
	value, present := item["value"]
	if !present {
		t.Fatalf("item %q carries no value: %v", id, item)
	}
	return value
}

// Rows returns the rows one item's value carries under key.
func Rows(t *testing.T, page map[string]any, id, key string) []map[string]any {
	t.Helper()
	value, ok := Value(t, page, id).(map[string]any)
	if !ok {
		t.Fatalf("item %q is not an object: %v", id, Value(t, page, id))
	}
	rows, ok := value[key].([]any)
	if !ok {
		t.Fatalf("item %q carries no row list %q: %v", id, key, value)
	}
	decoded := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		record, ok := row.(map[string]any)
		if !ok {
			t.Fatalf("item %q row %v is not an object", id, row)
		}
		decoded = append(decoded, record)
	}
	return decoded
}

// Row returns the row whose field equals value.
func Row(t *testing.T, rows []map[string]any, field, value string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if harness.String(row, field) == value {
			return row
		}
	}
	t.Fatalf("no row carries %s = %q; rows = %v", field, value, rows)
	return nil
}

// RequireReadOnly requires an item that states why it has no modification entry.
func RequireReadOnly(t *testing.T, item map[string]any) {
	t.Helper()
	if reason := harness.String(item, "reason"); reason == "" {
		t.Fatalf("read-only item %q states no reason", harness.String(item, "id"))
	}
	if entry, present := item["entry"]; present && entry != nil {
		t.Fatalf("read-only item %q offers a modification entry: %v", harness.String(item, "id"), entry)
	}
	if action := harness.String(item, "action"); action != "" {
		t.Fatalf("read-only item %q names an action: %s", harness.String(item, "id"), action)
	}
}

// RequireSource requires an item's declared source type (KOSD-C-53).
func RequireSource(t *testing.T, item map[string]any, source string) {
	t.Helper()
	if got := harness.String(item, "source"); got != source {
		t.Fatalf("item %q source = %q, want %q", harness.String(item, "id"), got, source)
	}
}

// Alerts decodes the certificate alert rows of the page into the product's own
// type, so a case reads the alert contract rather than map fields.
func Alerts(t *testing.T, page map[string]any) []network.Alert {
	t.Helper()
	encoded, err := json.Marshal(Rows(t, page, network.ItemCertificateAlerts, "alerts"))
	if err != nil {
		t.Fatalf("encode alerts: %v", err)
	}
	var alerts []network.Alert
	if err := json.Unmarshal(encoded, &alerts); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	return alerts
}

// MemStore is one browser's local storage (KOSD-C-53). Two MemStore values are
// two browsers: neither sees the other's record.
type MemStore map[string]string

// Get returns the stored value and whether the key holds one.
func (m MemStore) Get(key string) (string, bool) {
	value, found := m[key]
	return value, found
}

// Set writes one value.
func (m MemStore) Set(key, value string) { m[key] = value }

// RowRef returns the row whose object reference names value; the ledger
// projection identifies its rows by reference, not by a top-level name.
func RowRef(t *testing.T, rows []map[string]any, value string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if harness.String(row, "ref", "name") == value {
			return row
		}
	}
	t.Fatalf("no row references %q; rows = %v", value, rows)
	return nil
}
