package support

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/endpoint"
	"github.com/tsic404/kosd/pkg/reconcile"
	"github.com/tsic404/kosd/tests/acceptance/harness"

	"github.com/tsic404/kos-network/pkg/network"
)

// Reconciler builds an endpoint reconciler over this world's cluster.
func (w *World) Reconciler(facts endpoint.Facts) *endpoint.Reconciler {
	w.t.Helper()
	return endpoint.NewReconciler(w.Cluster.Writer(), facts)
}

// Reconcile runs one endpoint round and fails the case on a round error.
func (w *World) Reconcile(reconciler *endpoint.Reconciler, name string) {
	w.t.Helper()
	harness.ReconcileEndpoint(w.t, reconciler, name)
}

// Application creates one application the endpoints may target.
func (w *World) Application(name string) {
	w.t.Helper()
	w.Cluster.Create(w.t, harness.Application(name, nil))
}

// Endpoint creates one KOS-managed endpoint natively, the way an object that
// bypassed the page exists (KOSD-C-44).
func (w *World) Endpoint(name string, spec map[string]any) {
	w.t.Helper()
	w.Cluster.Create(w.t, harness.Endpoint(name, harness.ManagedLabels(), spec))
}

// EndpointObject reads one stored endpoint.
func (w *World) EndpointObject(name string) *unstructured.Unstructured {
	w.t.Helper()
	return w.Cluster.Get(w.t, "v1beta1", "Endpoint", name)
}

// ResourceVersion reads the precondition a page update or delete carries from
// the network page itself (KOSD-C-12): a browser holds no cluster credential,
// so the row it read must supply the value.
func (w *World) ResourceVersion(name string) string {
	w.t.Helper()
	row := Row(w.t, Rows(w.t, w.Page(), network.ItemEndpoints, "endpoints"), "name", name)
	version := harness.String(row, "resourceVersion")
	if version == "" {
		w.t.Fatalf("the page row of %s carries no resourceVersion: %v", name, row)
	}
	return version
}

// Provider registers one network provider with a verified capability and the
// conditions the validator writes (KOSD-C-47).
func (w *World) Provider(name, version, capability, capabilityVersion string) {
	w.t.Helper()
	object := map[string]any{
		"apiVersion": v1beta1.GroupVersion.String(),
		"kind":       "Provider",
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"componentRef": name,
			"version":      version,
			"capabilities": []any{map[string]any{
				"name": capability, "version": capabilityVersion,
				"scope": "Cluster", "caseSet": capability, "caseSetDigest": "sha256:cases",
			}},
		},
		"status": map[string]any{
			"phase":  "Ready",
			"health": map[string]any{"state": "Healthy"},
			"effectiveCapabilities": []any{map[string]any{
				"name": capability, "version": capabilityVersion,
				"scope": "Cluster", "caseSet": capability, "caseSetDigest": "sha256:cases",
			}},
			"capabilities": []any{map[string]any{
				"name": capability, "version": capabilityVersion, "state": "Verified",
			}},
			"conditions": []any{
				map[string]any{"type": "Registered", "status": "True", "reason": "Registered", "message": "已登记", "lastTransitionTime": w.nowText()},
				map[string]any{"type": "Verified", "status": "True", "reason": "Verified", "message": "校验通过", "lastTransitionTime": w.nowText()},
				map[string]any{"type": "Ready", "status": "True", "reason": "Ready", "message": "就绪", "lastTransitionTime": w.nowText()},
			},
		},
	}
	w.Cluster.Create(w.t, &unstructured.Unstructured{Object: object})
}

// nowText renders the wall clock for object conditions.
func (w *World) nowText() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// WriteCertificate stores a self-signed certificate at the TLS secret name the
// endpoint's route declares, with validity relative to now (KOSD-C-46).
func (w *World) WriteCertificate(name string, notBefore, notAfter time.Duration) {
	w.t.Helper()
	secretName := w.routeSecretName(name)
	certificate, key := selfSigned(w.t, name, notBefore, notAfter)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: harness.Namespace},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{"tls.crt": certificate, "tls.key": key},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	secrets := w.Cluster.Kube.CoreV1().Secrets(harness.Namespace)
	if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			w.t.Fatalf("create certificate secret %s: %v", secretName, err)
		}
		existing, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			w.t.Fatalf("read certificate secret %s: %v", secretName, err)
		}
		existing.Data = secret.Data
		if _, err := secrets.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			w.t.Fatalf("replace certificate secret %s: %v", secretName, err)
		}
	}
}

// routeSecretName reads the TLS secret name the endpoint's route declares.
func (w *World) routeSecretName(name string) string {
	w.t.Helper()
	ingress := harness.IngressOf(w.t, w.Cluster, name)
	if ingress == nil {
		w.t.Fatalf("%s owns no ingress route", name)
	}
	if len(ingress.Spec.TLS) != 1 || ingress.Spec.TLS[0].SecretName == "" {
		w.t.Fatalf("the route of %s declares no TLS secret: %+v", name, ingress.Spec.TLS)
	}
	return ingress.Spec.TLS[0].SecretName
}

// selfSigned mints a certificate whose validity is relative to now.
func selfSigned(t *testing.T, name string, notBefore, notAfter time.Duration) ([]byte, []byte) {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: name + ".test"},
		NotBefore:    now.Add(notBefore),
		NotAfter:     now.Add(notAfter),
		DNSNames:     []string{name + ".test"},
	}
	encoded, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	key, err := x509.MarshalECPrivateKey(private)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key})
}

// HostShape builds an http.host spec.
func HostShape(app, host string) map[string]any {
	return map[string]any{"kind": "http.host", "targetApp": app, "host": host}
}

// TLSShape builds an http.host spec that requests a certificate; an empty issuer
// omits the reference so the reconciler resolves the built-in issuer.
func TLSShape(app, host, issuer string) map[string]any {
	tls := map[string]any{}
	if issuer != "" {
		tls["issuerRef"] = issuer
	}
	return map[string]any{"kind": "http.host", "targetApp": app, "host": host, "tls": tls}
}

// ReconcileKind runs one round of one kind and fails on a round error.
func (w *World) ReconcileKind(reconciler *endpoint.Reconciler, kind, name string) {
	w.t.Helper()
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{Kind: kind, Namespace: harness.Namespace, Name: name}); err != nil {
		w.t.Fatalf("reconcile %s %s: %v", kind, name, err)
	}
}
