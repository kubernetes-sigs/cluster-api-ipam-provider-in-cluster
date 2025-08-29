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

package predicates

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestClaimReferencesPoolKind(t *testing.T) {
	tests := []struct {
		name   string
		ref    ipamv1.IPPoolReference
		result bool
	}{
		{
			name: "true for valid reference",
			ref: ipamv1.IPPoolReference{
				APIGroup: "ipam.cluster.x-k8s.io",
				Kind:     "InClusterIPPool",
			},
			result: true,
		},
		{
			name: "false when kind does not match",
			ref: ipamv1.IPPoolReference{
				APIGroup: "ipam.cluster.x-k8s.io",
				Kind:     "OutOfClusterIPPool",
			},
			result: false,
		},
		{
			name: "false when no group is set",
			ref: ipamv1.IPPoolReference{
				Kind: "InClusterIPPool",
			},
			result: false,
		},
		{
			name: "false when group does not match",
			ref: ipamv1.IPPoolReference{
				APIGroup: "cluster.x-k8s.io",
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
			claim := &ipamv1.IPAddressClaim{
				Spec: ipamv1.IPAddressClaimSpec{
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
