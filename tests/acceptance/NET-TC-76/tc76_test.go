// Package nettc76 verifies the two declarations an http.path endpoint needs: the
// path-prefix capability and a package that declares path-prefix use safe. A
// page write missing either is refused with a reason naming it, and an admitted
// path endpoint produces the route object KOSD-C-43 fixes (NET-AC-38,
// NET-TC-76).
package nettc76

import (
	"net/http"
	"strings"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/pkg/markers"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC76PathEndpointNeedsCapabilityAndDeclaration(t *testing.T) {
	facts := harness.Declarations{Capable: true, Safe: []string{"safe"}}
	world := support.Start(t, facts)
	world.Application("safe")
	world.Application("unsafe")
	reconciler := world.Reconciler(facts)

	// The declared package is accepted from the page and the endpoint is
	// admitted.
	status, body := world.CreateEndpoint("safe-path", network.EndpointSpec{
		Kind: "http.path", TargetApp: "safe", Host: "safe.test", Path: "/safe",
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the declared path endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "safe-path")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "safe-path", metav1.ConditionTrue)
	requireIngress(t, world, "safe-path", "safe.test", "/safe", "safe")

	// The page shows the admitted endpoint without a conflict.
	row := support.Row(t, support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints"), "name", "safe-path")
	if conflicts, present := row["conflicts"]; present && conflicts != nil {
		t.Fatalf("the admitted path endpoint reports a conflict: %v", row)
	}

	// An undeclared package is refused, and the reason points at the field.
	status, body = world.CreateEndpoint("unsafe-path", network.EndpointSpec{
		Kind: "http.path", TargetApp: "unsafe", Host: "unsafe.test", Path: "/unsafe",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("create the undeclared path endpoint: status %d, want 400: %v", status, body)
	}
	if code := harness.String(body, "error", "code"); code != controlplane.CodeInvalidField {
		t.Fatalf("code = %s, want %s", code, controlplane.CodeInvalidField)
	}
	if message := harness.String(body, "error", "message"); !strings.Contains(message, "spec.path") || !strings.Contains(message, "未声明") {
		t.Fatalf("message %q does not name the missing declaration", message)
	}

	// Deleting the endpoint removes its route object.
	status, body = world.DeleteEndpoint("safe-path", world.ResourceVersion("safe-path"))
	if status != http.StatusAccepted {
		t.Fatalf("delete the path endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "safe-path")
	if ingress := harness.IngressOf(t, world.Cluster, "safe-path"); ingress != nil {
		t.Fatal("the route object survived its endpoint")
	}
}

// TestTC76MissingCapabilityIsRefused covers the other half of the declaration
// pair: a cluster without the path-prefix capability refuses the write and
// names the capability.
func TestTC76MissingCapabilityIsRefused(t *testing.T) {
	world := support.Start(t, harness.Declarations{Capable: false, Safe: []string{"safe"}})
	world.Application("safe")

	status, body := world.CreateEndpoint("no-capability", network.EndpointSpec{
		Kind: "http.path", TargetApp: "safe", Host: "cap.test", Path: "/cap",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("create without the capability: status %d, want 400: %v", status, body)
	}
	if message := harness.String(body, "error", "message"); !strings.Contains(message, endpoint.PathPrefixCapability) {
		t.Fatalf("message %q does not name %s", message, endpoint.PathPrefixCapability)
	}
}

// requireIngress asserts the route facts NET-TC-76's 「路由可达」 names.
func requireIngress(t *testing.T, world *support.World, name, host, path, app string) {
	t.Helper()
	ingress := harness.IngressOf(t, world.Cluster, name)
	if ingress == nil {
		t.Fatalf("%s has no route object", name)
	}
	endpoint := world.EndpointObject(name)
	if got := ingress.Labels[markers.OwnerUID]; got != string(endpoint.GetUID()) {
		t.Fatalf("ingress owner-uid = %q, want the endpoint UID %q", got, endpoint.GetUID())
	}
	if ingress.Labels[markers.ManagedBy] == "" {
		t.Fatal("the ingress carries no managed-by marker")
	}
	if len(ingress.Spec.Rules) != 1 || ingress.Spec.Rules[0].Host != host {
		t.Fatalf("rules = %v, want one rule for %s", ingress.Spec.Rules, host)
	}
	paths := ingress.Spec.Rules[0].HTTP.Paths
	if len(paths) != 1 {
		t.Fatalf("paths = %v, want one", paths)
	}
	if paths[0].Path != path || paths[0].PathType == nil || *paths[0].PathType != networkingv1.PathTypePrefix {
		t.Fatalf("path = %v, want a %s prefix", paths[0], path)
	}
	backend := paths[0].Backend.Service
	if backend == nil || backend.Name != app || backend.Port.Name != "http" {
		t.Fatalf("backend = %v, want service %s port http", backend, app)
	}
}
