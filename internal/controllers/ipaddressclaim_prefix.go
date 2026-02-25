/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/index"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	pooltypes "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

// PrefixIPAddressClaimHandler reconciles claims against prefix pool resources.
type PrefixIPAddressClaimHandler struct {
	client.Client
	claim *ipamv1.IPAddressClaim
	pool  pooltypes.GenericInClusterPrefixPool
}

var _ ipamutil.ClaimHandler = &PrefixIPAddressClaimHandler{}

func (i *InClusterProviderAdapter) prefixPoolToIPClaims(kind string) func(context.Context, client.Object) []reconcile.Request {
	return func(ctx context.Context, a client.Object) []reconcile.Request {
		pool, ok := a.(pooltypes.GenericInClusterPrefixPool)
		if !ok {
			return nil
		}
		requests := []reconcile.Request{}
		claims := &ipamv1.IPAddressClaimList{}
		err := i.Client.List(ctx, claims,
			client.MatchingFields{
				index.IPAddressClaimPoolRefCombinedField: index.IPPoolRefValue(ipamv1.IPPoolReference{
					Name:     pool.GetName(),
					Kind:     kind,
					APIGroup: v1alpha2.GroupVersion.Group,
				}),
			},
			client.InNamespace(pool.GetNamespace()),
		)
		if err != nil {
			return requests
		}
		for _, claim := range claims.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})
		}
		return requests
	}
}

// FetchPool fetches the (Global)InClusterPrefixPool.
func (h *PrefixIPAddressClaimHandler) FetchPool(ctx context.Context) (client.Object, *ctrl.Result, error) {
	switch h.claim.Spec.PoolRef.Kind {
	case inClusterPrefixPoolKind:
		pool := &v1alpha2.InClusterPrefixPool{}
		if err := h.Get(ctx, types.NamespacedName{Namespace: h.claim.Namespace, Name: h.claim.Spec.PoolRef.Name}, pool); err != nil {
			return nil, nil, fmt.Errorf("failed to fetch pool: %w", err)
		}
		h.pool = pool
	case globalInClusterPrefixPoolKind:
		pool := &v1alpha2.GlobalInClusterPrefixPool{}
		if err := h.Get(ctx, types.NamespacedName{Name: h.claim.Spec.PoolRef.Name}, pool); err != nil {
			return nil, nil, fmt.Errorf("failed to fetch pool: %w", err)
		}
		h.pool = pool
	}

	if h.pool == nil {
		err := errors.New("pool not found")
		ctrl.LoggerFrom(ctx).Error(err, "the referenced pool could not be found")
		return nil, nil, nil
	}

	return h.pool, nil, nil
}

// EnsureAddress ensures that the IPAddress contains a valid prefix allocation.
func (h *PrefixIPAddressClaimHandler) EnsureAddress(ctx context.Context, address *ipamv1.IPAddress) (*ctrl.Result, error) {
	addressesInUse, err := poolutil.ListAddressesInUse(ctx, h.Client, h.pool.GetNamespace(), h.claim.Spec.PoolRef)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses: %w", err)
	}

	allocated := slices.ContainsFunc(addressesInUse, func(a ipamv1.IPAddress) bool {
		return a.Name == address.Name && a.Namespace == address.Namespace
	})

	if !allocated {
		poolSpec := h.pool.PoolSpec()
		inUseIPSet, err := poolutil.AddressesToAllocationPrefixSet(addressesInUse)
		if err != nil {
			return nil, fmt.Errorf("failed to convert IPAddressList to allocation prefix set: %w", err)
		}

		freePrefix, err := poolutil.FindFreePrefix(poolutil.NewPrefixPoolConfig(poolSpec), inUseIPSet)
		if err != nil {
			return nil, fmt.Errorf("failed to find free prefix: %w", err)
		}

		address.Spec.Address = freePrefix.Addr().String()
		address.Spec.Gateway = poolSpec.Gateway
		address.Spec.Prefix = ptr.To(int32(freePrefix.Bits())) //nolint:gosec // Bits() is [0,128] for valid prefixes
	}
	return nil, nil
}

// ReleaseAddress releases the allocated prefix.
func (h *PrefixIPAddressClaimHandler) ReleaseAddress(_ context.Context) (*ctrl.Result, error) {
	// We don't need to do anything here, since the prefix is released when the IPAddress is deleted.
	return nil, nil
}

func prefixPoolStatus(o client.Object) *v1alpha2.InClusterPrefixPoolStatusAddresses {
	switch ipPool := o.(type) {
	case *v1alpha2.InClusterPrefixPool:
		return ipPool.Status.Addresses
	case *v1alpha2.GlobalInClusterPrefixPool:
		return ipPool.Status.Addresses
	default:
		return nil
	}
}

// prefixPoolNoLongerEmpty only returns true if the pool status previously had
// 0 free prefixes and now has free prefixes.
func prefixPoolNoLongerEmpty() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldStatus := prefixPoolStatus(e.ObjectOld)
			newStatus := prefixPoolStatus(e.ObjectNew)
			if oldStatus != nil && newStatus != nil {
				if oldStatus.Free == 0 && newStatus.Free > 0 {
					return true
				}
			}
			return false
		},
	}
}
