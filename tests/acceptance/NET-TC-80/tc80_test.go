// Package nettc80 verifies that the certificate is a first-class item of the
// network page: a served certificate and an unserved one are distinguishable,
// the issuer in use is recorded, and a certificate inside KOSD-C-46's 14-day
// window produces an alert that points at the endpoint (NET-AC-40, NET-TC-80).
package nettc80

import (
	"testing"
	"time"

	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC80CertificateStateAndExpiryAlert(t *testing.T) {
	world := support.Start(t, nil)
	world.Application("web")
	reconciler := world.Reconciler(nil)

	// A served certificate outside the window, one inside it, and one whose
	// issuer wrote nothing yet.
	world.Endpoint("served", support.TLSShape("web", "served.test", "internal"))
	world.Endpoint("expiring", support.TLSShape("web", "expiring.test", ""))
	world.Endpoint("pending", support.TLSShape("web", "pending.test", ""))
	for _, name := range []string{"served", "expiring", "pending"} {
		world.Reconcile(reconciler, name)
	}
	world.WriteCertificate("served", -time.Hour, 90*24*time.Hour)
	world.WriteCertificate("expiring", -time.Hour, 10*24*time.Hour)
	for _, name := range []string{"served", "expiring", "pending"} {
		world.Reconcile(reconciler, name)
	}

	page := world.Page()

	// The certificate item distinguishes ready from unready and records the
	// issuer in use.
	rows := support.Rows(t, page, network.ItemCertificates, "endpoints")
	served := support.Row(t, rows, "name", "served")
	servedCertificate, _ := served["certificate"].(map[string]any)
	if ready, _ := servedCertificate["ready"].(bool); !ready {
		t.Fatalf("served certificate = %v, want ready", servedCertificate)
	}
	if issuer := harness.String(servedCertificate, "issuer"); issuer != "internal" {
		t.Fatalf("served certificate issuer = %q, want internal", issuer)
	}
	pending := support.Row(t, rows, "name", "pending")
	pendingCertificate, _ := pending["certificate"].(map[string]any)
	if ready, _ := pendingCertificate["ready"].(bool); ready {
		t.Fatalf("an endpoint whose issuer wrote nothing reports ready: %v", pendingCertificate)
	}
	if issuer := harness.String(pendingCertificate, "issuer"); issuer != endpoint.DefaultIssuer {
		t.Fatalf("pending certificate issuer = %q, want the built-in issuer", issuer)
	}
	if reason := harness.String(pendingCertificate, "reason"); reason == "" {
		t.Fatal("the unready certificate states no reason")
	}

	// The alert item is computed from certificate validity: the 14-day window
	// names the entry inside it and leaves the 90-day certificate alone.
	alerts := support.Alerts(t, page)
	expiring := alertOf(t, alerts, "expiring")
	if !expiring.ExpiringSoon {
		t.Fatalf("the alert = %v, want an expiring certificate", expiring)
	}
	if expiring.Namespace != harness.Namespace || expiring.Name != "expiring" {
		t.Fatalf("the alert points at %s/%s, want %s/expiring", expiring.Namespace, expiring.Name, harness.Namespace)
	}
	if expiring.Identity == "" {
		t.Fatal("the alert carries no certificate identity")
	}
	for _, name := range []string{"served"} {
		for _, alert := range alerts {
			if alert.Name == name {
				t.Fatalf("%s carries an alert %v although its certificate is outside the window", name, alert)
			}
		}
	}

	// The warning window the page states is KOSD-C-46's 14 days.
	value := support.Value(t, page, network.ItemCertificateAlerts).(map[string]any)
	if window := harness.String(value, "window"); window != (14 * 24 * time.Hour).String() {
		t.Fatalf("alert window = %q, want %q", window, (14 * 24 * time.Hour).String())
	}
	if expiring.NotAfter == "" {
		t.Fatal("the alert names no expiry")
	}
}

// alertOf returns the alert of one endpoint.
func alertOf(t *testing.T, alerts []network.Alert, name string) network.Alert {
	t.Helper()
	for _, alert := range alerts {
		if alert.Name == name {
			return alert
		}
	}
	t.Fatalf("no alert for %s: %v", name, alerts)
	return network.Alert{}
}
