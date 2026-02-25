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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ipampredicates "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/predicates"
)

// InClusterProviderAdapter is used as middle layer for provider integration.
type InClusterProviderAdapter struct {
	Client           client.Client
	WatchFilterValue string
}

var _ ipamutil.ProviderAdapter = &InClusterProviderAdapter{}

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
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  inClusterPrefixPoolKind,
				}),
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  globalInClusterPrefixPoolKind,
				}),
			),
		)).
		WithOptions(controller.Options{
			// To avoid race conditions when allocating IP Addresses, we explicitly set this to 1
			MaxConcurrentReconciles: 1,
		}).
		Watches(
			&v1alpha2.InClusterIPPool{},
			handler.EnqueueRequestsFromMapFunc(i.ipPoolToIPClaims(inClusterIPPoolKind)),
			builder.WithPredicates(predicate.Or(
				resourceTransitionedToUnpaused(),
				poolNoLongerEmpty(),
			)),
		).
		Watches(
			&v1alpha2.GlobalInClusterIPPool{},
			handler.EnqueueRequestsFromMapFunc(i.ipPoolToIPClaims(globalInClusterIPPoolKind)),
			builder.WithPredicates(predicate.Or(
				resourceTransitionedToUnpaused(),
				poolNoLongerEmpty(),
			)),
		).
		Watches(
			&v1alpha2.InClusterPrefixPool{},
			handler.EnqueueRequestsFromMapFunc(i.prefixPoolToIPClaims(inClusterPrefixPoolKind)),
			builder.WithPredicates(predicate.Or(
				resourceTransitionedToUnpaused(),
				prefixPoolNoLongerEmpty(),
			)),
		).
		Watches(
			&v1alpha2.GlobalInClusterPrefixPool{},
			handler.EnqueueRequestsFromMapFunc(i.prefixPoolToIPClaims(globalInClusterPrefixPoolKind)),
			builder.WithPredicates(predicate.Or(
				resourceTransitionedToUnpaused(),
				prefixPoolNoLongerEmpty(),
			)),
		).
		Owns(&ipamv1.IPAddress{}, builder.WithPredicates(
			predicate.Or(
				ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  inClusterIPPoolKind,
				}),
				ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  globalInClusterIPPoolKind,
				}),
				ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  inClusterPrefixPoolKind,
				}),
				ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha2.GroupVersion.Group,
					Kind:  globalInClusterPrefixPoolKind,
				}),
			),
		))
	return nil
}

// ClaimHandlerFor returns a claim handler for a specific claim.
func (i *InClusterProviderAdapter) ClaimHandlerFor(_ client.Client, claim *ipamv1.IPAddressClaim) ipamutil.ClaimHandler {
	switch claim.Spec.PoolRef.Kind {
	case inClusterPrefixPoolKind, globalInClusterPrefixPoolKind:
		return &PrefixIPAddressClaimHandler{
			Client: i.Client,
			claim:  claim,
		}
	default:
		return &IPAddressClaimHandler{
			Client: i.Client,
			claim:  claim,
		}
	}
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
