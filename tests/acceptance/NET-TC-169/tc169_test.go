// Package nettc169 verifies the endpoint action surface: the network page offers
// creating, rewriting and deleting an endpoint, each submitted through the
// control-plane API, and a submission missing a required field is refused with a
// reason naming that field (NET-AC-79, NET-TC-169; NET-C-2, KOSD-C-12,
// KOSD-C-43).
package nettc169

import (
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC169EndpointActionsSubmitThroughTheControlPlane(t *testing.T) {
	facts := harness.Declarations{Capable: true, Safe: []string{"web"}}
	world := support.Start(t, facts)
	world.Application("web")
	reconciler := world.Reconciler(facts)

	// The page offers exactly three writes, all of them control-plane API paths
	// (NET-C-2, DASH-C-22).
	page := support.DeclaredPage()
	if len(page.Actions) != 3 {
		t.Fatalf("page actions = %v, want create, update and delete", page.Actions)
	}
	expected := map[string]string{
		network.ActionCreateEndpoint: http.MethodPost,
		network.ActionUpdateEndpoint: http.MethodPut,
		network.ActionDeleteEndpoint: http.MethodDelete,
	}
	for _, action := range page.Actions {
		method, known := expected[action.ID]
		if !known {
			t.Fatalf("undeclared action %q", action.ID)
		}
		if action.Method != method {
			t.Fatalf("action %s method = %s, want %s", action.ID, action.Method, method)
		}
		if !strings.HasPrefix(action.Path, "/api/v1/") {
			t.Fatalf("action %s path %q is not a control-plane API path", action.ID, action.Path)
		}
	}

	// Create a hostname endpoint from the page.
	status, body := world.CreateEndpoint("site", network.EndpointSpec{
		Kind: "http.host", TargetApp: "web", Host: "site.test",
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the host endpoint: status %d, %v", status, body)
	}
	if accepted, _ := body["accepted"].(bool); !accepted {
		t.Fatalf("create answer = %v, want an accepted write with its object", body)
	}
	world.Reconcile(reconciler, "site")
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "site", metav1.ConditionTrue)
	if host := harness.String(world.EndpointObject("site").Object, "spec", "host"); host != "site.test" {
		t.Fatalf("the created object's host = %q, want site.test", host)
	}

	// Create a path endpoint and rewrite its path; the object and its route
	// object follow the submission.
	status, body = world.CreateEndpoint("site-api", network.EndpointSpec{
		Kind: "http.path", TargetApp: "web", Host: "api.test", Path: "/v1",
	})
	if status != http.StatusAccepted {
		t.Fatalf("create the path endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site-api")
	status, body = world.UpdateEndpoint("site-api", network.EndpointSpec{
		Kind: "http.path", TargetApp: "web", Host: "api.test", Path: "/v2",
	}, world.ResourceVersion("site-api"))
	if status != http.StatusAccepted {
		t.Fatalf("rewrite the path: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site-api")
	if path := harness.String(world.EndpointObject("site-api").Object, "spec", "path"); path != "/v2" {
		t.Fatalf("the rewritten object's path = %q, want /v2", path)
	}
	ingress := harness.IngressOf(t, world.Cluster, "site-api")
	if ingress == nil || len(ingress.Spec.Rules) != 1 || len(ingress.Spec.Rules[0].HTTP.Paths) != 1 {
		t.Fatalf("the rewritten path endpoint has no route object: %+v", ingress)
	}
	if got := ingress.Spec.Rules[0].HTTP.Paths[0].Path; got != "/v2" {
		t.Fatalf("the route object's path = %q, want the submitted /v2", got)
	}

	// Delete the hostname endpoint from the page.
	status, body = world.DeleteEndpoint("site", world.ResourceVersion("site"))
	if status != http.StatusAccepted {
		t.Fatalf("delete the host endpoint: status %d, %v", status, body)
	}
	world.Reconcile(reconciler, "site")
	if status, body := world.Call(http.MethodGet, "/api/v1/resources/Endpoint/site?namespace="+harness.Namespace, nil); status != http.StatusNotFound {
		t.Fatalf("the deleted endpoint answered %d: %v", status, body)
	}

	// A submission missing a required field is refused, and the reason names
	// the field (KOSD-C-43).
	status, body = world.CreateEndpoint("broken", network.EndpointSpec{
		Kind: "http.host", TargetApp: "web",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("submit a missing host: status %d, want 400: %v", status, body)
	}
	if code := harness.String(body, "error", "code"); code != controlplane.CodeInvalidField {
		t.Fatalf("code = %s, want %s", code, controlplane.CodeInvalidField)
	}
	if message := harness.String(body, "error", "message"); !strings.Contains(message, "spec.host") {
		t.Fatalf("message %q does not name spec.host", message)
	}
}
