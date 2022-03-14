package ipamutil

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewIPAddress(claim *clusterv1exp.IPAddressClaim, pool client.Object) clusterv1exp.IPAddress {
	gv, _ := schema.ParseGroupVersion(claim.APIVersion)

	return clusterv1exp.IPAddress{
		ObjectMeta: v1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
			Labels: map[string]string{
				clusterv1exp.IPPoolLabel: pool.GetName(),
			},
			OwnerReferences: []v1.OwnerReference{
				*v1.NewControllerRef(pool, pool.GetObjectKind().GroupVersionKind()),
			},
		},
		Spec: clusterv1exp.IPAddressSpec{
			Claim: clusterv1.PinnedLocalObjectReference{
				Group: gv.Group,
				Kind:  claim.Kind,
				Name:  claim.Name,
				UID:   claim.UID,
			},
			Pool: *clusterv1.NewPinnedLocalObjectReference(pool),
		},
	}
}
