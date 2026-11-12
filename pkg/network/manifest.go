package network

import (
	"encoding/json"
	"fmt"

	"github.com/tsic404/kos-sdk/extension"
	"github.com/tsic404/kos-sdk/manifest"
	"github.com/tsic404/kos-sdk/packaging"
	"github.com/tsic404/kosd/api"
	kosd "github.com/tsic404/kosd/pkg/extension"
)

// FrameworkVersion is the framework API version this product targets
// (KOSD-C-16).
const FrameworkVersion = "1.1"

// Manifest returns the author-facing declaration kos-sdk validates and packs
// (KOSD-C-18, SDK-C-3). It is the distribution form of the same contribution
// Declaration describes to the control plane, so the two are built from one
// page declaration and cannot drift.
func Manifest() (manifest.ExtensionManifest, error) {
	page, err := pagePayload(networkPage())
	if err != nil {
		return manifest.ExtensionManifest{}, err
	}
	return manifest.ExtensionManifest{
		Identifier:       Identifier,
		Version:          Version,
		FrameworkVersion: FrameworkVersion,
		Contributions: manifest.Contributions{
			Pages: []map[string]any{page},
		},
		Dependencies: manifest.Dependencies{},
		// The page reads the endpoint, application and provider classes to
		// render NET-C-1; it serves no class of its own, so no writer list.
		Permissions: []manifest.ExtensionPermission{{
			Contribution: PageNetwork,
			ReadScope: manifest.ReadScope{
				Groups: []string{api.Group},
				Kinds: []extension.GroupKind{
					{Group: api.Group, Kind: "Endpoint"},
					{Group: api.Group, Kind: "Application"},
					{Group: api.Group, Kind: "Provider"},
				},
			},
		}},
		Configuration: manifest.Configuration{},
		// The page is a contribution, not data: uninstalling this component
		// withdraws it and keeps the endpoints, certificates and ledger the
		// kernel owns (KOSD-C-18, KOSD-AC-72).
		Retention: []manifest.Retention{{Resource: kosd.ClassPages, Keep: false}},
	}, nil
}

// Document returns the manifest as the decoded JSON document kos-sdk validates
// and packs (SDK-C-3).
func Document() (map[string]any, error) {
	declaration, err := Manifest()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(declaration)
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	document := map[string]any{}
	if err := json.Unmarshal(encoded, &document); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return document, nil
}

// Artifact packs the product into the installable artifact of SDK-C-4: one
// gzipped tar carrying the manifest under manifest.json and the digest that
// covers it, which is the structure the kernel installs (KOSD-C-17). The page
// is a server-side projection rendered by the console shell, so the package
// carries no asset and the digest covers the manifest alone.
func Artifact() (packaging.Package, []byte, error) {
	document, err := Document()
	if err != nil {
		return packaging.Package{}, nil, err
	}
	return packaging.Pack(document, nil)
}

// pagePayload renders one declared page into the manifest payload form
// (DASH-C-22): the page shape the console shell renders.
func pagePayload(page kosd.Page) (map[string]any, error) {
	encoded, err := json.Marshal(page)
	if err != nil {
		return nil, fmt.Errorf("encode page %s: %w", page.ID, err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, fmt.Errorf("decode page %s: %w", page.ID, err)
	}
	return payload, nil
}
