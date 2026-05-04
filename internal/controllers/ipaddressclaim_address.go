/*
Copyright 2023 The Kubernetes Authors.

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
	"time"

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

type genericInClusterPool interface {
	client.Object
	PoolSpec() *v1alpha2.InClusterIPPoolSpec
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/finalizers,verbs=update
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools/finalizers,verbs=update
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/finalizers;ipaddresses/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch

// IPAddressClaimHandler reconciles claims against IP address pool resources.
type IPAddressClaimHandler struct {
	client.Client
	claim *ipamv1.IPAddressClaim
	pool  genericInClusterPool
}

var _ ipamutil.ClaimHandler = &IPAddressClaimHandler{}

func (i *InClusterProviderAdapter) ipPoolToIPClaims(kind string) func(context.Context, client.Object) []reconcile.Request {
	return func(ctx context.Context, a client.Object) []reconcile.Request {
		pool := a.(pooltypes.GenericInClusterPool)
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
			r := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			}
			requests = append(requests, r)
		}
		return requests
	}
}

// FetchPool fetches the (Global)InClusterIPPool.
func (h *IPAddressClaimHandler) FetchPool(ctx context.Context) (client.Object, *ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	switch h.claim.Spec.PoolRef.Kind {
	case inClusterIPPoolKind:
		pool := &v1alpha2.InClusterIPPool{}
		if err := h.Get(ctx, types.NamespacedName{Namespace: h.claim.Namespace, Name: h.claim.Spec.PoolRef.Name}, pool); err != nil {
			return nil, nil, fmt.Errorf("failed to fetch pool: %w", err)
		}
		h.pool = pool
	case globalInClusterIPPoolKind:
		pool := &v1alpha2.GlobalInClusterIPPool{}
		if err := h.Get(ctx, types.NamespacedName{Name: h.claim.Spec.PoolRef.Name}, pool); err != nil {
			return nil, nil, fmt.Errorf("failed to fetch pool: %w", err)
		}
		h.pool = pool
	}

	if h.pool == nil {
		err := errors.New("pool not found")
		log.Error(err, "the referenced pool could not be found")
		return nil, nil, nil
	}

	return h.pool, nil, nil
}

// EnsureAddress ensures that the IPAddress contains a valid address.
func (h *IPAddressClaimHandler) EnsureAddress(ctx context.Context, address *ipamv1.IPAddress) (*ctrl.Result, error) {
	addressesInUse, err := poolutil.ListAddressesInUse(ctx, h.Client, h.pool.GetNamespace(), h.claim.Spec.PoolRef)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses: %w", err)
	}

	allocated := slices.ContainsFunc(addressesInUse, func(a ipamv1.IPAddress) bool {
		return a.Name == address.Name && a.Namespace == address.Namespace
	})

	if !allocated {
		poolSpec := h.pool.PoolSpec()
		inUseIPSet, err := poolutil.AddressesToIPSet(buildAddressList(addressesInUse, poolSpec.Gateway))
		if err != nil {
			return nil, fmt.Errorf("failed to convert IPAddressList to IPSet: %w", err)
		}

		poolIPSet, err := poolutil.PoolSpecToIPSet(poolSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to convert pool to range: %w", err)
		}

		freeIP, err := poolutil.FindFreeAddress(poolIPSet, inUseIPSet)
		if err != nil {
			return nil, fmt.Errorf("failed to find free address: %w", err)
		}

		address.Spec.Address = freeIP.String()
		address.Spec.Gateway = poolSpec.Gateway
		address.Spec.Prefix = ptr.To(int32(poolSpec.Prefix)) //nolint:gosec
	}
	return nil, nil
}

// ReleaseAddress implements the grace period logic for IP address reuse.
// When a grace period is configured on the pool, this method delays the
// actual deletion of the IPAddress object by returning a requeue result
// until the grace period has elapsed since the claim's deletion timestamp.
func (h *IPAddressClaimHandler) ReleaseAddress(ctx context.Context) (*ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if h.claim.DeletionTimestamp == nil {
		return nil, nil
	}

	// Skip grace period if pool was deleted or is being deleted (e.g. cluster teardown).
	if h.pool == nil || !h.pool.GetDeletionTimestamp().IsZero() {
		return nil, nil
	}

	gracePeriodSeconds := h.pool.PoolSpec().AddressReuseGracePeriodSeconds
	// A nil or zero value means no grace period; addresses are available for reuse immediately.
	if gracePeriodSeconds == nil || *gracePeriodSeconds <= 0 {
		return nil, nil
	}

	gracePeriod := time.Duration(*gracePeriodSeconds) * time.Second
	elapsed := time.Since(h.claim.DeletionTimestamp.Time)
	remaining := gracePeriod - elapsed
	if remaining > 0 {
		log.Info("Address reuse grace period active, delaying IP address release",
			"remaining", remaining.Round(time.Second).String(),
			"gracePeriodSeconds", *gracePeriodSeconds)
		return &ctrl.Result{RequeueAfter: remaining}, nil
	}

	return nil, nil
}

func buildAddressList(addressesInUse []ipamv1.IPAddress, gateway string) []string {
	// Add extra capacity for the case that the pool's gateway is specified
	addrStrings := make([]string, len(addressesInUse), len(addressesInUse)+1)
	for i, address := range addressesInUse {
		addrStrings[i] = address.Spec.Address
	}

	if gateway != "" {
		addrStrings = append(addrStrings, gateway)
	}

	return addrStrings
}

func poolStatus(o client.Object) *v1alpha2.InClusterIPPoolStatusIPAddresses {
	switch ipPool := o.(type) {
	case *v1alpha2.InClusterIPPool:
		return ipPool.Status.Addresses
	case *v1alpha2.GlobalInClusterIPPool:
		return ipPool.Status.Addresses
	default:
		return nil
	}
}

// poolNoLongerEmpty only returns true if the Pool status previously had 0 free
// addresses and now has free addresses.
func poolNoLongerEmpty() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldStatus := poolStatus(e.ObjectOld)
			newStatus := poolStatus(e.ObjectNew)
			if oldStatus != nil && newStatus != nil {
				if oldStatus.Free == 0 && newStatus.Free > 0 {
					return true
				}
			}
			return false
		},
	}
}
