// Package nettc170 verifies that acknowledging a certificate expiry alert
// affects only the browser that acknowledged it: the record lives in that
// browser's storage, survives a reload, leaves another browser untouched, and
// changes neither the endpoint nor its conditions; a renewed or replaced
// certificate warns again (NET-AC-80, NET-TC-170; NET-C-3, KOSD-C-53).
package nettc170

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC170AcknowledgementStaysInTheOriginatingBrowser(t *testing.T) {
	world := support.Start(t, nil)
	world.Application("web")
	reconciler := world.Reconciler(nil)

	// One endpoint whose certificate expires inside KOSD-C-46's warning window.
	world.Endpoint("site", support.TLSShape("web", "site.test", ""))
	world.Reconcile(reconciler, "site")
	world.WriteCertificate("site", -time.Hour, 10*24*time.Hour)
	world.Reconcile(reconciler, "site")

	page := world.Page()
	alerts := support.Alerts(t, page)
	if len(alerts) != 1 || alerts[0].Name != "site" {
		t.Fatalf("alerts = %v, want the expiring certificate of site", alerts)
	}
	if alerts[0].Identity == "" {
		t.Fatal("the alert carries no certificate identity")
	}

	// The page declares the browser-local record the acknowledgement writes.
	local := support.Item(t, page, network.ItemAcknowledgements)
	if store := harness.String(local, "store"); store != network.StoreAcknowledgements {
		t.Fatalf("the acknowledgement item's store = %q, want %q", store, network.StoreAcknowledgements)
	}

	before := objectDocument(t, world)

	// One browser acknowledges the alert.
	first := support.MemStore{}
	acknowledgements := &network.Acknowledgements{Store: first}
	acknowledgements.Acknowledge(alerts[0].Identity, time.Now())
	if pending := acknowledgements.Pending(alerts); len(pending) != 0 {
		t.Fatalf("the acknowledging browser still shows %v", pending)
	}
	if _, found := first.Get(network.StoreAcknowledgements); !found {
		t.Fatal("the acknowledgement wrote no browser storage record")
	}

	// A reload keeps the record: the same browser storage, read afresh.
	reloaded := &network.Acknowledgements{Store: first}
	if pending := reloaded.Pending(alerts); len(pending) != 0 {
		t.Fatalf("the alert reappeared after a reload: %v", pending)
	}

	// Another browser starts empty and still shows the alert.
	second := support.MemStore{}
	if pending := (&network.Acknowledgements{Store: second}).Pending(alerts); len(pending) != 1 {
		t.Fatalf("the other browser shows %v, want the alert", pending)
	}

	// The page's own read is unchanged: acknowledgement is not a control-plane
	// write, so the alert list still carries the certificate.
	if alertsAgain := support.Alerts(t, world.Page()); len(alertsAgain) != 1 {
		t.Fatalf("the acknowledgment changed the page's server-side read: %v", alertsAgain)
	}

	// The endpoint and its conditions are identical field by field.
	after := objectDocument(t, world)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("acknowledging changed the endpoint:\nbefore %s\nafter  %s", before, after)
	}
	if strings.Contains(after, "acknowledg") {
		t.Fatalf("the acknowledgement reached the cluster object: %s", after)
	}

	// Replacing the certificate with another one inside the window brings the
	// warning back to the first browser: the identity moved with the certificate.
	world.WriteCertificate("site", -time.Hour, 9*24*time.Hour)
	world.Reconcile(reconciler, "site")
	replaced := support.Alerts(t, world.Page())
	if len(replaced) != 1 || replaced[0].Name != "site" {
		t.Fatalf("alerts after the replacement = %v, want the new certificate", replaced)
	}
	if replaced[0].Identity == alerts[0].Identity {
		t.Fatalf("the replaced certificate kept the identity %q", replaced[0].Identity)
	}
	if pending := reloaded.Pending(replaced); len(pending) != 1 {
		t.Fatalf("the new certificate's alert does not reappear: %v", pending)
	}
}

// objectDocument renders one endpoint as the cluster holds it, so the case can
// compare the object field by field across an acknowledgement.
func objectDocument(t *testing.T, world *support.World) string {
	t.Helper()
	encoded, err := json.Marshal(world.EndpointObject("site").Object)
	if err != nil {
		t.Fatalf("encode the endpoint: %v", err)
	}
	return string(encoded)
}
