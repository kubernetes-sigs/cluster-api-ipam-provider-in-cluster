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
	"fmt"
	"slices"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/index"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ipampredicates "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/predicates"
	pooltypes "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

const (
	// ReleaseAddressFinalizer is used to release an IP address before cleaning up the claim.
	ReleaseAddressFinalizer = "ipam.cluster.x-k8s.io/ReleaseAddress"

	// ProtectAddressFinalizer is used to prevent deletion of an IPAddress object while its claim is not deleted.
	ProtectAddressFinalizer = "ipam.cluster.x-k8s.io/ProtectAddress"
)

type genericInClusterPool interface {
	client.Object
	PoolSpec() *v1alpha2.InClusterIPPoolSpec
}

// InClusterProviderAdapter is used as middle layer for provider integration.
type InClusterProviderAdapter struct {
	Client           client.Client
	WatchFilterValue string
}

var _ ipamutil.ProviderAdapter = &InClusterProviderAdapter{}

// IPAddressClaimHandler reconciles a InClusterIPPool object.
type IPAddressClaimHandler struct {
	client.Client
	claim *ipamv1.IPAddressClaim
	pool  genericInClusterPool
}

var _ ipamutil.ClaimHandler = &IPAddressClaimHandler{}

// SetupWithManager sets up the controller with the Manager.
func (i *InClusterProviderAdapter) SetupWithManager(_ context.Context, b *ctrl.Builder) error {
	b.
		For(&ipamv1.IPAddressClaim{}, builder.WithPredicates(
			predicate.Or(
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  inClusterIPPoolKind,
				}),
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  globalInClusterIPPoolKind,
				}),
			),
		)).
		WithOptions(controller.Options{
			// To avoid race conditions when allocating IP Addresses, we explicitly set this to 1
			MaxConcurrentReconciles: 1,
		}).
		Watches(
			&v1alpha2.InClusterIPPool{},
			handler.EnqueueRequestsFromMapFunc(i.inClusterIPPoolToIPClaims("InClusterIPPool")),
			builder.WithPredicates(predicate.Or(
				resourceTransitionedToUnpaused(),
				poolNoLongerEmpty(),
			)),
		).
		Watches(
			&v1alpha2.GlobalInClusterIPPool{},
			handler.EnqueueRequestsFromMapFunc(i.inClusterIPPoolToIPClaims("GlobalInClusterIPPool")),
			builder.WithPredicates(predicate.Or(
				resourceTransitionedToUnpaused(),
				poolNoLongerEmpty(),
			)),
		).
		Owns(&ipamv1.IPAddress{}, builder.WithPredicates(
			ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
				Group: v1alpha2.GroupVersion.Group,
				Kind:  inClusterIPPoolKind,
			}),
		))
	return nil
}

func (i *InClusterProviderAdapter) inClusterIPPoolToIPClaims(kind string) func(context.Context, client.Object) []reconcile.Request {
	return func(ctx context.Context, a client.Object) []reconcile.Request {
		pool := a.(pooltypes.GenericInClusterPool)
		requests := []reconcile.Request{}
		claims := &ipamv1.IPAddressClaimList{}
		err := i.Client.List(ctx, claims,
			client.MatchingFields{
				"index.poolRef": index.IPPoolRefValue(corev1.TypedLocalObjectReference{
					Name:     pool.GetName(),
					Kind:     kind,
					APIGroup: &v1alpha2.GroupVersion.Group,
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

// ClaimHandlerFor returns a claim handler for a specific claim.
func (i *InClusterProviderAdapter) ClaimHandlerFor(_ client.Client, claim *ipamv1.IPAddressClaim) ipamutil.ClaimHandler {
	return &IPAddressClaimHandler{
		Client: i.Client,
		claim:  claim,
	}
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
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch

// FetchPool fetches the (Global)InClusterIPPool.
func (h *IPAddressClaimHandler) FetchPool(ctx context.Context) (client.Object, *ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if h.claim.Spec.PoolRef.Kind == inClusterIPPoolKind {
		icippool := &v1alpha2.InClusterIPPool{}
		if err := h.Client.Get(ctx, types.NamespacedName{Namespace: h.claim.Namespace, Name: h.claim.Spec.PoolRef.Name}, icippool); err != nil {
			return nil, nil, errors.Wrap(err, "failed to fetch pool")
		}
		h.pool = icippool
	} else if h.claim.Spec.PoolRef.Kind == globalInClusterIPPoolKind {
		gicippool := &v1alpha2.GlobalInClusterIPPool{}
		if err := h.Client.Get(ctx, types.NamespacedName{Name: h.claim.Spec.PoolRef.Name}, gicippool); err != nil {
			return nil, nil, err
		}
		h.pool = gicippool
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
		address.Spec.Prefix = poolSpec.Prefix
	}

	return nil, nil
}

// ReleaseAddress releases the ip address.
func (h *IPAddressClaimHandler) ReleaseAddress(_ context.Context) (*ctrl.Result, error) {
	// We don't need to do anything here, since the ip address is released when the IPAddress is deleted
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

func resourceTransitionedToUnpaused() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return annotations.HasPaused(e.ObjectOld) && !annotations.HasPaused(e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return !annotations.HasPaused(e.Object)
		},
	}
}

func poolStatus(o client.Object) *v1alpha2.InClusterIPPoolStatusIPAddresses {
	if ipPool, ok := o.(*v1alpha2.InClusterIPPool); ok {
		return ipPool.Status.Addresses
	} else if ipPool, ok := o.(*v1alpha2.GlobalInClusterIPPool); ok {
		return ipPool.Status.Addresses
	}
	return nil
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
