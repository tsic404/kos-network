// Package nettc78 verifies the multicast endpoint: a legal mdns entry is
// admitted, the page states the shared-medium security impact, a forbidden field
// is refused with the field named, and two entries sharing a service name and
// port conflict while the same name on another port coexists (NET-AC-39,
// NET-TC-78; KOSD-C-43, KOSD-C-44).
package nettc78

import (
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC78MulticastEntryIsAdmittedAndSharedMediaIsStated(t *testing.T) {
	world := support.Start(t, nil)
	world.Application("web")
	reconciler := world.Reconciler(nil)

	// The legal multicast entry is admitted with its service name and port.
	status, body := world.CreateEndpoint("printer", network.EndpointSpec{
		Kind: "mdns", TargetApp: "web", ServiceName: "printer", Port: 631,
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the mdns endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "printer")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "printer", metav1.ConditionTrue)

	// The page states the shared-medium security impact for the broadcast entry.
	ledger := support.Value(t, world.Page(), network.ItemLedger).(map[string]any)
	if note := harness.String(ledger, "note"); !strings.Contains(note, "共享介质") {
		t.Fatalf("the ledger note %q does not state the shared-medium impact", note)
	}
	printerRow := support.RowRef(t, support.Rows(t, world.Page(), network.ItemLedger, "projection"), "printer")
	if shared, _ := printerRow["sharedMedium"].(bool); !shared {
		t.Fatalf("the mdns row is not marked as shared medium: %v", printerRow)
	}
	if note := harness.String(printerRow, "note"); !strings.Contains(note, "共享介质") {
		t.Fatalf("the mdns row note %q does not state the security impact", note)
	}

	// A forbidden field of the shape matrix is refused, naming the field.
	for _, forbidden := range []struct {
		name  string
		spec  network.EndpointSpec
		field string
	}{
		{"mdns-host", network.EndpointSpec{Kind: "mdns", TargetApp: "web", ServiceName: "printer", Port: 631, Host: "printer.test"}, "spec.host"},
		{"mdns-tls", network.EndpointSpec{Kind: "mdns", TargetApp: "web", ServiceName: "printer", Port: 631, IssuerRef: "issuer"}, "spec.tls"},
	} {
		status, body = world.CreateEndpoint(forbidden.name, forbidden.spec)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400: %v", forbidden.name, status, body)
		}
		if code := harness.String(body, "error", "code"); code != controlplane.CodeInvalidField {
			t.Fatalf("%s code = %s, want %s", forbidden.name, code, controlplane.CodeInvalidField)
		}
		if message := harness.String(body, "error", "message"); !strings.Contains(message, forbidden.field) {
			t.Fatalf("%s message %q does not name %s", forbidden.name, message, forbidden.field)
		}
	}

	// A second entry with the same service name and port conflicts and is
	// listed; the same name on another port coexists.
	status, body = world.CreateEndpoint("printer-copy", network.EndpointSpec{
		Kind: "mdns", TargetApp: "web", ServiceName: "printer", Port: 631,
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the duplicate mdns endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "printer-copy")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "printer-copy", metav1.ConditionFalse)

	rows := support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints")
	copyRow := support.Row(t, rows, "name", "printer-copy")
	conflicts, _ := copyRow["conflicts"].([]any)
	if len(conflicts) == 0 {
		t.Fatalf("the duplicate mdns entry coexists silently: %v", copyRow)
	}
	conflict, _ := conflicts[0].(map[string]any)
	if kind := harness.String(conflict, "kind"); kind != endpoint.ConflictDuplicateKey {
		t.Fatalf("conflict kind = %q, want %q", kind, endpoint.ConflictDuplicateKey)
	}
	ledgerRows := support.Rows(t, world.Page(), network.ItemLedger, "projection")
	listed := support.RowRef(t, ledgerRows, "printer-copy")
	if conflicts, _ := listed["conflicts"].([]any); len(conflicts) == 0 {
		t.Fatalf("the ledger lists the duplicate mdns entry without a conflict: %v", listed)
	}

	status, body = world.CreateEndpoint("printer-other-port", network.EndpointSpec{
		Kind: "mdns", TargetApp: "web", ServiceName: "printer", Port: 632,
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the same name on another port: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "printer-other-port")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "printer-other-port", metav1.ConditionTrue)
}
