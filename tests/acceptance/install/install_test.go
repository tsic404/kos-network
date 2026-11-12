// Package installtest verifies that the network system application installs and
// uninstalls as its own artifact: installing makes the network page and its
// actions appear, uninstalling removes the page and its actions while the
// endpoints, certificates and ledger data stay and another product's page is
// untouched, and reinstalling restores the page (KOSD-AC-72).
//
// KOSD-TC-155's case for the kernel's per-application install semantics lives in
// the kosd repository; this case verifies the same semantics for this
// repository's packaged artifact only.
package installtest

import (
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kos-sdk/packaging"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

// otherPageID is the page a second, unrelated product contributes, so the case
// can show that one product's removal leaves the other in place.
const otherPageID = "other"

func TestInstallAppearsAndUninstallWithdrawsOnlyThisProduct(t *testing.T) {
	world := support.Boot(t, nil)
	world.Application("web")
	reconciler := world.Reconciler(nil)
	world.Endpoint("site", support.HostShape("web", "site.test"))
	world.Reconcile(reconciler, "site")

	// The delivered artifact has the structure and fields the kernel installs
	// (KOSD-C-17, KOSD-C-18, SDK-C-3, SDK-C-4).
	pkg, archive, err := network.Artifact()
	if err != nil {
		t.Fatalf("pack the network artifact: %v", err)
	}
	verified, err := packaging.Verify(archive, pkg.Digest)
	if err != nil {
		t.Fatalf("verify the delivered artifact: %v", err)
	}
	if verified.Manifest.Identifier != network.Identifier || verified.Manifest.Version != network.Version {
		t.Fatalf("artifact identifier/version = %q/%q", verified.Manifest.Identifier, verified.Manifest.Version)
	}
	if verified.Manifest.FrameworkVersion != network.FrameworkVersion {
		t.Fatalf("artifact frameworkVersion = %q, want %q", verified.Manifest.FrameworkVersion, network.FrameworkVersion)
	}
	if len(verified.Manifest.Permissions) != 1 || verified.Manifest.Permissions[0].Contribution != network.PageNetwork {
		t.Fatalf("artifact permissions = %+v, want the network page read scope", verified.Manifest.Permissions)
	}
	if kinds := verified.Manifest.Permissions[0].ReadScope.Kinds; len(kinds) != 3 {
		t.Fatalf("artifact read scope = %+v, want endpoint, application and provider", verified.Manifest.Permissions[0].ReadScope)
	}
	if len(verified.Manifest.Retention) != 1 || verified.Manifest.Retention[0].Resource != extension.ClassPages ||
		verified.Manifest.Retention[0].Keep {
		t.Fatalf("artifact retention = %+v, want pages deleted with the component", verified.Manifest.Retention)
	}

	// A second product contributes its own page.
	other := world.Cluster.InstallPackageWith(t, "panel-other", otherDeclaration(), world.Artifacts, nil)
	world.Registry.Publish(other.Registry())

	// Installing the delivered bytes is the only install path this case uses.
	installation := world.InstallProduct()
	result, found := installation.Component(support.ComponentName)
	if !found {
		t.Fatalf("the install pass does not report component %q: %+v", support.ComponentName, installation.Components())
	}
	if !result.HasVerifiedPackage || result.Digest != pkg.Digest.String() {
		t.Fatalf("component = %+v, want the delivered artifact verified", result)
	}
	if result.FrameworkVersion != network.FrameworkVersion {
		t.Fatalf("installed frameworkVersion = %q, want %q", result.FrameworkVersion, network.FrameworkVersion)
	}
	if len(result.Retention) != 1 || result.Retention[0].Resource != extension.ClassPages {
		t.Fatalf("installed retention = %+v, want pages deleted with the component", result.Retention)
	}
	if result.Phase != extension.PhaseReady {
		t.Fatalf("component phase = %s, want %s (%s)", result.Phase, extension.PhaseReady, result.Message)
	}
	if len(result.Contributions.Pages) != 1 || result.Contributions.Pages[0].ID != network.PageNetwork {
		t.Fatalf("installed pages = %+v, want the network page", result.Contributions.Pages)
	}

	if !listed(t, world, network.PageNetwork) {
		t.Fatal("the installed network application contributes no network page")
	}
	if !listed(t, world, otherPageID) {
		t.Fatalf("the unrelated product's page %q is not listed", otherPageID)
	}
	page := world.Page()
	if rows := support.Rows(t, page, network.ItemEndpoints, "endpoints"); len(rows) != 1 {
		t.Fatalf("the network page shows %v, want the installed endpoint", rows)
	}
	if len(support.DeclaredPage().Actions) == 0 {
		t.Fatal("the installed page offers no endpoint action")
	}

	// Uninstalling this product withdraws its page and actions.
	world.Cluster.DeleteComponent(t, support.ComponentName)
	removed := world.Cluster.ExtensionState(t, world.Artifacts)
	world.Registry.Publish(removed.Registry())

	if listed(t, world, network.PageNetwork) {
		t.Fatal("the uninstalled application still contributes its page")
	}
	if status, body := world.Call(http.MethodGet, "/api/v1/pages/"+network.PageNetwork, nil); status != http.StatusNotFound {
		t.Fatalf("the withdrawn page answered %d: %v", status, body)
	}
	if !listed(t, world, otherPageID) {
		t.Fatal("uninstalling the network application withdrew another product's page")
	}

	// The endpoint, its certificate state and the ledger data stay.
	stored := world.EndpointObject("site")
	if host := harness.String(stored.Object, "spec", "host"); host != "site.test" {
		t.Fatalf("the endpoint's spec changed on uninstall: %v", stored.Object)
	}
	harness.RequireReconciled(t, world.Cluster, "Endpoint", "site", metav1.ConditionTrue)
	world.Reconcile(reconciler, "site")

	// Reinstalling the same delivered artifact restores the page with the same
	// data (KOSD-AC-72).
	reinstalled := world.InstallProduct()
	if result, found := reinstalled.Component(support.ComponentName); !found || !result.HasVerifiedPackage {
		t.Fatalf("reinstalling the delivered artifact did not verify: %+v", reinstalled.Components())
	}
	if !listed(t, world, network.PageNetwork) {
		t.Fatal("reinstalling the network application restored no page")
	}
	if rows := support.Rows(t, world.Page(), network.ItemEndpoints, "endpoints"); len(rows) != 1 {
		t.Fatalf("the reinstalled page shows %v, want the kept endpoint", rows)
	}
}

// listed reports whether the page registry lists one page.
func listed(t *testing.T, world *support.World, id string) bool {
	t.Helper()
	status, body := world.Call(http.MethodGet, "/api/v1/pages", nil)
	if status != http.StatusOK {
		t.Fatalf("read the page registry: status %d, %v", status, body)
	}
	for _, item := range harness.Items(t, body) {
		if harness.String(item, "id") == id {
			return true
		}
	}
	return false
}

// otherDeclaration is a second product contributing one page, so the case can
// tell one application's removal from another's.
func otherDeclaration() extension.Declaration {
	return extension.Declaration{
		Identifier: "panel-other",
		Version:    "1.0.0",
		Contributions: extension.Contributions{
			Pages: []extension.Page{{
				ID:             otherPageID,
				Title:          "其他",
				Location:       "panel",
				ReadOnly:       true,
				ReadOnlyReason: "本页只读：用例用来证明卸载只影响本产品",
				Items: []extension.PageItem{{
					ID: "note", Label: "说明", Source: extension.PageSourceLocal,
					Store: "panel-other.note",
				}},
			}},
		},
	}
}
