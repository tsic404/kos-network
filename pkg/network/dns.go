package network

import (
	"context"
	"net"
	"sort"
	"time"

	"github.com/tsic404/kosd/pkg/controlplane"
)

// probeTimeout bounds one DNS probe: the page reads a result with its
// observation time, never a blocked request (NET-C-1).
const probeTimeout = 2 * time.Second

// probeLimit bounds how many hosts one read probes.
const probeLimit = 32

// resolutionResult is one host's read-time probe result (NET-C-1).
type resolutionResult struct {
	Host      string   `json:"host"`
	Addresses []string `json:"addresses,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// resolution probes the hosts the endpoints declare. The page shows the
// answers this read observed and the moment the shell stamps the read, because
// a probe result is a read-time fact, not a stored one (NET-C-1, KOSD-C-53).
func (r *Readings) resolution(ctx context.Context) (any, *controlplane.Error) {
	observations, failure := r.observe(ctx)
	if failure != nil {
		return nil, failure
	}
	hosts := make([]string, 0, len(observations))
	seen := map[string]bool{}
	for _, observed := range observations {
		host := observed.stored.Spec.Host
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	if len(hosts) > probeLimit {
		hosts = hosts[:probeLimit]
	}
	resolver := r.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	results := make([]resolutionResult, 0, len(hosts))
	for _, host := range hosts {
		addresses, err := resolver.LookupHost(probeCtx, host)
		result := resolutionResult{Host: host, Addresses: addresses}
		if err != nil {
			result.Addresses = nil
			result.Error = err.Error()
		}
		results = append(results, result)
	}
	return map[string]any{
		"probe":   "dns",
		"timeout": probeTimeout.String(),
		"results": results,
	}, nil
}
