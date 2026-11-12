// Package nettc1 verifies that the network page reads every item NET-C-1
// names: the five endpoint forms with their target application, the certificate
// state, the port ledger with its conflicts, the read-time resolution with its
// observation time, the network provider capabilities, and the read-only items
// that state why they offer no modification entry (NET-AC-1, NET-TC-1).
package nettc1

import (
	"sort"
	"testing"
	"time"

	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
	"github.com/tsic404/kos-network/tests/acceptance/support"
)

func TestTC1NetworkPageReadsEveryDeclaredItem(t *testing.T) {
	facts := harness.Declarations{Capable: true, Safe: []string{"web"}}
	world := support.Start(t, facts)
	world.Application("web")
	reconciler := world.Reconciler(facts)

	// Five endpoint forms, one per kind of KOSD-C-43, plus a TCP port reused
	// behind the API's back so the ledger has a conflict to list.
	world.Endpoint("shop", support.TLSShape("web", "shop.test", ""))
	world.Endpoint("shop-api", map[string]any{
		"kind": "http.path", "targetApp": "web", "host": "api.test", "path": "/v1",
	})
	world.Endpoint("redis-tcp", map[string]any{"kind": "l4.tcp", "targetApp": "web", "port": 8080})
	world.Endpoint("redis-udp", map[string]any{"kind": "l4.udp", "targetApp": "web", "port": 8080})
	world.Endpoint("printer", map[string]any{
		"kind": "mdns", "targetApp": "web", "serviceName": "printer", "port": 9100,
	})
	world.Endpoint("redis-tcp-dup", map[string]any{"kind": "l4.tcp", "targetApp": "web", "port": 8080})
	world.Provider("edge", "1.2.0", "network.http-endpoint", "v1")
	for _, name := range []string{"shop", "shop-api", "redis-tcp", "redis-udp", "printer", "redis-tcp-dup"} {
		world.Reconcile(reconciler, name)
	}

	// The issuer writes at the route's name; the served certificate makes the
	// certificate item observable (KOSD-C-46).
	world.WriteCertificate("shop", -time.Hour, 90*24*time.Hour)
	world.Reconcile(reconciler, "shop")

	page := world.Page()

	// Every declared item renders its declared source type (KOSD-C-53).
	for id, source := range map[string]string{
		network.ItemEndpoints:         extension.PageSourceObject,
		network.ItemCertificates:      extension.PageSourceObject,
		network.ItemCertificateAlerts: extension.PageSourceComputed,
		network.ItemLedger:            extension.PageSourceObject,
		network.ItemResolution:        extension.PageSourceProbe,
		network.ItemProviders:         extension.PageSourceObject,
		network.ItemAcknowledgements:  extension.PageSourceLocal,
	} {
		support.RequireSource(t, support.Item(t, page, id), source)
	}

	// The five forms are readable with the target application they expose.
	endpoints := support.Rows(t, page, network.ItemEndpoints, "endpoints")
	kinds := map[string]bool{}
	for _, row := range endpoints {
		kinds[harness.String(row, "kind")] = true
		if app := harness.String(row, "targetApp"); app != "web" {
			t.Fatalf("endpoint %s targets %q, want web", harness.String(row, "name"), app)
		}
	}
	want := []string{"http.host", "http.path", "l4.tcp", "l4.udp", "mdns"}
	got := make([]string, 0, len(kinds))
	for kind := range kinds {
		got = append(got, kind)
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("endpoint forms = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("endpoint forms = %v, want %v", got, want)
		}
	}

	// The certificate state is per endpoint and records the issuer in use.
	shop := support.Row(t, endpoints, "name", "shop")
	certificate, ok := shop["certificate"].(map[string]any)
	if !ok {
		t.Fatalf("shop carries no certificate state: %v", shop)
	}
	if ready, _ := certificate["ready"].(bool); !ready {
		t.Fatalf("shop certificate = %v, want ready", certificate)
	}
	if issuer := harness.String(certificate, "issuer"); issuer != endpoint.DefaultIssuer {
		t.Fatalf("shop certificate issuer = %q, want the built-in issuer %q", issuer, endpoint.DefaultIssuer)
	}
	certificates := support.Rows(t, page, network.ItemCertificates, "endpoints")
	if certificateRow := support.Row(t, certificates, "name", "shop"); certificateRow["certificate"] == nil {
		t.Fatal("the certificate item does not show the served certificate")
	}
	unserved := support.Row(t, certificates, "name", "redis-tcp")
	if unserved["certificate"] != nil {
		t.Fatalf("an endpoint without TLS carries a certificate: %v", unserved)
	}

	// The ledger lists the conflicting objects instead of letting them coexist.
	ledgerRows := support.Rows(t, page, network.ItemLedger, "projection")
	reused := support.RowRef(t, ledgerRows, "redis-tcp")
	conflicts, _ := reused["conflicts"].([]any)
	if len(conflicts) == 0 {
		t.Fatalf("the reused TCP port carries no conflict: %v", reused)
	}
	first, _ := conflicts[0].(map[string]any)
	if kind := harness.String(first, "kind"); kind != endpoint.ConflictDuplicateKey {
		t.Fatalf("conflict kind = %q, want %q", kind, endpoint.ConflictDuplicateKey)
	}
	if with, _ := first["with"].([]any); len(with) == 0 {
		t.Fatalf("the conflict lists no object: %v", first)
	}
	if note := harness.String(ValueMap(t, page, network.ItemLedger), "note"); note == "" {
		t.Fatal("the ledger item carries no shared-medium note")
	}

	// The resolution item answers a result per host with its observation time.
	resolution := ValueMap(t, page, network.ItemResolution)
	results, _ := resolution["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("probe results = %v, want one per host", results)
	}
	for _, raw := range results {
		result, _ := raw.(map[string]any)
		if addresses, _ := result["addresses"].([]any); len(addresses) != 1 {
			t.Fatalf("probe result %v carries no address", result)
		}
	}
	item := support.Item(t, page, network.ItemResolution)
	if observedAt := harness.String(item, "observedAt"); observedAt == "" {
		t.Fatal("the probe item carries no observation time")
	}
	if stale, _ := item["stale"].(bool); stale {
		t.Fatal("a read-time probe is marked stale")
	}

	// The provider item reads the registered capability and its verification.
	providers := support.Rows(t, page, network.ItemProviders, "providers")
	edge := support.Row(t, providers, "name", "edge")
	capabilities, _ := edge["capabilities"].([]any)
	if len(capabilities) != 1 {
		t.Fatalf("edge capabilities = %v, want the registered network capability", capabilities)
	}
	capability, _ := capabilities[0].(map[string]any)
	if name := harness.String(capability, "name"); name != "network.http-endpoint" {
		t.Fatalf("capability = %q, want network.http-endpoint", name)
	}
	if verified, _ := capability["verified"].(bool); !verified {
		t.Fatalf("capability = %v, want verified", capability)
	}

	// The three read-only items state why and offer no modification entry.
	for _, id := range []string{network.ItemLedger, network.ItemResolution, network.ItemProviders} {
		support.RequireReadOnly(t, support.Item(t, page, id))
	}
	// The endpoint item is modifiable: its entry is the declared update action.
	endpointItem := support.Item(t, page, network.ItemEndpoints)
	entry, ok := endpointItem["entry"].(map[string]any)
	if !ok {
		t.Fatalf("the endpoint item offers no modification entry: %v", endpointItem)
	}
	if method := harness.String(entry, "method"); method != "PUT" {
		t.Fatalf("endpoint entry method = %q, want PUT", method)
	}
	if path := harness.String(entry, "path"); path != network.EndpointUpdatePath {
		t.Fatalf("endpoint entry path = %q, want %q", path, network.EndpointUpdatePath)
	}
}

// ValueMap returns one item's value as an object.
func ValueMap(t *testing.T, page map[string]any, id string) map[string]any {
	t.Helper()
	value, ok := support.Value(t, page, id).(map[string]any)
	if !ok {
		t.Fatalf("item %q is not an object", id)
	}
	return value
}
