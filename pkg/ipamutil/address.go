package ipamutil

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewIPAddress(claim *clusterv1exp.IPAddressClaim, pool client.Object) clusterv1exp.IPAddress {
	poolGVK := pool.GetObjectKind().GroupVersionKind()

	return clusterv1exp.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
			Labels: map[string]string{
				clusterv1exp.IPPoolLabel: pool.GetName(),
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pool, pool.GetObjectKind().GroupVersionKind()),
			},
		},
		Spec: clusterv1exp.IPAddressSpec{
			ClaimRef: corev1.LocalObjectReference{
				Name: claim.Name,
			},
			PoolRef: corev1.TypedLocalObjectReference{
				APIGroup: &poolGVK.Group,
				Kind:     poolGVK.Kind,
				Name:     pool.GetName(),
			},
		},
	}
}
