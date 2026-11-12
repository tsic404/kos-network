package network

import (
	"context"

	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/endpoint"
)

// ledgerProjection reads the port ledger the endpoint reconciler maintains
// (KOSD-C-44). The projection is derived state: every row names its endpoint,
// the keys it holds and the conflicts that keep it from being admitted, so a
// duplicate created behind the API's back is listed instead of coexisting
// silently. The item is read-only: the ledger belongs to the reconciler alone
// (NET-C-1).
func (r *Readings) ledgerProjection(ctx context.Context) (any, *controlplane.Error) {
	observations, failure := r.observe(ctx)
	if failure != nil {
		return nil, failure
	}
	ledger := ledgerOf(observations)
	return map[string]any{
		"projection": ledger.ProjectionAt(r.clock()),
		"note":       endpoint.SharedMediumNote,
	}, nil
}
