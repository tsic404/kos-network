// Package nettc79 verifies that deleting a system component an endpoint depends
// on is refused: the component stays visible, carries a blocked condition that
// lists every dependent, and only leaves once nothing refers to it; a component
// nothing depends on is removed. The network page reads the dependency edge from
// the endpoint: the issuer reference in its spec and the issuer its certificate
// status resolved (NET-AC-40, NET-TC-79; KOSD-C-17, KOSD-C-28, KOSD-C-46).
package nettc79

import (
	"context"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC79DependentComponentDeletionIsBlocked(t *testing.T) {
	world := support.Start(t, nil)
	world.Application("web")
	reconciler := world.Reconciler(nil)

	// Two endpoints name the issuer component in their spec; a third omits the
	// reference, so its dependency edge comes from the issuer its status
	// resolved (KOSD-C-46).
	world.Endpoint("site-a", support.TLSShape("web", "a.test", "issuer"))
	world.Endpoint("site-b", support.TLSShape("web", "b.test", "issuer"))
	world.Endpoint("auto", support.TLSShape("web", "auto.test", ""))
	for _, name := range []string{"site-a", "site-b", "auto"} {
		world.Reconcile(reconciler, name)
	}

	// The page reads the dependency edge back: the named issuer reference and
	// the issuer the status resolved.
	rows := support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints")
	for _, name := range []string{"site-a", "site-b"} {
		row := support.Row(t, rows, "name", name)
		if issuerRef := harness.String(row, "issuerRef"); issuerRef != "issuer" {
			t.Fatalf("%s issuerRef = %q, want issuer", name, issuerRef)
		}
	}
	auto := support.Row(t, rows, "name", "auto")
	certificate, _ := auto["certificate"].(map[string]any)
	if issuer := harness.String(certificate, "issuer"); issuer != endpoint.DefaultIssuer {
		t.Fatalf("auto recorded issuer %q, want the built-in issuer", issuer)
	}
	if ready, _ := certificate["ready"].(bool); ready {
		t.Fatalf("auto reports a ready certificate without an issuer writing one: %v", certificate)
	}

	// Three components: the named issuer, the built-in issuer and one nothing
	// refers to.
	for _, name := range []string{"issuer", endpoint.DefaultIssuer, "unused"} {
		world.Cluster.Create(t, componentObject(name))
	}
	world.Cluster.ExtensionState(t, world.Artifacts)
	for _, name := range []string{"issuer", endpoint.DefaultIssuer, "unused"} {
		requireFinalizer(t, world, name)
	}

	for _, name := range []string{"issuer", endpoint.DefaultIssuer, "unused"} {
		world.Cluster.DeleteComponent(t, name)
	}
	world.Cluster.ExtensionState(t, world.Artifacts)

	// The depended-on components stay and list their dependents one by one.
	requireBlocked(t, world, "issuer", "default/site-a", "default/site-b")
	requireBlocked(t, world, endpoint.DefaultIssuer, "default/auto")

	// The component nothing depends on is gone.
	eventuallyGone(t, world, "unused")

	// Removing the dependents releases the blocked components.
	for _, name := range []string{"site-a", "site-b", "auto"} {
		harness.Delete(t, world.Cluster, "Endpoint", name)
	}
	world.Cluster.ExtensionState(t, world.Artifacts)
	for _, name := range []string{"issuer", endpoint.DefaultIssuer} {
		eventuallyGone(t, world, name)
	}
}

// componentObject is a managed system component without a package, so the case
// exercises the deletion gate rather than the install pipeline.
func componentObject(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: harness.Component(name, "", "")}
}

// requireFinalizer requires the deletion gate on a component.
func requireFinalizer(t *testing.T, world *support.World, name string) {
	t.Helper()
	object := world.Cluster.Get(t, "v1beta1", "SystemComponent", name)
	for _, finalizer := range object.GetFinalizers() {
		if finalizer == extension.ComponentFinalizer {
			return
		}
	}
	t.Fatalf("component %s carries no deletion gate: %v", name, object.GetFinalizers())
}

// requireBlocked requires a component that stayed and lists its dependents.
func requireBlocked(t *testing.T, world *support.World, name string, dependents ...string) {
	t.Helper()
	object := world.Cluster.Get(t, "v1beta1", "SystemComponent", name)
	if object.GetDeletionTimestamp() == nil {
		t.Fatalf("component %s is not being deleted", name)
	}
	condition := harness.RequireCondition(t, world.Cluster, "SystemComponent", name, extension.ConditionDeprovisionBlocked)
	if condition.Status != metav1.ConditionTrue {
		t.Fatalf("%s %s = %s, want True", name, extension.ConditionDeprovisionBlocked, condition.Status)
	}
	for _, dependent := range dependents {
		if !strings.Contains(condition.Message, dependent) {
			t.Fatalf("blocked condition of %s does not list %s: %q", name, dependent, condition.Message)
		}
	}
}

// eventuallyGone requires a component to be removed once nothing blocks it.
func eventuallyGone(t *testing.T, world *support.World, name string) {
	t.Helper()
	harness.Eventually(t, 15*time.Second, "component "+name+" to be removed", func() bool {
		_, err := world.Cluster.GetObject(context.Background(), v1beta1.GroupVersion.Version, "SystemComponent", "", name)
		return apierrors.IsNotFound(err)
	})
}
