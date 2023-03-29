// Package index implements several indexes for the controller-runtime Managers cache.
package index

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// IPAddressPoolRefCombinedField is an index for the poolRef of an IPAddress.
	IPAddressPoolRefCombinedField = "index.poolRef"
)

// SetupIndexes adds indexes to the cache of a Manager.
func SetupIndexes(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetCache().IndexField(ctx, &ipamv1.IPAddress{},
		IPAddressPoolRefCombinedField,
		ipAddressByCombinedPoolRef,
	)
}

func ipAddressByCombinedPoolRef(o client.Object) []string {
	ip, ok := o.(*ipamv1.IPAddress)
	if !ok {
		panic(fmt.Sprintf("Expected an IPAddress but got a %T", o))
	}
	return []string{IPAddressPoolRefValue(ip.Spec.PoolRef)}
}

// IPAddressPoolRefValue turns a corev1.TypedLocalObjectReference to an indexable value.
func IPAddressPoolRefValue(ref corev1.TypedLocalObjectReference) string {
	return fmt.Sprintf("%s%s", ref.Kind, ref.Name)
}
