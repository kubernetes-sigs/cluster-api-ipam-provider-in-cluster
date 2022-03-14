package predicates

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func processIfClaimReferencesPoolKind(gk v1.GroupKind, obj client.Object) bool {
	var claim *clusterv1.IPAddressClaim
	var ok bool
	if claim, ok = obj.(*clusterv1.IPAddressClaim); !ok {
		return false
	}

	poolGK := claim.Spec.Pool.GroupKind()

	if poolGK.Kind != gk.Kind || poolGK.Group != gk.Group {
		return false
	}

	return true
}

func ClaimReferencesPoolKind(gvk v1.GroupKind) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfClaimReferencesPoolKind(gvk, e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfClaimReferencesPoolKind(gvk, e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfClaimReferencesPoolKind(gvk, e.ObjectNew)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfClaimReferencesPoolKind(gvk, e.Object)
		},
	}
}
