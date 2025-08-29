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

// Package ipamutil implements various utility functions to assist with CAPI IPAM implementation.
package ipamutil

import (
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// NewIPAddress creates a new ipamv1.IPAddress with references to a pool and claim.
func NewIPAddress(claim *ipamv1.IPAddressClaim, pool client.Object) ipamv1.IPAddress {
	poolGVK := pool.GetObjectKind().GroupVersionKind()

	return ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
		},
		Spec: ipamv1.IPAddressSpec{
			ClaimRef: ipamv1.IPAddressClaimReference{
				Name: claim.Name,
			},
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: poolGVK.Group,
				Kind:     poolGVK.Kind,
				Name:     pool.GetName(),
			},
		},
	}
}

// ensureIPAddressOwnerReferences ensures that an IPAddress has the
// IPAddressClaim and IPPool as an OwnerReference.
func ensureIPAddressOwnerReferences(scheme *runtime.Scheme, address *ipamv1.IPAddress, claim *ipamv1.IPAddressClaim, pool client.Object) error {
	if err := controllerutil.SetControllerReference(claim, address, scheme); err != nil {
		if _, ok := err.(*controllerutil.AlreadyOwnedError); !ok {
			return errors.Wrap(err, "Failed to update address's claim owner reference")
		}
	}

	if err := controllerutil.SetOwnerReference(pool, address, scheme); err != nil {
		return errors.Wrap(err, "Failed to update address's pool owner reference")
	}

	var poolRefIdx int
	poolGVK := pool.GetObjectKind().GroupVersionKind()
	for i, ownerRef := range address.GetOwnerReferences() {
		if ownerRef.APIVersion == poolGVK.GroupVersion().String() &&
			ownerRef.Kind == poolGVK.Kind &&
			ownerRef.Name == pool.GetName() {
			poolRefIdx = i
		}
	}

	address.OwnerReferences[poolRefIdx].Controller = ptr.To(false)
	address.OwnerReferences[poolRefIdx].BlockOwnerDeletion = ptr.To(true)

	return nil
}
