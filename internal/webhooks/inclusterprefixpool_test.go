/*
Copyright 2026 The Kubernetes Authors.

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

package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/index"
)

func TestInClusterPrefixPoolInvalidScenarios(t *testing.T) {
	tests := []struct {
		name  string
		spec  v1alpha2.InClusterPrefixPoolSpec
		error string
	}{
		{
			name:  "prefixes required",
			spec:  v1alpha2.InClusterPrefixPoolSpec{},
			error: "prefixes is required",
		},
		{
			name:  "cidr only",
			spec:  v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::1"}},
			error: "provided entry is not a valid IPv6 CIDR",
		},
		{
			name:  "cidr must be network aligned",
			spec:  v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::1/64"}},
			error: "CIDR must be network-aligned",
		},
		{
			name:  "prefix length must be <= allocationPrefixLength",
			spec:  v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::/65"}},
			error: "CIDR prefix length must be <= allocationPrefixLength",
		},
		{
			name:  "ipv4 prefixes rejected",
			spec:  v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"10.0.0.0/24"}},
			error: "only IPv6 prefixes are supported",
		},
		{
			name:  "gateway must be ipv6",
			spec:  v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::/64"}, Gateway: "10.0.0.1"},
			error: "provided gateway must be IPv6",
		},
		{
			name:  "excludedPrefixes must be ipv6",
			spec:  v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::/64"}, ExcludedPrefixes: []string{"10.0.0.1"}},
			error: "only IPv6 excluded prefixes are supported",
		},
		{
			name: "disjoint prefixes accepted",
			spec: v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::/64", "fd01::/64"}},
		},
		{
			name: "custom allocationPrefixLength works",
			spec: v1alpha2.InClusterPrefixPoolSpec{
				Prefixes:               []string{"fd00::/48"},
				AllocationPrefixLength: 56,
			},
		},
		{
			name: "allocationPrefixLength=96 accepted",
			spec: v1alpha2.InClusterPrefixPoolSpec{
				Prefixes:               []string{"fd00::/48"},
				AllocationPrefixLength: 96,
			},
		},
		{
			name: "allocationPrefixLength=128 accepted (/128 loopback)",
			spec: v1alpha2.InClusterPrefixPoolSpec{
				Prefixes:               []string{"fd00::/48"},
				AllocationPrefixLength: 128,
			},
		},
		{
			name: "allocationPrefixLength=64 accepted",
			spec: v1alpha2.InClusterPrefixPoolSpec{
				Prefixes:               []string{"fd00::/48"},
				AllocationPrefixLength: 64,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()
			webhook := newPrefixWebhook(t)
			pool := &v1alpha2.InClusterPrefixPool{
				TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec:       tt.spec,
			}

			if tt.error == "" {
				_, err := webhook.ValidateCreate(ctx, pool)
				g.Expect(err).NotTo(HaveOccurred())
			} else {
				_, err := webhook.ValidateCreate(ctx, pool)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.error))
			}
		})
	}
}

func TestInClusterPrefixPoolDeleteValidation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	namespacedPool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "my-prefix-pool", Namespace: "ns1"},
		Spec:       v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::/64"}},
	}
	globalPool := &v1alpha2.GlobalInClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: globalInClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "my-global-prefix-pool"},
		Spec:       v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::/64"}},
	}

	webhook := newPrefixWebhook(t,
		createPrefixIP("ip-1", "ns1", "fd00::", 64, inClusterPrefixPoolKind, namespacedPool.Name),
		createPrefixIP("ip-2", "ns1", "fd00::", 64, globalInClusterPrefixPoolKind, globalPool.Name),
	)

	_, err := webhook.ValidateDelete(ctx, namespacedPool)
	g.Expect(err).To(HaveOccurred())
	_, err = webhook.ValidateDelete(ctx, globalPool)
	g.Expect(err).To(HaveOccurred())

	g.Expect(webhook.Client.(client.Client).DeleteAllOf(ctx, &ipamv1.IPAddress{})).To(Succeed())

	_, err = webhook.ValidateDelete(ctx, namespacedPool)
	g.Expect(err).NotTo(HaveOccurred())
	_, err = webhook.ValidateDelete(ctx, globalPool)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestInClusterPrefixPoolDeleteSkipAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pool := &v1alpha2.InClusterPrefixPool{
		TypeMeta: metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-prefix-pool",
			Namespace: "ns1",
			Annotations: map[string]string{
				SkipValidateDeleteWebhookAnnotation: "",
			},
		},
		Spec: v1alpha2.InClusterPrefixPoolSpec{Prefixes: []string{"fd00::/64"}},
	}

	webhook := newPrefixWebhook(t,
		createPrefixIP("ip-1", "ns1", "fd00::", 64, inClusterPrefixPoolKind, pool.Name),
	)

	_, err := webhook.ValidateDelete(ctx, pool)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestInClusterPrefixPoolUpdateRejectsOrphanedAllocations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	oldPool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "my-prefix-pool", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd00::/62"},
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.Prefixes = []string{"fd00:0:0:1::/64"}

	webhook := newPrefixWebhook(t,
		createPrefixIP("ip-1", "ns1", "fd00::", 64, inClusterPrefixPoolKind, oldPool.Name),
	)

	_, err := webhook.ValidateUpdate(ctx, oldPool, newPool)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("existing allocations become non-allocatable"))
}

func TestGlobalInClusterPrefixPoolUpdateRejectsOrphanedAllocations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	oldPool := &v1alpha2.GlobalInClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: globalInClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "my-global-prefix-pool"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd10::/62"},
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.Prefixes = []string{"fd10:0:0:1::/64"}

	webhook := newPrefixWebhook(t,
		createPrefixIP("ip-1", "ns1", "fd10::", 64, globalInClusterPrefixPoolKind, oldPool.Name),
	)

	_, err := webhook.ValidateUpdate(ctx, oldPool, newPool)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring(errNonAllocatableAllocations))
}

func TestInClusterPrefixPoolUpdateAllowsPolicyChangesForInRangeAllocations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	oldPool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "my-prefix-pool", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd20::/62"},
		},
	}
	newPool := oldPool.DeepCopy()
	// Exclude an unallocated prefix (fd20:0:0:2::/64) and set a gateway.
	// Existing allocation fd20::/64 remains allocatable, so update is allowed.
	newPool.Spec.ExcludedPrefixes = []string{"fd20:0:0:2::/64"}
	newPool.Spec.Gateway = "fd20:0:0:1::1"

	webhook := newPrefixWebhook(t,
		createPrefixIP("ip-1", "ns1", "fd20::", 64, inClusterPrefixPoolKind, oldPool.Name),
	)

	_, err := webhook.ValidateUpdate(ctx, oldPool, newPool)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestInClusterPrefixPoolUpdateRejectsAllocationPrefixLengthChange(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	oldPool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "my-prefix-pool", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes:               []string{"fd00::/48"},
			AllocationPrefixLength: 56,
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.AllocationPrefixLength = 64

	webhook := newPrefixWebhook(t)

	_, err := webhook.ValidateUpdate(ctx, oldPool, newPool)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("spec.allocationPrefixLength is immutable"))
}

func TestInClusterPrefixPoolUpdateRejectsExcludedPrefixesOverlappingExistingAllocations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	oldPool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "my-prefix-pool", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd30::/62"},
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.ExcludedPrefixes = []string{"fd30::/64"}

	webhook := newPrefixWebhook(t,
		createPrefixIP("ip-1", "ns1", "fd30::", 64, inClusterPrefixPoolKind, oldPool.Name),
	)

	_, err := webhook.ValidateUpdate(ctx, oldPool, newPool)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring(errNonAllocatableAllocations))
}

func TestInClusterPrefixPoolCreateWarnsOn128WithGateway(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	webhook := newPrefixWebhook(t)
	pool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "warn-pool", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes:               []string{"fd40::/64"},
			AllocationPrefixLength: 128,
			Gateway:                "fd40::1",
		},
	}

	warnings, err := webhook.ValidateCreate(ctx, pool)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).NotTo(BeEmpty())
	g.Expect(warnings).To(ContainElement(warnPrefixLength))
	g.Expect(warnings).To(ContainElement(warnGatewayCollision))
}

func TestInClusterPrefixPoolUpdateWarnsOn128WithGateway(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	webhook := newPrefixWebhook(t)
	oldPool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "warn-pool", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes:               []string{"fd40::/64"},
			AllocationPrefixLength: 128,
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.Gateway = "fd40::1"

	warnings, err := webhook.ValidateUpdate(ctx, oldPool, newPool)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).NotTo(BeEmpty())
	g.Expect(warnings).To(ContainElement(warnPrefixLength))
	g.Expect(warnings).To(ContainElement(warnGatewayCollision))
}

func TestInClusterPrefixPoolCreateNoWarningWhenGatewayOutsidePrefixes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	webhook := newPrefixWebhook(t)
	pool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "warn-pool-outside", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd50::/64"},
			Gateway:  "fd51::1",
		},
	}

	warnings, err := webhook.ValidateCreate(ctx, pool)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).To(BeEmpty())
}

func TestInClusterPrefixPoolUpdateNoWarningWhenGatewayOutsidePrefixes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	webhook := newPrefixWebhook(t)
	oldPool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "warn-pool-outside", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd50::/64"},
		},
	}
	newPool := oldPool.DeepCopy()
	newPool.Spec.Gateway = "fd51::1"

	warnings, err := webhook.ValidateUpdate(ctx, oldPool, newPool)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).To(BeEmpty())
}

func TestInClusterPrefixPoolCreateWarnsOn128WithoutGateway(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	webhook := newPrefixWebhook(t)
	pool := &v1alpha2.InClusterPrefixPool{
		TypeMeta:   metav1.TypeMeta{Kind: inClusterPrefixPoolKind, APIVersion: v1alpha2.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "warn-pool-128", Namespace: "ns1"},
		Spec: v1alpha2.InClusterPrefixPoolSpec{
			Prefixes:               []string{"fd60::/64"},
			AllocationPrefixLength: 128,
		},
	}

	warnings, err := webhook.ValidateCreate(ctx, pool)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(warnings).To(ContainElement(warnPrefixLength))
}

func newPrefixWebhook(t *testing.T, objects ...client.Object) *InClusterPrefixPool {
	t.Helper()
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())

	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&ipamv1.IPAddress{}, index.IPAddressPoolRefCombinedField, index.IPAddressByCombinedPoolRef)
	if len(objects) > 0 {
		builder = builder.WithObjects(objects...)
	}

	return &InClusterPrefixPool{Client: builder.Build()}
}

func createPrefixIP(name, namespace, address string, prefix int32, poolKind, poolName string) *ipamv1.IPAddress {
	return &ipamv1.IPAddress{
		TypeMeta: metav1.TypeMeta{Kind: "IPAddress", APIVersion: ipamv1.GroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ipamv1.IPAddressSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: v1alpha2.GroupVersion.Group,
				Kind:     poolKind,
				Name:     poolName,
			},
			Address: address,
			Prefix:  ptr.To(prefix),
		},
	}
}
