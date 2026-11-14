package network

import (
	"context"
	"sort"
	"time"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
)

// Alert is one certificate that needs attention on the network page: the
// endpoint it belongs to, the certificate's identity and the moment it expires
// (NET-C-1, KOSD-C-46).
type Alert struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Issuer       string `json:"issuer"`
	NotAfter     string `json:"notAfter,omitempty"`
	ExpiringSoon bool   `json:"expiringSoon"`
	Reason       string `json:"reason,omitempty"`
	// Identity pins the certificate the alert is about, so an acknowledged
	// alert stays acknowledged for that certificate and a renewed one warns
	// again (NET-C-3).
	Identity string `json:"identity"`
}

// certificateAlerts computes the expiry warning from certificate validity: a
// certificate that is not ready, reports an imminent expiry, or expires inside
// KOSD-C-46's warning window appears here (computed source, NET-C-1).
func (r *Readings) certificateAlerts(ctx context.Context) (any, *controlplane.Error) {
	observations, failure := r.observe(ctx)
	if failure != nil {
		return nil, failure
	}
	now := r.clock()
	alerts := make([]Alert, 0)
	for _, observed := range observations {
		entry := observed.entry
		if entry.Certificate == nil {
			continue
		}
		if !needsAttention(*entry.Certificate, now) {
			continue
		}
		alerts = append(alerts, Alert{
			Namespace: observed.stored.Namespace, Name: observed.stored.Name,
			Issuer:       entry.Certificate.Issuer,
			NotAfter:     timeText(entry.Certificate.NotAfter),
			ExpiringSoon: entry.Certificate.ExpiringSoon || expiringSoon(*entry.Certificate, now),
			Reason:       entry.Certificate.Reason,
			Identity:     CertificateIdentity(observed.stored.Namespace, observed.stored.Name, *entry.Certificate),
		})
	}
	sort.Slice(alerts, func(left, right int) bool {
		if alerts[left].Namespace != alerts[right].Namespace {
			return alerts[left].Namespace < alerts[right].Namespace
		}
		return alerts[left].Name < alerts[right].Name
	})
	return map[string]any{
		"window": endpoint.CertificateWarningWindow.String(),
		"alerts": alerts,
	}, nil
}

// needsAttention reports whether a certificate belongs in the alert list.
func needsAttention(certificate endpoint.Certificate, now time.Time) bool {
	return !certificate.Ready || certificate.ExpiringSoon || expiringSoon(certificate, now)
}

// expiringSoon reports an expiry inside KOSD-C-46's warning window.
func expiringSoon(certificate endpoint.Certificate, now time.Time) bool {
	if certificate.NotAfter == nil {
		return false
	}
	return !now.Add(endpoint.CertificateWarningWindow).Before(*certificate.NotAfter)
}

// CertificateIdentity names the certificate an alert is about (NET-C-3). The
// endpoint's status carries the issuer in use and the expiry of the served
// certificate, and a renewal or a replacement moves that expiry; the identity is
// therefore issuer plus expiry, and an acknowledged alert does not silence the
// next certificate.
func CertificateIdentity(namespace, name string, certificate endpoint.Certificate) string {
	notAfter := ""
	if certificate.NotAfter != nil {
		notAfter = certificate.NotAfter.UTC().Format(time.RFC3339Nano)
	}
	return namespace + "/" + name + "|" + certificate.Issuer + "|" + notAfter
}

// timeText renders an optional timestamp.
func timeText(moment *time.Time) string {
	if moment == nil {
		return ""
	}
	return moment.UTC().Format(time.RFC3339)
}
