package predicates

import (
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterexpv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func processIfClaimReferencesPoolKind(gk v1.GroupKind, obj client.Object) bool {
	var claim *clusterexpv1.IPAddressClaim
	var ok bool
	fmt.Println(obj)
	if claim, ok = obj.(*clusterexpv1.IPAddressClaim); !ok {
		return false
	}

	if claim.Spec.PoolRef.Kind != gk.Kind || claim.Spec.PoolRef.APIGroup == nil || *claim.Spec.PoolRef.APIGroup != gk.Group {
		return false
	}

	return true
}

func ClaimReferencesPoolKind(gk v1.GroupKind) predicate.Funcs {
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
