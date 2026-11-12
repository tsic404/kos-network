package network

import (
	"github.com/tsic404/kosd/api/v1beta1"
)

// EndpointSpec is the form the network page edits. It carries exactly the fields
// KOSD-C-43's matrix names; a field the chosen shape does not accept is left
// empty, and the control plane rejects the combination instead of the page
// inventing a second judgement.
type EndpointSpec struct {
	// Kind is one of http.host, http.path, l4.tcp, l4.udp, mdns.
	Kind string
	// TargetApp names the application the endpoint exposes.
	TargetApp string
	// Host is the hostname of an http.host or http.path endpoint.
	Host string
	// Path is the path prefix of an http.path endpoint.
	Path string
	// Port is the port of an l4.tcp, l4.udp or mdns endpoint.
	Port int32
	// ServiceName is the advertised name of an mdns endpoint.
	ServiceName string
	// IssuerRef names the certificate issuer of an http.host or http.path
	// endpoint; empty takes the distribution's built-in issuer (KOSD-C-46).
	IssuerRef string
	// AuthProviderRef names the forward-auth provider (KOSD-C-46).
	AuthProviderRef string
}

// EndpointManifest builds the object body one page action submits (KOSD-C-12).
// Every field the page form holds is written as the user gave it; nothing is
// defaulted, so a missing required field reaches the control plane as missing
// and is rejected there with the field named.
func EndpointManifest(namespace, name string, spec EndpointSpec) map[string]any {
	endpointSpec := map[string]any{
		"kind":      spec.Kind,
		"targetApp": spec.TargetApp,
	}
	setText(endpointSpec, "host", spec.Host)
	setText(endpointSpec, "path", spec.Path)
	setText(endpointSpec, "serviceName", spec.ServiceName)
	if spec.Port != 0 {
		endpointSpec["port"] = spec.Port
	}
	if spec.IssuerRef != "" {
		endpointSpec["tls"] = map[string]any{"issuerRef": spec.IssuerRef}
	}
	if spec.AuthProviderRef != "" {
		endpointSpec["auth"] = map[string]any{"providerRef": spec.AuthProviderRef}
	}
	return map[string]any{
		"apiVersion": v1beta1.GroupVersion.String(),
		"kind":       EndpointKind,
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       endpointSpec,
	}
}

// setText writes one optional string field.
func setText(fields map[string]any, key, value string) {
	if value != "" {
		fields[key] = value
	}
}
