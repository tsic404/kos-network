package network

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tsic404/kos-sdk/packaging"
	"github.com/tsic404/kosd/pkg/extension"
)

// TestDeclarationLoads checks that the network page is accepted as declared and
// carries the sources NET-C-1 names (KOSD-C-18, DASH-C-22).
func TestDeclarationLoads(t *testing.T) {
	loaded, err := Declaration().Load()
	if err != nil {
		t.Fatalf("load declaration: %v", err)
	}
	if len(loaded.Problems) != 0 {
		t.Fatalf("problems = %+v, want none", loaded.Problems)
	}
	if loaded.Identifier != Identifier {
		t.Fatalf("identifier = %q, want %q", loaded.Identifier, Identifier)
	}
	if len(loaded.Contributions.Pages) != 1 {
		t.Fatalf("pages = %d, want the network page", len(loaded.Contributions.Pages))
	}
	page := loaded.Contributions.Pages[0]
	if page.ID != PageNetwork || page.Title == "" || page.Location == "" {
		t.Fatalf("page = %+v, want an identified, titled panel page", page)
	}
	if page.ReadOnly {
		t.Fatalf("the network page is read-only although NET-C-2 declares endpoint actions: %+v", page)
	}

	wantSources := map[string]string{
		ItemEndpoints:         extension.PageSourceObject,
		ItemCertificates:      extension.PageSourceObject,
		ItemCertificateAlerts: extension.PageSourceComputed,
		ItemLedger:            extension.PageSourceObject,
		ItemResolution:        extension.PageSourceProbe,
		ItemProviders:         extension.PageSourceObject,
		ItemAcknowledgements:  extension.PageSourceLocal,
	}
	if len(page.Items) != len(wantSources) {
		t.Fatalf("%s declares %d items, want %d", page.ID, len(page.Items), len(wantSources))
	}
	for _, item := range page.Items {
		source, known := wantSources[item.ID]
		if !known {
			t.Fatalf("unexpected item %q", item.ID)
		}
		if item.Source != source {
			t.Fatalf("item %q source = %q, want %q", item.ID, item.Source, source)
		}
		if item.Source == extension.PageSourceLocal {
			if item.Store != StoreAcknowledgements || item.Reference != "" {
				t.Fatalf("local item %q = %+v, want the browser storage key and no control-plane reference", item.ID, item)
			}
			continue
		}
		if want := Identifier + "/" + item.ID; item.Reference != want {
			t.Fatalf("item %q reference = %q, want %q", item.ID, item.Reference, want)
		}
	}

	// The three read-only items state why and offer no modification entry.
	for _, id := range []string{ItemLedger, ItemResolution, ItemProviders} {
		item := itemOf(t, page, id)
		if item.Reason == "" || item.Action != "" {
			t.Fatalf("read-only item %q = %+v, want a reason and no entry", id, item)
		}
	}

	// The endpoint actions are registered control-plane writes and nothing else.
	wantActions := map[string]string{
		ActionCreateEndpoint: http.MethodPost,
		ActionUpdateEndpoint: http.MethodPut,
		ActionDeleteEndpoint: http.MethodDelete,
	}
	if len(page.Actions) != len(wantActions) {
		t.Fatalf("actions = %v, want create, update and delete", page.Actions)
	}
	for _, action := range page.Actions {
		method, known := wantActions[action.ID]
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
}

// TestPackageArtifactCarriesTheManifestAndItsDigest checks the packaged artifact
// SDK-C-4 fixes: one manifest, one digest covering it, no asset the page does
// not declare, and a digest that moves when the manifest does.
func TestPackageArtifactCarriesTheManifestAndItsDigest(t *testing.T) {
	pkg, archive, err := Artifact()
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if pkg.Manifest.Identifier != Identifier {
		t.Fatalf("packaged identifier = %q, want %q", pkg.Manifest.Identifier, Identifier)
	}
	if pkg.Manifest.FrameworkVersion != FrameworkVersion {
		t.Fatalf("packaged frameworkVersion = %q, want %q", pkg.Manifest.FrameworkVersion, FrameworkVersion)
	}
	if len(pkg.Manifest.Permissions) != 1 || pkg.Manifest.Permissions[0].Contribution != PageNetwork {
		t.Fatalf("packaged permissions = %+v, want the network page's read scope", pkg.Manifest.Permissions)
	}
	if kinds := pkg.Manifest.Permissions[0].ReadScope.Kinds; len(kinds) != 3 {
		t.Fatalf("permission read scope = %+v, want endpoint, application and provider", kinds)
	}
	if len(pkg.Manifest.Retention) != 1 || pkg.Manifest.Retention[0].Resource != extension.ClassPages ||
		pkg.Manifest.Retention[0].Keep {
		t.Fatalf("packaged retention = %+v, want pages deleted with the component", pkg.Manifest.Retention)
	}
	if len(pkg.Assets) != 0 {
		t.Fatalf("the package carries assets a server-side page does not declare: %v", pkg.Assets)
	}
	digest := pkg.Digest.String()
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("digest = %q, want sha256:<hex>", digest)
	}

	// The install path's check accepts the artifact exactly as packed.
	verified, err := packaging.Verify(archive, pkg.Digest)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if verified.Manifest.Identifier != Identifier {
		t.Fatalf("verified identifier = %q, want %q", verified.Manifest.Identifier, Identifier)
	}

	// The digest covers the manifest: the same input packs to the same digest,
	// and a changed declaration packs to a different one.
	again, _, err := Artifact()
	if err != nil {
		t.Fatalf("repack: %v", err)
	}
	if again.Digest != pkg.Digest {
		t.Fatalf("the same declaration packed to %s and %s", pkg.Digest, again.Digest)
	}
	changed, err := Document()
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	changed["version"] = "1.0.1"
	_, changedArchive, err := packaging.Pack(changed, nil)
	if err != nil {
		t.Fatalf("pack the changed declaration: %v", err)
	}
	if string(changedArchive) == string(archive) {
		t.Fatal("a changed declaration packed to the same artifact")
	}
}

// itemOf returns one declared item by identity.
func itemOf(t *testing.T, page extension.Page, id string) extension.PageItem {
	t.Helper()
	for _, item := range page.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("page %s declares no item %q", page.ID, id)
	return extension.PageItem{}
}
