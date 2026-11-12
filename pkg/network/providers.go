package network

import (
	"context"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
)

// NetworkCapabilityPrefix selects the providers the network page shows: a
// provider belongs here when it registers at least one network capability.
const NetworkCapabilityPrefix = "network."

// capabilityView is one capability of a provider: the registered declaration and
// the verification result of the effective set (KOSD-C-47).
type capabilityView struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Scope      string   `json:"scope,omitempty"`
	Verified   bool     `json:"verified"`
	State      string   `json:"state,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Guarantees []string `json:"guarantees,omitempty"`
}

// providerView is one network provider as the page shows it: the registering
// component, the declared version and capabilities, and the conditions the
// validator writes (NET-C-1, KOSD-C-47).
type providerView struct {
	Name           string            `json:"name"`
	ComponentRef   string            `json:"componentRef,omitempty"`
	Version        string            `json:"version,omitempty"`
	StorageClasses []string          `json:"storageClasses,omitempty"`
	Capabilities   []capabilityView  `json:"capabilities"`
	Conditions     map[string]string `json:"conditions,omitempty"`
	Phase          string            `json:"phase,omitempty"`
	Health         string            `json:"health,omitempty"`
}

// providers reads the network providers and their capabilities. The item is
// read-only: registration is the component registration path and verification
// belongs to the kernel validator, so the page offers no modification entry
// (NET-C-1, KOSD-C-47).
func (r *Readings) providers(ctx context.Context) (any, *controlplane.Error) {
	objects, failure := r.listObjects(ctx, "Provider")
	if failure != nil {
		return nil, failure
	}
	views := make([]providerView, 0, len(objects))
	for _, object := range objects {
		var stored v1beta1.Provider
		if failure := decode(object, &stored); failure != nil {
			return nil, failure
		}
		view := providerViewOf(stored)
		if len(view.Capabilities) == 0 {
			continue
		}
		views = append(views, view)
	}
	sort.Slice(views, func(left, right int) bool { return views[left].Name < views[right].Name })
	return map[string]any{"providers": views}, nil
}

// providerViewOf renders one stored provider, keeping only the network
// capabilities.
func providerViewOf(stored v1beta1.Provider) providerView {
	results := make(map[string]v1beta1.ProviderCapabilityResult, len(stored.Status.Capabilities))
	for _, result := range stored.Status.Capabilities {
		results[result.Name+"@"+result.Version] = result
	}
	view := providerView{
		Name:           stored.Name,
		ComponentRef:   stored.Spec.ComponentRef,
		Version:        stored.Spec.Version,
		StorageClasses: stored.Spec.StorageClasses,
		Phase:          stored.Status.Phase,
		Health:         string(stored.Status.Health.State),
	}
	for _, capability := range stored.Spec.Capabilities {
		if !strings.HasPrefix(capability.Name, NetworkCapabilityPrefix) {
			continue
		}
		entry := capabilityView{
			Name: capability.Name, Version: capability.Version,
			Scope: string(capability.Scope), Guarantees: guaranteeNames(capability.Guarantees),
		}
		if result, known := results[capability.Name+"@"+capability.Version]; known {
			entry.Verified = result.State == "Verified"
			entry.State = result.State
			entry.Reason = result.Reason
		} else if containsCapability(stored.Status.EffectiveCapabilities, capability.Name, capability.Version) {
			entry.State = "Registered"
		}
		view.Capabilities = append(view.Capabilities, entry)
	}
	view.Conditions = conditionsOf(stored.Status.Conditions)
	return view
}

// guaranteeNames renders the guarantee keys a capability promises.
func guaranteeNames(guarantees []v1beta1.CapabilityGuarantee) []string {
	names := make([]string, 0, len(guarantees))
	for _, guarantee := range guarantees {
		names = append(names, guarantee.Key)
	}
	sort.Strings(names)
	return names
}

// containsCapability reports whether the effective set carries one capability.
func containsCapability(capabilities []v1beta1.ProviderCapability, name, version string) bool {
	for _, capability := range capabilities {
		if capability.Name == name && capability.Version == version {
			return true
		}
	}
	return false
}

// conditionsOf renders the conditions as type to status, so a page can read the
// Registered, Verified, UpgradeBlocked and Ready conditions (KOSD-C-47).
func conditionsOf(conditions []metav1.Condition) map[string]string {
	if len(conditions) == 0 {
		return nil
	}
	rendered := make(map[string]string, len(conditions))
	for _, condition := range conditions {
		rendered[condition.Type] = string(condition.Status)
	}
	return rendered
}
