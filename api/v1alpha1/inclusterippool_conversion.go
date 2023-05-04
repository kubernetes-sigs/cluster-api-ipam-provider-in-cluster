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

//nolint:forcetypeassert,golint,revive,stylecheck
package v1alpha1

import (
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1alpha2 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

// ConvertTo converts v1alpha1.InClusterIPPool to v1alpha2.InClusterIPPool.
func (src *InClusterIPPool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.InClusterIPPool)
	if err := Convert_v1alpha1_InClusterIPPool_To_v1alpha2_InClusterIPPool(src, dst, nil); err != nil {
		return err
	}

	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts v1alpha2.InClusterIPPool to v1alpha1.InClusterIPPool.
func (dst *InClusterIPPool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.InClusterIPPool)
	if err := Convert_v1alpha2_InClusterIPPool_To_v1alpha1_InClusterIPPool(src, dst, nil); err != nil {
		return err
	}

	restored := &InClusterIPPool{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	restoredV2 := &v1alpha2.InClusterIPPool{}
	if err := Convert_v1alpha1_InClusterIPPool_To_v1alpha2_InClusterIPPool(restored, restoredV2, nil); err != nil {
		return err
	}

	addressesAreEqual := true
	if len(restoredV2.Spec.Addresses) == len(src.Spec.Addresses) {
		for i := range restoredV2.Spec.Addresses {
			if restoredV2.Spec.Addresses[i] != src.Spec.Addresses[i] {
				addressesAreEqual = false
				break
			}
		}
	} else {
		addressesAreEqual = false
	}

	if addressesAreEqual &&
		restoredV2.Spec.Prefix == src.Spec.Prefix &&
		restoredV2.Spec.Gateway == src.Spec.Gateway {
		dst.Spec = restored.Spec
	}

	return nil
}

// ConvertTo converts v1alpha1.GlobalInClusterIPPool to v1alpha2.GlobalInClusterIPPool.
func (src *GlobalInClusterIPPool) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.GlobalInClusterIPPool)
	if err := Convert_v1alpha1_GlobalInClusterIPPool_To_v1alpha2_GlobalInClusterIPPool(src, dst, nil); err != nil {
		return err
	}

	if err := utilconversion.MarshalData(src, dst); err != nil {
		return err
	}

	return nil
}

// ConvertTo converts v1alpha2.GlobalInClusterIPPool to v1alpha1.GlobalInClusterIPPool.
func (dst *GlobalInClusterIPPool) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.GlobalInClusterIPPool)
	if err := Convert_v1alpha2_GlobalInClusterIPPool_To_v1alpha1_GlobalInClusterIPPool(src, dst, nil); err != nil {
		return err
	}

	restored := &GlobalInClusterIPPool{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Spec = restored.Spec
	return nil
}

// ConvertTo converts v1alpha1.InClusterIPPoolList to v1alpha2.InClusterIPPoolList.
func (src *InClusterIPPoolList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.InClusterIPPoolList)
	return Convert_v1alpha1_InClusterIPPoolList_To_v1alpha2_InClusterIPPoolList(src, dst, nil)
}

// ConvertTo converts v1alpha2.InClusterIPPoolList to v1alpha1.InClusterIPPoolList.
func (dst *InClusterIPPoolList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.InClusterIPPoolList)
	return Convert_v1alpha2_InClusterIPPoolList_To_v1alpha1_InClusterIPPoolList(src, dst, nil)
}

// ConvertTo converts v1alpha1.GlobalInClusterIPPoolList to v1alpha2.GlobalInClusterIPPoolList.
func (src *GlobalInClusterIPPoolList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.GlobalInClusterIPPoolList)
	return Convert_v1alpha1_GlobalInClusterIPPoolList_To_v1alpha2_GlobalInClusterIPPoolList(src, dst, nil)
}

// ConvertTo converts v1alpha2.GlobalInClusterIPPoolList to v1alpha1.GlobalInClusterIPPoolList.
func (dst *GlobalInClusterIPPoolList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.GlobalInClusterIPPoolList)
	return Convert_v1alpha2_GlobalInClusterIPPoolList_To_v1alpha1_GlobalInClusterIPPoolList(src, dst, nil)
}
