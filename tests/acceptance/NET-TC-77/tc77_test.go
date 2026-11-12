// Package nettc77 verifies that TCP and UDP ports are separate key spaces, that
// a reused port is refused or marked as a conflict instead of coexisting, and
// that deleting an endpoint releases its port for the next entry (NET-AC-39,
// NET-TC-77; KOSD-C-44).
package nettc77

import (
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC77L4PortsAreAllocatedAndReleased(t *testing.T) {
	world := support.Start(t, nil)
	world.Application("web")
	reconciler := world.Reconciler(nil)

	// One TCP and one UDP endpoint on the same port number: the two spaces do
	// not contend.
	for _, entry := range []struct {
		name string
		kind string
	}{
		{"redis-tcp", "l4.tcp"},
		{"redis-udp", "l4.udp"},
	} {
		status, body := world.CreateEndpoint(entry.name, network.EndpointSpec{
			Kind: entry.kind, TargetApp: "web", Port: 8080,
		})
		if status != http.StatusAccepted {
			t.Fatalf("create %s: status %d, %v", entry.name, status, body)
		}
		world.Reconcile(reconciler, entry.name)
		harness.RequireReconciled(t, world.Cluster, "Endpoint", entry.name, metav1.ConditionTrue)
	}
	rows := support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints")
	for _, name := range []string{"redis-tcp", "redis-udp"} {
		row := support.Row(t, rows, "name", name)
		if conflicts, present := row["conflicts"]; present && conflicts != nil {
			t.Fatalf("%s reports a conflict although the spaces differ: %v", name, row)
		}
	}

	// Reusing the occupied TCP port is marked as a conflict, not admitted.
	status, body := world.CreateEndpoint("redis-tcp-again", network.EndpointSpec{
		Kind: "l4.tcp", TargetApp: "web", Port: 8080,
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the reused TCP port: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "redis-tcp-again")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "redis-tcp-again", metav1.ConditionFalse)

	rows = support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints")
	row := support.Row(t, rows, "name", "redis-tcp-again")
	conflicts, _ := row["conflicts"].([]any)
	if len(conflicts) == 0 {
		t.Fatalf("the reused port coexists silently: %v", row)
	}
	conflict, _ := conflicts[0].(map[string]any)
	if kind := harness.String(conflict, "kind"); kind != endpoint.ConflictDuplicateKey {
		t.Fatalf("conflict kind = %q, want %q", kind, endpoint.ConflictDuplicateKey)
	}
	ledgerRow := support.RowRef(t, support.Rows(t, world.Page(), network.ItemLedger, "projection"), "redis-tcp-again")
	if listed, _ := ledgerRow["conflicts"].([]any); len(listed) == 0 {
		t.Fatalf("the ledger lists the reused port without a conflict: %v", ledgerRow)
	}

	// Deleting the holder releases the port for the next entry.
	status, body = world.DeleteEndpoint("redis-tcp", world.ResourceVersion("redis-tcp"))
	if status != http.StatusAccepted {
		t.Fatalf("delete the holder: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "redis-tcp")
	world.Reconcile(reconciler, "redis-tcp-again")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "redis-tcp-again", metav1.ConditionTrue)
}
