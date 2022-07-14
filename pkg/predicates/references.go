// Package predicates implements predicates to filter events during ipamv1.IPAddressClaim processing.
package predicates

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterexpv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func processIfClaimReferencesPoolKind(gk metav1.GroupKind, obj client.Object) bool {
	var claim *clusterexpv1.IPAddressClaim
	var ok bool
	if claim, ok = obj.(*clusterexpv1.IPAddressClaim); !ok {
		return false
	}

	if claim.Spec.PoolRef.Kind != gk.Kind || claim.Spec.PoolRef.APIGroup == nil || *claim.Spec.PoolRef.APIGroup != gk.Group {
		return false
	}

	return true
}

// ClaimReferencesPoolKind is a predicate that ensures an ipamv1.IPAddressClaim references a specified pool kind.
func ClaimReferencesPoolKind(gk metav1.GroupKind) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfClaimReferencesPoolKind(gk, e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfClaimReferencesPoolKind(gk, e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfClaimReferencesPoolKind(gk, e.ObjectNew)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfClaimReferencesPoolKind(gk, e.Object)
		},
	}
}

func processIfAddressReferencesPoolKind(gk metav1.GroupKind, obj client.Object) bool {
	var addr *clusterexpv1.IPAddress
	var ok bool
	if addr, ok = obj.(*clusterexpv1.IPAddress); !ok {
		return false
	}

	if addr.Spec.PoolRef.Kind != gk.Kind || addr.Spec.PoolRef.APIGroup == nil || *addr.Spec.PoolRef.APIGroup != gk.Group {
		return false
	}

	return true
}

// AddressReferencesPoolKind is a predicate that ensures an ipamv1.IPAddress references a specified pool kind.
func AddressReferencesPoolKind(gk metav1.GroupKind) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfAddressReferencesPoolKind(gk, e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfAddressReferencesPoolKind(gk, e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfAddressReferencesPoolKind(gk, e.ObjectNew)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfAddressReferencesPoolKind(gk, e.Object)
		},
	}
}
