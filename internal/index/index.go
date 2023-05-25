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

// Package index implements several indexes for the controller-runtime Managers cache.
package index

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// IPAddressPoolRefCombinedField is an index for the poolRef of an IPAddress.
	IPAddressPoolRefCombinedField = "index.poolRef"

	// IPAddressClaimPoolRefCombinedField is an index for the poolRef of an IPAddressClaim.
	IPAddressClaimPoolRefCombinedField = "index.poolRef"
)

// SetupIndexes adds indexes to the cache of a Manager.
func SetupIndexes(ctx context.Context, mgr manager.Manager) error {
	err := mgr.GetCache().IndexField(ctx, &ipamv1.IPAddress{},
		IPAddressPoolRefCombinedField,
		IPAddressByCombinedPoolRef,
	)
	if err != nil {
		return err
	}

	return mgr.GetCache().IndexField(ctx, &ipamv1.IPAddressClaim{},
		IPAddressClaimPoolRefCombinedField,
		ipAddressClaimByCombinedPoolRef,
	)
}

// IPAddressByCombinedPoolRef fulfills the IndexerFunc for IPAddress poolRefs.
func IPAddressByCombinedPoolRef(o client.Object) []string {
	ip, ok := o.(*ipamv1.IPAddress)
	if !ok {
		panic(fmt.Sprintf("Expected an IPAddress but got a %T", o))
	}
	return []string{IPPoolRefValue(ip.Spec.PoolRef)}
}

func ipAddressClaimByCombinedPoolRef(o client.Object) []string {
	ip, ok := o.(*ipamv1.IPAddressClaim)
	if !ok {
		panic(fmt.Sprintf("Expected an IPAddressClaim but got a %T", o))
	}
	return []string{IPPoolRefValue(ip.Spec.PoolRef)}
}

// IPPoolRefValue turns a corev1.TypedLocalObjectReference to an indexable value.
func IPPoolRefValue(ref corev1.TypedLocalObjectReference) string {
	return fmt.Sprintf("%s%s", ref.Kind, ref.Name)
}
