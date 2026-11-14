package network

import (
	"context"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"

	"github.com/tsic404/kosd/api"
	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/pkg/extension"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/write"
)

// Resolver is the read-time name resolver the domain probe uses (NET-C-1).
// net.Resolver implements it; a deployment may supply another one.
type Resolver interface {
	// LookupHost resolves a host name to its addresses.
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Readings resolves the items the network page declares (NET-C-1). It reads
// through the control plane's own cluster client, never through a browser
// credential: the page the console shell renders stays a control-plane
// projection (DASH-C-22).
type Readings struct {
	// Dynamic reads the cluster.
	Dynamic dynamic.Interface
	// Now supplies the read clock; defaults to time.Now.
	Now func() time.Time
	// Resolver answers the read-time probe; nil uses the process resolver.
	Resolver Resolver
}

// Read implements pages.Readings.
func (r *Readings) Read(ctx context.Context, session identity.Session, item extension.PageItem) (pages.Reading, *controlplane.Error) {
	value, failure := r.value(ctx, item)
	if failure != nil {
		return pages.Reading{}, failure
	}
	return pages.Reading{Value: value, ObservedAt: r.clock()}, nil
}

// value resolves one item from the source it declares.
func (r *Readings) value(ctx context.Context, item extension.PageItem) (any, *controlplane.Error) {
	switch item.ID {
	case ItemEndpoints:
		return r.endpoints(ctx)
	case ItemCertificates:
		return r.certificates(ctx)
	case ItemCertificateAlerts:
		return r.certificateAlerts(ctx)
	case ItemLedger:
		return r.ledgerProjection(ctx)
	case ItemResolution:
		return r.resolution(ctx)
	case ItemProviders:
		return r.providers(ctx)
	default:
		return nil, unknownItem(item.ID)
	}
}

// clock returns the moment this read observes its values (KOSD-C-13).
func (r *Readings) clock() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

// observation is one stored endpoint together with the ledger entry describing
// it.
type observation struct {
	stored v1beta1.Endpoint
	entry  endpoint.Entry
}

// observe reads every endpoint with the facts the ledger and the page judge: the
// certificate state, the target application's existence and its paused flag.
func (r *Readings) observe(ctx context.Context) ([]observation, *controlplane.Error) {
	objects, failure := r.listObjects(ctx, "Endpoint")
	if failure != nil {
		return nil, failure
	}
	paused, failure := r.applicationPause(ctx)
	if failure != nil {
		return nil, failure
	}
	observations := make([]observation, 0, len(objects))
	for _, object := range objects {
		unstructuredObject := &unstructured.Unstructured{Object: object}
		entry := endpoint.EntryOf(unstructuredObject)
		var stored v1beta1.Endpoint
		if failure := decode(object, &stored); failure != nil {
			return nil, failure
		}
		entry.Certificate = certificateOf(stored.Status.Certificate)
		target := stored.Namespace + "/" + stored.Spec.TargetApp
		if isPaused, known := paused[target]; known {
			entry.TargetPaused = isPaused
		} else {
			entry.TargetMissing = true
		}
		observations = append(observations, observation{stored: stored, entry: entry})
	}
	sort.Slice(observations, func(left, right int) bool {
		return observations[left].entry.Ref.Key() < observations[right].entry.Ref.Key()
	})
	return observations, nil
}

// ledgerOf builds the C-44 allocation projection from the observations.
func ledgerOf(observations []observation) *endpoint.Ledger {
	ledger := endpoint.NewLedger()
	entries := make([]endpoint.Entry, 0, len(observations))
	for _, observed := range observations {
		entries = append(entries, observed.entry)
	}
	ledger.Replace(entries)
	return ledger
}

// applicationPause indexes every application by namespace and name for the
// endpoint target judgement (KOSD-C-45, KOSD-C-66).
func (r *Readings) applicationPause(ctx context.Context) (map[string]bool, *controlplane.Error) {
	objects, failure := r.listObjects(ctx, "Application")
	if failure != nil {
		return nil, failure
	}
	paused := make(map[string]bool, len(objects))
	for _, object := range objects {
		var stored v1beta1.Application
		if failure := decode(object, &stored); failure != nil {
			return nil, failure
		}
		paused[stored.Namespace+"/"+stored.Name] = stored.Spec.Paused
	}
	return paused, nil
}

// listObjects reads every object of one kernel kind across namespaces.
func (r *Readings) listObjects(ctx context.Context, kind string) ([]map[string]any, *controlplane.Error) {
	resource, known := api.ResourceName(kind)
	if !known {
		return nil, internalFailure("未知的内核对象类 " + kind)
	}
	list, err := r.Dynamic.Resource(gvr(resource)).List(ctx, metav1ListOptions())
	if err != nil {
		return nil, readFailure(kind, err)
	}
	objects := make([]map[string]any, 0, len(list.Items))
	for index := range list.Items {
		objects = append(objects, list.Items[index].Object)
	}
	return objects, nil
}

// decode converts one unstructured object into its typed form.
func decode(object map[string]any, into any) *controlplane.Error {
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object, into); err != nil {
		return internalFailure("读取对象失败：" + err.Error())
	}
	return nil
}

// certificateOf converts the stored certificate state into the ledger's own
// shape (KOSD-C-46).
func certificateOf(stored *v1beta1.EndpointCertificate) *endpoint.Certificate {
	if stored == nil {
		return nil
	}
	certificate := &endpoint.Certificate{
		Issuer: stored.Issuer, Ready: stored.Ready,
		ExpiringSoon: stored.ExpiringSoon, Reason: stored.Reason,
	}
	if stored.NotAfter != nil {
		moment := stored.NotAfter.UTC()
		certificate.NotAfter = &moment
	}
	return certificate
}

// conflictView is one reason the page says an endpoint is not admitted
// (KOSD-C-44).
type conflictView struct {
	Kind string    `json:"kind"`
	Key  string    `json:"key,omitempty"`
	With []refView `json:"with,omitempty"`
}

// refView identifies one object without dragging its whole spec into a
// conflict list.
type refView struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// refsOf renders the endpoints a conflict names.
func refsOf(refs []write.Ref) []refView {
	views := make([]refView, 0, len(refs))
	for _, ref := range refs {
		views = append(views, refView{Namespace: ref.Namespace, Name: ref.Name})
	}
	return views
}

// unknownItem marks an item this product does not declare.
func unknownItem(id string) *controlplane.Error {
	return &controlplane.Error{
		Code:        controlplane.CodeCapabilityMissing,
		Message:     "kos-network 没有声明页面条目 " + id,
		Stage:       controlplane.StageCapability,
		SideEffects: controlplane.SideEffectsNone,
	}
}

// internalFailure reports a read this process could not complete.
func internalFailure(message string) *controlplane.Error {
	return &controlplane.Error{
		Code:        controlplane.CodeInternal,
		Message:     message,
		Stage:       controlplane.StageRead,
		SideEffects: controlplane.SideEffectsNone,
	}
}

// readFailure reports a cluster read the API could not answer (KOSD-C-64).
func readFailure(kind string, err error) *controlplane.Error {
	return &controlplane.Error{
		Code:        controlplane.CodeClusterUnavailable,
		Message:     "读取 " + kind + " 失败：" + err.Error(),
		Stage:       controlplane.StageRead,
		SideEffects: controlplane.SideEffectsNone,
	}
}
