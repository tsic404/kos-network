// Package nettc73 verifies the host key space of the network page's actions: a
// hostname carried by an http.host endpoint keeps every http.path on that host
// out with a reason naming the mutual exclusion, and deleting the holder
// releases the key so the path endpoint can be created (NET-AC-37, NET-TC-73).
package nettc73

import (
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC73HostAndPathKeysAreExclusiveAndReleased(t *testing.T) {
	facts := harness.Declarations{Capable: true, Safe: []string{"web"}}
	world := support.Start(t, facts)
	world.Application("web")
	reconciler := world.Reconciler(facts)

	// The hostname endpoint is created from the page and admitted.
	status, body := world.CreateEndpoint("site", network.EndpointSpec{
		Kind: "http.host", TargetApp: "web", Host: "site.test",
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the host endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "site", metav1.ConditionTrue)

	// A path endpoint on the same host is refused by the mutual exclusion; the
	// page marks it with the conflict and names the holding object.
	status, body = world.CreateEndpoint("site-path", network.EndpointSpec{
		Kind: "http.path", TargetApp: "web", Host: "site.test", Path: "/api",
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the path endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site-path")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "site-path", metav1.ConditionFalse)

	row := support.Row(t, support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints"), "name", "site-path")
	conflicts, _ := row["conflicts"].([]any)
	if len(conflicts) == 0 {
		t.Fatalf("the page shows no conflict for the path endpoint: %v", row)
	}
	conflict, _ := conflicts[0].(map[string]any)
	if kind := harness.String(conflict, "kind"); kind != endpoint.ConflictHostExclusive {
		t.Fatalf("conflict kind = %q, want %q", kind, endpoint.ConflictHostExclusive)
	}
	if key := harness.String(conflict, "key"); key != "site.test" {
		t.Fatalf("conflict key = %q, want the exclusive host", key)
	}
	with, _ := conflict["with"].([]any)
	if len(with) != 1 {
		t.Fatalf("the conflict names %v, want the holding endpoint", with)
	}
	holder, _ := with[0].(map[string]any)
	if name := harness.String(holder, "name"); name != "site" {
		t.Fatalf("the conflict names %q, want site", name)
	}

	// Deleting the holder from the page releases the key.
	status, body = world.DeleteEndpoint("site", world.ResourceVersion("site"))
	if status != http.StatusAccepted {
		t.Fatalf("delete the host endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site")
	world.Reconcile(reconciler, "site-path")

	// The path endpoint is now admitted, and creating it again succeeds: the
	// key is free.
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "site-path", metav1.ConditionTrue)
	row = support.Row(t, support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints"), "name", "site-path")
	if conflictList, present := row["conflicts"]; present && conflictList != nil {
		t.Fatalf("the released path endpoint still reports a conflict: %v", row)
	}

	status, body = world.DeleteEndpoint("site-path", world.ResourceVersion("site-path"))
	if status != http.StatusAccepted {
		t.Fatalf("delete the path endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site-path")
	status, body = world.CreateEndpoint("site-path", network.EndpointSpec{
		Kind: "http.path", TargetApp: "web", Host: "site.test", Path: "/api",
	})
	if status != http.StatusAccepted {
		t.Fatalf("recreate the path endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site-path")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "site-path", metav1.ConditionTrue)
}
