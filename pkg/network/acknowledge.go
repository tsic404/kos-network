package network

import (
	"encoding/json"
	"time"
)

// StoreAcknowledgements is the browser storage key of the certificate alert
// acknowledgement record (NET-C-3). The item the page declares under
// ItemAcknowledgements names this key as its local source.
const StoreAcknowledgements = "kos-network.certificate-alert-acknowledgements"

// BrowserStore is the browser storage a local source reads and writes
// (KOSD-C-53). One store belongs to one browser: a second browser starts empty
// and never sees the first browser's record.
type BrowserStore interface {
	// Get returns the stored value and whether the key holds one.
	Get(key string) (string, bool)
	// Set writes one value.
	Set(key string, value string)
}

// Acknowledgements is the browser-local record of acknowledged certificate
// alerts (NET-C-3). Acknowledging writes this browser's storage only: it never
// reaches a cluster projection or the preference database, and it never changes
// the endpoint or its conditions.
type Acknowledgements struct {
	// Store is the browser storage of the reading browser.
	Store BrowserStore
}

// Acknowledge records that this browser has seen the alert about certificate
// identity, so that certificate's warning stops showing here.
func (a *Acknowledgements) Acknowledge(identity string, at time.Time) {
	records := a.records()
	records[identity] = at.UTC().Format(time.RFC3339Nano)
	a.write(records)
}

// Acknowledged reports whether this browser silenced the alert about identity.
func (a *Acknowledgements) Acknowledged(identity string) bool {
	_, acknowledged := a.records()[identity]
	return acknowledged
}

// Pending returns the alerts this browser still shows: every alert whose
// certificate identity it has not acknowledged. A renewed or replaced
// certificate carries a different identity, so its alert appears again even
// though the old identity stays acknowledged (NET-C-3).
func (a *Acknowledgements) Pending(alerts []Alert) []Alert {
	records := a.records()
	pending := make([]Alert, 0, len(alerts))
	for _, entry := range alerts {
		if _, acknowledged := records[entry.Identity]; acknowledged {
			continue
		}
		pending = append(pending, entry)
	}
	return pending
}

// records decodes this browser's acknowledgement map. Missing, empty or damaged
// storage reads as no acknowledgement: a browser that never acknowledged shows
// every alert.
func (a *Acknowledgements) records() map[string]string {
	records := map[string]string{}
	if a.Store == nil {
		return records
	}
	raw, found := a.Store.Get(StoreAcknowledgements)
	if !found || raw == "" {
		return records
	}
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return map[string]string{}
	}
	return records
}

// write stores the acknowledgement map back into this browser's storage.
func (a *Acknowledgements) write(records map[string]string) {
	if a.Store == nil {
		return
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		return
	}
	a.Store.Set(StoreAcknowledgements, string(encoded))
}
