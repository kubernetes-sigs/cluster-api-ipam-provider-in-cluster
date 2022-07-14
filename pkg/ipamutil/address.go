// Package ipamutil implements various utility functions to assist with CAPI IPAM implementation.
package ipamutil

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewIPAddress creates a new ipamv1.IPAddress with references to a pool and claim.
func NewIPAddress(claim *ipamv1.IPAddressClaim, pool client.Object) ipamv1.IPAddress {
	poolGVK := pool.GetObjectKind().GroupVersionKind()

	return ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(claim, claim.GetObjectKind().GroupVersionKind()),
				{
					APIVersion:         pool.GetObjectKind().GroupVersionKind().GroupVersion().String(),
					Kind:               pool.GetObjectKind().GroupVersionKind().Kind,
					Name:               pool.GetName(),
					UID:                pool.GetUID(),
					BlockOwnerDeletion: pointer.Bool(true),
					Controller:         pointer.Bool(false),
				},
			},
		},
		Spec: ipamv1.IPAddressSpec{
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
