// Package support boots the network-page acceptance world: one cluster, one
// composed control plane with the network package installed, and one signed-in
// Owner whose token reaches the page shell and the resource API. It exists so a
// case reads the product the way a browser does instead of reaching into
// unexported state.
package support

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
)

const (
	// OwnerName is the account the cases sign in.
	OwnerName = "owner"
	// OwnerPassword is the password the account settles on.
	OwnerPassword = "correct-horse-battery"
	// ComponentName is the system component the network package installs as.
	ComponentName = "network"
)

// World is one running acceptance world for the network page.
type World struct {
	// Cluster is the API server the objects live on.
	Cluster *harness.Cluster
	// Registry is the contribution projection the installed package publishes.
	Registry *harness.RegistrySource
	// Readings is the product reader the page shell dispatches to.
	Readings *network.Readings
	// Token is the Owner session token.
	Token string
	// Artifacts serves the packages installed in this world, so a later install
	// pass settles every component with its own artifact.
	Artifacts extension.MemoryArtifacts

	surface *harness.Identity
	t       *testing.T
}

// Start boots the acceptance world and installs the network package, so every
// case but the install one reads the product as an installed application.
func Start(t *testing.T, facts endpoint.Facts) *World {
	t.Helper()
	return boot(t, facts, true)
}

// Boot starts a cluster and the composed control plane with no system
// component installed, so a case that judges installation itself starts from
// the state a fresh desktop is in.
func Boot(t *testing.T, facts endpoint.Facts) *World {
	t.Helper()
	return boot(t, facts, false)
}

// boot builds the acceptance world; installNetwork installs this product's
// package the way a desktop install does. facts is the endpoint declaration
// seam the resource API judges http.path writes by (KOSD-C-43); nil leaves
// both declarations absent.
func boot(t *testing.T, facts endpoint.Facts, installNetwork bool) *World {
	t.Helper()
	cluster := harness.Start(t)
	registry := &harness.RegistrySource{}
	surface := harness.OpenIdentity(t, nil, func(config *identity.Config) {
		config.Preferences = registry
	})
	logs := &bytes.Buffer{}
	server, err := controlplane.New(controlplane.Config{
		Dynamic:       cluster.Dynamic,
		Extensions:    registry,
		Authorizer:    identity.Authorizer{},
		EndpointFacts: facts,
		Log:           slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("build control plane: %v", err)
	}
	t.Cleanup(server.Close)

	readings := &network.Readings{Dynamic: cluster.Dynamic, Resolver: FixedResolver{}}
	// The page's freshness bound and the certificate validity window are
	// judged against the wall clock: the certificates the cases mint are
	// relative to now, and C-13's stale bound compares two wall-clock instants.
	pagesService := pages.New(registry, pages.Config{
		Now:      time.Now,
		Readings: pages.Router{network.Identifier: readings},
	})
	pagesHandler := pages.NewAPI(pagesService).Handler()
	mux := http.NewServeMux()
	mux.Handle("/api/v1/pages", pagesHandler)
	mux.Handle("/api/v1/pages/", pagesHandler)
	mux.Handle("/", server.Handler())
	surface.Serve(t, mux)

	world := &World{
		Cluster: cluster, Registry: registry, Readings: readings,
		Artifacts: extension.MemoryArtifacts{}, surface: surface, t: t,
	}
	if installNetwork {
		world.InstallProduct()
	}

	oneTime := surface.BootstrapOwner(t, OwnerName)
	token, _ := surface.SignIn(t, OwnerName, oneTime)
	surface.ChangePassword(t, token, oneTime, OwnerPassword)
	token, _ = surface.SignIn(t, OwnerName, OwnerPassword)
	world.Token = token
	return world
}

// InstallProduct installs this repository's packaged artifact as the network
// system component and returns the install pass that judged it (KOSD-C-17,
// KOSD-AC-72). The bytes are the ones Artifact delivers, so a case exercises the
// shipped package rather than a runtime declaration.
func (w *World) InstallProduct() *extension.Installation {
	w.t.Helper()
	pkg, archive, err := network.Artifact()
	if err != nil {
		w.t.Fatalf("pack the network artifact: %v", err)
	}
	reference := "test://" + ComponentName
	w.Artifacts[reference] = archive
	w.Cluster.Create(w.t, &unstructured.Unstructured{
		Object: harness.Component(ComponentName, reference, pkg.Digest.String()),
	})
	installation := w.Cluster.ExtensionStateWith(w.t, w.Artifacts, w.surface.Service)
	w.Registry.Publish(installation.Registry())
	return installation
}

// Call sends one authenticated request to the composed control plane.
func (w *World) Call(method, path string, body any) (int, map[string]any) {
	w.t.Helper()
	return w.surface.HTTP.Call(w.t, method, path, body, map[string]string{"Authorization": "Bearer " + w.Token})
}

// Page reads the network page as the signed-in session sees it.
func (w *World) Page() map[string]any {
	w.t.Helper()
	status, body := w.Call(http.MethodGet, "/api/v1/pages/"+network.PageNetwork, nil)
	if status != http.StatusOK {
		w.t.Fatalf("read the network page: status %d, %v", status, body)
	}
	return body
}

// DeclaredPage is the page declaration the panel renders. A case takes an
// action path from it instead of calling an address the page does not offer
// (DASH-C-22).
func DeclaredPage() extension.Page {
	return network.Declaration().Contributions.Pages[0]
}

// Action resolves one declared page action into its method and path, with the
// {name} placeholder filled.
func (w *World) Action(id, name string) (string, string) {
	w.t.Helper()
	return harness.PageAction(w.t, DeclaredPage(), id, name, harness.Namespace)
}

// CreateEndpoint submits one endpoint through the declared create action.
func (w *World) CreateEndpoint(name string, spec network.EndpointSpec) (int, map[string]any) {
	w.t.Helper()
	method, path := w.Action(network.ActionCreateEndpoint, name)
	return w.Call(method, path, network.EndpointManifest(harness.Namespace, name, spec))
}

// UpdateEndpoint submits one endpoint rewrite through the declared update
// action, behind the version the page read (KOSD-C-12).
func (w *World) UpdateEndpoint(name string, spec network.EndpointSpec, resourceVersion string) (int, map[string]any) {
	w.t.Helper()
	method, path := w.Action(network.ActionUpdateEndpoint, name)
	body := network.EndpointManifest(harness.Namespace, name, spec)
	body["metadata"].(map[string]any)["resourceVersion"] = resourceVersion
	return w.Call(method, path, body)
}

// DeleteEndpoint submits one endpoint deletion through the declared delete action.
func (w *World) DeleteEndpoint(name, resourceVersion string) (int, map[string]any) {
	w.t.Helper()
	method, path := w.Action(network.ActionDeleteEndpoint, name)
	headers := map[string]string{
		"If-Match":      resourceVersion,
		"Authorization": "Bearer " + w.Token,
	}
	return w.surface.HTTP.Call(w.t, method, path, nil, headers)
}

// FixedResolver answers every probe with one address, so a read-time probe
// result is deterministic without reaching a real resolver.
type FixedResolver struct{}

// LookupHost resolves any host to one address.
func (FixedResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	return []string{"192.0.2.10"}, nil
}
