package predicates

import (
	"testing"

	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterexpv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestClaimReferencesPoolKind(t *testing.T) {
	tests := []struct {
		name   string
		ref    v1.TypedLocalObjectReference
		result bool
	}{
		{
			name: "true for valid reference",
			ref: v1.TypedLocalObjectReference{
				APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
				Kind:     "InClusterIPPool",
			},
			result: true,
		},
		{
			name: "false when kind does not match",
			ref: v1.TypedLocalObjectReference{
				APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
				Kind:     "OutOfClusterIPPool",
			},
			result: false,
		},
		{
			name: "false when no group is set",
			ref: v1.TypedLocalObjectReference{
				Kind: "InClusterIPPool",
			},
			result: false,
		},
		{
			name: "false when group does not match",
			ref: v1.TypedLocalObjectReference{
				APIGroup: pointer.String("cluster.x-k8s.io"),
				Kind:     "InClusterIPPool",
			},
			result: false,
		},
	}

	gk := metav1.GroupKind{
		Group: "ipam.cluster.x-k8s.io",
		Kind:  "InClusterIPPool",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			claim := &clusterexpv1.IPAddressClaim{
				Spec: clusterexpv1.IPAddressClaimSpec{
					PoolRef: tt.ref,
				},
			}
			funcs := ClaimReferencesPoolKind(gk)
			g.Expect(funcs.CreateFunc(event.CreateEvent{Object: claim})).To(Equal(tt.result))
			g.Expect(funcs.DeleteFunc(event.DeleteEvent{Object: claim})).To(Equal(tt.result))
			g.Expect(funcs.GenericFunc(event.GenericEvent{Object: claim})).To(Equal(tt.result))
			g.Expect(funcs.UpdateFunc(event.UpdateEvent{ObjectNew: claim})).To(Equal(tt.result))
		})
	}
}
