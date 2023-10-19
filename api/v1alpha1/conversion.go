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

package v1alpha1

import (
	"fmt"
	"net/netip"

	"go4.org/netipx"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/util/validation/field"

	v1alpha2 "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

func Convert_v1alpha1_InClusterIPPoolSpec_To_v1alpha2_InClusterIPPoolSpec(in *InClusterIPPoolSpec, out *v1alpha2.InClusterIPPoolSpec, _ conversion.Scope) error {
	out.Gateway = in.Gateway
	out.Prefix = in.Prefix
	out.Addresses = in.Addresses

	if in.Subnet != "" {
		prefix, err := netip.ParsePrefix(in.Subnet)
		if err != nil {
			return field.Invalid(field.NewPath("spec", "subnet"), in.Subnet, err.Error())
		}

		prefixRange := netipx.RangeOfPrefix(prefix)
		if in.First == "" {
			in.First = prefixRange.From().Next().String() // omits the first address, the assumed network address
		}
		if in.Last == "" {
			in.Last = prefixRange.To().Prev().String() // omits the last address, the assumed broadcast
		}
		if in.Prefix == 0 {
			in.Prefix = prefix.Bits()
			out.Prefix = prefix.Bits()
		}
	}

	if in.First != "" && in.Last != "" {
		out.Addresses = append(out.Addresses, fmt.Sprintf("%s-%s", in.First, in.Last))
	}

	return nil
}

func Convert_v1alpha2_InClusterIPPoolSpec_To_v1alpha1_InClusterIPPoolSpec(in *v1alpha2.InClusterIPPoolSpec, out *InClusterIPPoolSpec, s conversion.Scope) error {
	return autoConvert_v1alpha2_InClusterIPPoolSpec_To_v1alpha1_InClusterIPPoolSpec(in, out, s)
}
