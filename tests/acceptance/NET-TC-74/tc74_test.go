// Package nettc74 verifies that an endpoint's target application is immutable,
// that the reverse lookup lists exactly the endpoints exposing one application,
// and that deleting the application removes the taken-over endpoints while the
// unmanaged one stays visible and is marked as pointing at a missing
// application (NET-AC-37, NET-TC-74; KOSD-C-45).
package nettc74

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC74TargetAppIsFixedAndReverseLookupMatchesDeletion(t *testing.T) {
	world := support.Start(t, nil)
	world.Application("web")
	world.Application("other")
	reconciler := world.Reconciler(nil)

	// One endpoint created from the page, one taken-over endpoint created
	// natively, and one KOS never adopted.
	status, body := world.CreateEndpoint("site", network.EndpointSpec{
		Kind: "http.host", TargetApp: "web", Host: "site.test",
	})
	if status != http.StatusAccepted {
		t.Fatalf("create site: status %d, %v", status, body)
	}
	world.Endpoint("extra", support.HostShape("other", "extra.test"))
	legacy := harness.Endpoint("legacy", nil, support.HostShape("web", "legacy.test"))
	world.Cluster.Create(t, legacy)
	for _, name := range []string{"site", "extra", "legacy"} {
		world.Reconcile(reconciler, name)
	}
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "site", metav1.ConditionTrue)
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "extra", metav1.ConditionTrue)

	// The reverse lookup lists the endpoints that expose one application,
	// taken over or not.
	if got := reverse(t, world, "web"); !equal(got, []string{"legacy", "site"}) {
		t.Fatalf("reverse lookup of web = %v, want [legacy site]", got)
	}
	if got := reverse(t, world, "other"); !equal(got, []string{"extra"}) {
		t.Fatalf("reverse lookup of other = %v, want [extra]", got)
	}

	// Rewriting the target application is refused, and the page surfaces the
	// refusal with the field it names.
	status, body = world.UpdateEndpoint("site", network.EndpointSpec{
		Kind: "http.host", TargetApp: "other", Host: "site.test",
	}, world.ResourceVersion("site"))
	if status != http.StatusBadRequest {
		t.Fatalf("rewrite the target application: status %d, want 400: %v", status, body)
	}
	if code := harness.String(body, "error", "code"); code != controlplane.CodeInvalidField {
		t.Fatalf("code = %s, want %s", code, controlplane.CodeInvalidField)
	}
	if message := harness.String(body, "error", "message"); !strings.Contains(message, "spec.targetApp") {
		t.Fatalf("message %q does not name the field", message)
	}
	if kept := harness.String(world.EndpointObject("site").Object, "spec", "targetApp"); kept != "web" {
		t.Fatalf("targetApp = %q after a refused write, want web", kept)
	}

	// Deleting the application removes the taken-over endpoints and keeps the
	// unmanaged one, which the page marks as pointing at a missing application.
	harness.Delete(t, world.Cluster, "Application", "web")
	actions := endpoint.NewActions(world.Cluster.Writer())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := actions.DeleteForApplication(ctx, endpoint.ApplicationRef(harness.Namespace, "web"))
	if err != nil {
		t.Fatalf("cascade: %v", err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].Name != "site" {
		t.Fatalf("deleted = %v, want site", result.Deleted)
	}
	if len(result.Retained) != 1 || result.Retained[0].Name != "legacy" {
		t.Fatalf("retained = %v, want legacy", result.Retained)
	}
	world.Reconcile(reconciler, "site")
	world.ReconcileKind(reconciler, "Application", "web")
	if got := reverse(t, world, "web"); !equal(got, []string{"legacy"}) {
		t.Fatalf("reverse lookup after deletion = %v, want [legacy]", got)
	}

	// The page lists the two survivors, the endpoint of the application that
	// still exists and the unmanaged one, and marks the latter with the
	// missing-target conflict while leaving its spec and markers untouched.
	rows := support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints")
	if len(rows) != 2 {
		t.Fatalf("endpoints after the cascade = %v, want the survivors extra and legacy", rows)
	}
	legacyRow := support.Row(t, rows, "name", "legacy")
	support.Row(t, rows, "name", "extra")
	conflicts, _ := legacyRow["conflicts"].([]any)
	if len(conflicts) != 1 {
		t.Fatalf("legacy conflicts = %v, want a missing target", legacyRow)
	}
	conflict, _ := conflicts[0].(map[string]any)
	if kind := harness.String(conflict, "kind"); kind != endpoint.ConflictTargetMissing {
		t.Fatalf("conflict kind = %q, want %q", kind, endpoint.ConflictTargetMissing)
	}
	stored := world.EndpointObject("legacy")
	if stored.GetLabels() != nil {
		t.Fatalf("the unmanaged endpoint gained markers: %v", stored.GetLabels())
	}
	if kept := harness.String(stored.Object, "spec", "targetApp"); kept != "web" {
		t.Fatalf("the unmanaged endpoint spec changed: %v", harness.String(stored.Object, "spec"))
	}
}

// reverse asks the control plane which endpoints expose one application.
func reverse(t *testing.T, world *support.World, app string) []string {
	t.Helper()
	status, body := world.Call(http.MethodGet,
		"/api/v1/resources/Endpoint?namespace="+harness.Namespace+"&targetApp="+app, nil)
	if status != http.StatusOK {
		t.Fatalf("reverse lookup status = %d: %v", status, body)
	}
	names := make([]string, 0)
	for _, item := range harness.Items(t, body) {
		names = append(names, harness.String(item, "ref", "name"))
	}
	sort.Strings(names)
	return names
}

// equal compares two string lists.
func equal(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
