package network

import (
	"context"
	"time"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
)

// certificateView is the observed certificate state of one endpoint (KOSD-C-46).
type certificateView struct {
	Issuer       string `json:"issuer,omitempty"`
	Ready        bool   `json:"ready"`
	NotAfter     string `json:"notAfter,omitempty"`
	ExpiringSoon bool   `json:"expiringSoon"`
	Reason       string `json:"reason,omitempty"`
}

// endpointView is one endpoint as the network page shows it (NET-C-1): the form
// and target application of every entry, its admission state and the conflicts
// that keep it from being admitted (KOSD-C-43, KOSD-C-44).
type endpointView struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// ResourceVersion is the precondition an update or delete carries
	// (KOSD-C-12): the browser reads it here and sends it back unchanged.
	ResourceVersion string           `json:"resourceVersion"`
	Kind            string           `json:"kind"`
	TargetApp       string           `json:"targetApp"`
	Host            string           `json:"host,omitempty"`
	Path            string           `json:"path,omitempty"`
	Port            int32            `json:"port,omitempty"`
	ServiceName     string           `json:"serviceName,omitempty"`
	IssuerRef       string           `json:"issuerRef,omitempty"`
	AuthProvider    string           `json:"authProviderRef,omitempty"`
	Phase           string           `json:"phase,omitempty"`
	Health          string           `json:"health,omitempty"`
	Certificate     *certificateView `json:"certificate,omitempty"`
	Conflicts       []conflictView   `json:"conflicts,omitempty"`
}

// endpoints reads the five endpoint forms with their target application and
// admission state (NET-C-1).
func (r *Readings) endpoints(ctx context.Context) (any, *controlplane.Error) {
	observations, failure := r.observe(ctx)
	if failure != nil {
		return nil, failure
	}
	ledger := ledgerOf(observations)
	views := make([]endpointView, 0, len(observations))
	for _, observed := range observations {
		view := endpointViewOf(observed)
		for _, conflict := range ledger.Conflicts(observed.entry.Ref) {
			view.Conflicts = append(view.Conflicts, conflictView{Kind: conflict.Kind, Key: conflict.Key, With: refsOf(conflict.With)})
		}
		views = append(views, view)
	}
	return map[string]any{"endpoints": views}, nil
}

// endpointViewOf renders one observed endpoint.
func endpointViewOf(observed observation) endpointView {
	stored := observed.stored
	view := endpointView{
		Namespace: stored.Namespace, Name: stored.Name,
		ResourceVersion: stored.ResourceVersion,
		Kind:            string(stored.Spec.Kind), TargetApp: stored.Spec.TargetApp,
		Host: stored.Spec.Host, Path: stored.Spec.Path, Port: stored.Spec.Port,
		ServiceName: stored.Spec.ServiceName,
		Phase:       stored.Status.Phase, Health: string(stored.Status.Health.State),
	}
	if stored.Spec.TLS != nil {
		view.IssuerRef = stored.Spec.TLS.IssuerRef
	}
	if stored.Spec.Auth != nil {
		view.AuthProvider = stored.Spec.Auth.ProviderRef
	}
	if certificate := stored.Status.Certificate; certificate != nil {
		var notAfter *time.Time
		if certificate.NotAfter != nil {
			moment := certificate.NotAfter.UTC()
			notAfter = &moment
		}
		view.Certificate = &certificateView{
			Issuer: certificate.Issuer, Ready: certificate.Ready,
			NotAfter: timeText(notAfter), ExpiringSoon: certificate.ExpiringSoon,
			Reason: certificate.Reason,
		}
	}
	return view
}

// certificateRow is one row of the certificate item: the endpoint and the
// certificate state its reconciler recorded (NET-C-1, KOSD-C-46).
type certificateRow struct {
	Namespace   string           `json:"namespace"`
	Name        string           `json:"name"`
	Certificate *certificateView `json:"certificate,omitempty"`
}

// certificates reads the certificate state of every endpoint. A served
// certificate and an unserved one are distinguishable: an endpoint without TLS
// carries no certificate at all, and one whose issuer has not written a
// certificate yet carries ready=false with its reason.
func (r *Readings) certificates(ctx context.Context) (any, *controlplane.Error) {
	observations, failure := r.observe(ctx)
	if failure != nil {
		return nil, failure
	}
	rows := make([]certificateRow, 0, len(observations))
	for _, observed := range observations {
		view := endpointViewOf(observed)
		rows = append(rows, certificateRow{
			Namespace: view.Namespace, Name: view.Name, Certificate: view.Certificate,
		})
	}
	return map[string]any{"window": endpoint.CertificateWarningWindow.String(), "endpoints": rows}, nil
}
