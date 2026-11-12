// Package nettc75 verifies that two endpoints written around the API with one
// hostname and path are both marked as conflicting and listed as conflict
// objects instead of coexisting silently (NET-AC-37, NET-TC-75; KOSD-C-44).
package nettc75

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC75DuplicatesWrittenAroundTheApiAreMarkedAndListed(t *testing.T) {
	facts := harness.Declarations{Capable: true, Safe: []string{"web"}}
	world := support.Start(t, facts)
	world.Application("web")
	reconciler := world.Reconciler(facts)

	// Both objects bypass the control plane, so nothing refuses the write and
	// only the reconciler's ledger can judge the duplicate key.
	for _, name := range []string{"dup-a", "dup-b"} {
		world.Endpoint(name, map[string]any{
			"kind": "http.path", "targetApp": "web", "host": "dup.test", "path": "/same",
		})
	}
	world.Reconcile(reconciler, "dup-a")
	world.Reconcile(reconciler, "dup-b")

	// Neither object may be admitted silently.
	for _, name := range []string{"dup-a", "dup-b"} {
		harness.RequireReconciled(t, world.Cluster, "Endpoint", name, metav1.ConditionFalse)
		harness.RequireCondition(t, world.Cluster, "Endpoint", name, endpoint.ConditionConflict)
	}

	// The page marks both objects and names the peer each conflicts with.
	rows := support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints")
	for name, peer := range map[string]string{"dup-a": "dup-b", "dup-b": "dup-a"} {
		row := support.Row(t, rows, "name", name)
		conflicts, _ := row["conflicts"].([]any)
		if len(conflicts) == 0 {
			t.Fatalf("%s is marked with no conflict: %v", name, row)
		}
		conflict, _ := conflicts[0].(map[string]any)
		if kind := harness.String(conflict, "kind"); kind != endpoint.ConflictDuplicateKey {
			t.Fatalf("%s conflict kind = %q, want %q", name, kind, endpoint.ConflictDuplicateKey)
		}
		with, _ := conflict["with"].([]any)
		if len(with) != 1 {
			t.Fatalf("%s conflict names %v, want the peer alone", name, with)
		}
		named, _ := with[0].(map[string]any)
		if got := harness.String(named, "name"); got != peer {
			t.Fatalf("%s conflict names %q, want %q", name, got, peer)
		}
	}

	// The ledger projection lists both conflict objects; no row coexists
	// silently.
	ledgerRows := support.Rows(t, world.Page(), network.ItemLedger, "projection")
	if len(ledgerRows) != 2 {
		t.Fatalf("ledger rows = %v, want both duplicate objects", ledgerRows)
	}
	for _, name := range []string{"dup-a", "dup-b"} {
		row := support.RowRef(t, ledgerRows, name)
		conflicts, _ := row["conflicts"].([]any)
		if len(conflicts) == 0 {
			t.Fatalf("the ledger lists %s without a conflict: %v", name, row)
		}
	}
}
