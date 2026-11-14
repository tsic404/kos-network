package network

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/tsic404/kosd/api/v1beta1"
)

// gvr resolves one kernel resource name to the group-version-resource the
// control plane's own client reads (KOSD-C-49).
func gvr(resource string) schema.GroupVersionResource {
	return v1beta1.GroupVersion.WithResource(resource)
}

// metav1ListOptions reads every object of a kind, across namespaces.
func metav1ListOptions() metav1.ListOptions { return metav1.ListOptions{} }
