// Package network is the kos-network product surface: the network page of the
// panel, the sources its items read, the endpoint writes the page offers and
// the browser-local certificate alert acknowledgement.
//
// The page is a server-side projection (DASH-C-22, NET-C-1): the console shell
// renders it from the declaration below and the values a control-plane read
// answers. The product holds no cluster credential, writes no Ingress, Service
// or certificate, and its only write path is the control-plane API (KOSD-C-12,
// NET-C-2).
package network

import (
	"net/http"

	"github.com/tsic404/kosd/pkg/extension"
)

// Identifier is the product's extension identity across versions (KOSD-C-17).
const Identifier = "kos-network"

// Version is the package version this declaration belongs to.
const Version = "1.0.0"

// PageNetwork is the network page (NET-C-1).
const PageNetwork = "network"

// Item identities of the network page, one per source NET-C-1 names.
const (
	// ItemEndpoints shows the endpoints of the five forms and their target
	// application (object source).
	ItemEndpoints = "endpoints"
	// ItemCertificates shows the certificate state of every endpoint (object
	// source: the endpoint).
	ItemCertificates = "certificates"
	// ItemCertificateAlerts is the expiry warning computed from certificate
	// validity (computed source).
	ItemCertificateAlerts = "certificate-alerts"
	// ItemLedger is the port ledger projection (object source).
	ItemLedger = "port-ledger"
	// ItemResolution is the read-time domain probe (probe source).
	ItemResolution = "domain-resolution"
	// ItemProviders shows the network providers and their capabilities (object
	// source: the provider).
	ItemProviders = "providers"
	// ItemAcknowledgements is the browser-local record of acknowledged
	// certificate alerts (local source).
	ItemAcknowledgements = "certificate-alert-acknowledgements"
)

// Action identities of the endpoint write surface (NET-C-2).
const (
	// ActionCreateEndpoint creates an endpoint through the control plane.
	ActionCreateEndpoint = "create-endpoint"
	// ActionUpdateEndpoint rewrites an endpoint through the control plane.
	ActionUpdateEndpoint = "update-endpoint"
	// ActionDeleteEndpoint deletes an endpoint through the control plane.
	ActionDeleteEndpoint = "delete-endpoint"
)

// EndpointKind is the kind segment the control-plane resource API takes for an
// endpoint (KOSD-C-43).
const EndpointKind = "Endpoint"

// Action paths. Each one is a control-plane API path (KOSD-C-12, DASH-C-22):
// the page offers no other write path, and the browser submits with its own
// session, so the shell never proxies a native resource write. The namespace
// query names the namespace every write here lands in, which the resource
// route takes alongside the name it addresses.
const (
	EndpointCreatePath = "/api/v1/resources/" + EndpointKind + "?namespace={namespace}"
	EndpointUpdatePath = "/api/v1/resources/" + EndpointKind + "/{name}?namespace={namespace}"
	EndpointDeletePath = "/api/v1/resources/" + EndpointKind + "/{name}?namespace={namespace}"
)

// Read-only reasons. NET-C-1 requires every read-only item to state why it has
// no modification entry; the reasons name the writer that owns the value.
const (
	// LedgerReadOnlyReason explains the port ledger.
	LedgerReadOnlyReason = "端口台账是入口调和器独占写的分配投影，键由入口对象持有、删除入口才释放，页面不能修改"
	// ResolutionReadOnlyReason explains the domain probe.
	ResolutionReadOnlyReason = "域名解析是读时探测结果，只反映读取时刻的解析答案，页面不能修改"
	// ProvidersReadOnlyReason explains the provider capabilities.
	ProvidersReadOnlyReason = "网络 Provider 的能力由其组件注册与内核校验写入，网络页只读，不提供修改入口"
	// CertificatesReadOnlyReason explains the certificate state item.
	CertificatesReadOnlyReason = "证书由入口调和器按入口 spec 的签发者引用签发并写入入口状态，网络页只读"
	// AlertsReason explains why the alert list itself carries no control-plane
	// write: acknowledgment is a browser-local record (NET-C-3).
	AlertsReason = "到期提醒由证书有效期计算；确认只影响本浏览器提醒显示（浏览器本地源），不产生控制面写入"
	// AcknowledgementsReason explains the local acknowledgement record.
	AcknowledgementsReason = "浏览器本地源：确认记录只存在本浏览器，不进集群对象投影与偏好库"
)

// Declaration returns the contribution kos-network ships (KOSD-C-18): the
// network page with its items, its endpoint actions and the local
// acknowledgement record.
func Declaration() extension.Declaration {
	return extension.Declaration{
		Identifier: Identifier,
		Version:    Version,
		Contributions: extension.Contributions{
			Pages: []extension.Page{networkPage()},
		},
	}
}

// networkPage declares the network page of NET-C-1.
func networkPage() extension.Page {
	return extension.Page{
		ID:       PageNetwork,
		Title:    "网络",
		Location: "panel",
		Items: []extension.PageItem{
			// The endpoints are the page's one modifiable item: the entry of
			// the update action lets the panel rewrite a row it read.
			{ID: ItemEndpoints, Label: "入口与目标应用", Source: extension.PageSourceObject, Reference: reference(ItemEndpoints), Action: ActionUpdateEndpoint},
			{ID: ItemCertificates, Label: "证书状态", Source: extension.PageSourceObject, Reference: reference(ItemCertificates), Reason: CertificatesReadOnlyReason},
			{ID: ItemCertificateAlerts, Label: "证书到期告警", Source: extension.PageSourceComputed, Reference: reference(ItemCertificateAlerts), Reason: AlertsReason},
			{ID: ItemLedger, Label: "端口台账", Source: extension.PageSourceObject, Reference: reference(ItemLedger), Reason: LedgerReadOnlyReason},
			{ID: ItemResolution, Label: "域名解析", Source: extension.PageSourceProbe, Reference: reference(ItemResolution), Reason: ResolutionReadOnlyReason},
			{ID: ItemProviders, Label: "网络 Provider 能力", Source: extension.PageSourceObject, Reference: reference(ItemProviders), Reason: ProvidersReadOnlyReason},
			{ID: ItemAcknowledgements, Label: "本浏览器已确认的证书提醒", Source: extension.PageSourceLocal, Store: StoreAcknowledgements, Reason: AcknowledgementsReason},
		},
		Actions: []extension.Action{
			{ID: ActionCreateEndpoint, Method: http.MethodPost, Path: EndpointCreatePath},
			{ID: ActionUpdateEndpoint, Method: http.MethodPut, Path: EndpointUpdatePath},
			{ID: ActionDeleteEndpoint, Method: http.MethodDelete, Path: EndpointDeletePath},
		},
	}
}

// reference is the item reference this product owns; the page shell dispatches
// an item to the reader of the product named in front of it (DASH-C-22).
func reference(item string) string { return Identifier + "/" + item }
