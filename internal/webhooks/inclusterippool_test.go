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

package webhooks

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/index"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

func TestPoolDeletionWithExistingIPAddresses(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())

	namespacedPool := &v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    24,
			Gateway:   "10.0.0.1",
		},
	}

	globalPool := &v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    24,
			Gateway:   "10.0.0.1",
		},
	}

	ips := []client.Object{
		createIP("my-ip", "10.0.0.10", namespacedPool),
		createIP("my-ip-2", "10.0.0.10", globalPool),
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ips...).
		WithIndex(&ipamv1.IPAddress{}, index.IPAddressPoolRefCombinedField, index.IPAddressByCombinedPoolRef).
		Build()

	webhook := InClusterIPPool{
		Client: fakeClient,
	}

	g.Expect(webhook.ValidateDelete(ctx, namespacedPool)).Error().To(HaveOccurred(), "should not allow deletion when claims exist")
	g.Expect(webhook.ValidateDelete(ctx, globalPool)).Error().To(HaveOccurred(), "should not allow deletion when claims exist")

	g.Expect(fakeClient.DeleteAllOf(ctx, &ipamv1.IPAddress{})).To(Succeed())

	g.Expect(webhook.ValidateDelete(ctx, namespacedPool)).Error().NotTo(HaveOccurred(), "should allow deletion when no claims exist")
	g.Expect(webhook.ValidateDelete(ctx, globalPool)).Error().NotTo(HaveOccurred(), "should allow deletion when no claims exist")
}

func TestUpdatingPoolInUseAddresses(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())

	namespacedPool := &v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    24,
			Gateway:   "10.0.0.1",
		},
	}

	globalPool := &v1alpha2.GlobalInClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    24,
			Gateway:   "10.0.0.1",
		},
	}

	ips := []client.Object{
		createIP("my-ip", "10.0.0.10", namespacedPool),
		createIP("my-ip-2", "10.0.0.10", globalPool),
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ips...).
		WithIndex(&ipamv1.IPAddress{}, index.IPAddressPoolRefCombinedField, index.IPAddressByCombinedPoolRef).
		Build()

	webhook := InClusterIPPool{
		Client: fakeClient,
	}

	oldNamespacedPool := namespacedPool.DeepCopyObject()
	oldGlobalPool := globalPool.DeepCopyObject()
	namespacedPool.Spec.Addresses = []string{"10.0.0.15-10.0.0.20"}
	globalPool.Spec.Addresses = []string{"10.0.0.15-10.0.0.20"}

	g.Expect(webhook.ValidateUpdate(ctx, oldNamespacedPool, namespacedPool)).Error().To(HaveOccurred(), "should not allow removing in use IPs from addresses field in pool")
	g.Expect(webhook.ValidateUpdate(ctx, oldGlobalPool, globalPool)).Error().To(HaveOccurred(), "should not allow removing in use IPs from addresses field in pool")
}

func TestDeleteSkip(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())

	namespacedPool := &v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
			Annotations: map[string]string{
				SkipValidateDeleteWebhookAnnotation: "",
			},
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    24,
			Gateway:   "10.0.0.1",
		},
	}

	globalPool := &v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
			Annotations: map[string]string{
				SkipValidateDeleteWebhookAnnotation: "",
			},
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    24,
			Gateway:   "10.0.0.1",
		},
	}

	ips := []client.Object{
		createIP("my-ip", "", namespacedPool),
		createIP("my-ip-2", "", globalPool),
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ips...).
		WithIndex(&ipamv1.IPAddress{}, index.IPAddressPoolRefCombinedField, index.IPAddressByCombinedPoolRef).
		Build()

	webhook := InClusterIPPool{
		Client: fakeClient,
	}

	g.Expect(webhook.ValidateDelete(ctx, namespacedPool)).Error().NotTo(HaveOccurred())
	g.Expect(webhook.ValidateDelete(ctx, globalPool)).Error().NotTo(HaveOccurred())
}

func TestInClusterIPPoolDefaulting(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name   string
		spec   v1alpha2.InClusterIPPoolSpec
		expect v1alpha2.InClusterIPPoolSpec
		errors bool
	}{
		{
			name: "addresses with gateway and prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Gateway: "10.0.0.24",
				Prefix:  28,
			},
			expect: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Gateway: "10.0.0.24",
				Prefix:  28,
			},
		},
		{
			name: "addresses with prefix and no gateway",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Prefix: 28,
			},
			expect: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Prefix: 28,
			},
		},
		{
			name: "addresses with prefix and excluded addresses",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Prefix: 28,
				ExcludedAddresses: []string{
					"10.0.0.23",
					"10.0.0.24",
				},
			},
			expect: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Prefix: 28,
				ExcludedAddresses: []string{
					"10.0.0.23",
					"10.0.0.24",
				},
			},
		},
		{
			name: "IPv6 addresses with gateway and prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fe01::2",
					"fe01::3-fe01::5",
					"fe01::1:1/126",
				},
				Gateway: "fe01::1",
				Prefix:  28,
			},
			expect: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fe01::2",
					"fe01::3-fe01::5",
					"fe01::1:1/126",
				},
				Gateway: "fe01::1",
				Prefix:  28,
			},
		},
		{
			name: "IPv6 addresses and prefix with no gateway",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fe01::2",
					"fe01::3-fe01::5",
					"fe01::1:1/126",
				},
				Prefix: 28,
			},
			expect: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fe01::2",
					"fe01::3-fe01::5",
					"fe01::1:1/126",
				},
				Prefix: 28,
			},
		},
		{
			name: "IPv6 addresses and prefix and excluded addresses",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fe01::2",
					"fe01::3-fe01::5",
					"fe01::1:1/126",
				},
				Prefix: 28,
				ExcludedAddresses: []string{
					"fe01::4",
					"fe01::6-fe01::8",
					"fe01::1:10/126",
				},
			},
			expect: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fe01::2",
					"fe01::3-fe01::5",
					"fe01::1:1/126",
				},
				Prefix: 28,
				ExcludedAddresses: []string{
					"fe01::4",
					"fe01::6-fe01::8",
					"fe01::1:10/126",
				},
			},
		},
	}

	for _, tt := range tests {
		namespacedPool := &v1alpha2.InClusterIPPool{Spec: tt.spec}
		globalPool := &v1alpha2.GlobalInClusterIPPool{Spec: tt.spec}

		scheme := runtime.NewScheme()
		g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())
		webhook := InClusterIPPool{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithIndex(&ipamv1.IPAddress{}, index.IPAddressPoolRefCombinedField, index.IPAddressByCombinedPoolRef).
				Build(),
		}

		t.Run(tt.name, customDefaultValidateTest(ctx, namespacedPool.DeepCopyObject(), &webhook))
		g.Expect(webhook.Default(ctx, namespacedPool)).To(Succeed())
		g.Expect(namespacedPool.Spec).To(Equal(tt.expect))

		t.Run(tt.name, customDefaultValidateTest(ctx, globalPool.DeepCopyObject(), &webhook))
		g.Expect(webhook.Default(ctx, globalPool)).To(Succeed())
		g.Expect(globalPool.Spec).To(Equal(tt.expect))
	}
}

type invalidScenarioTest struct {
	testcase      string
	spec          v1alpha2.InClusterIPPoolSpec
	expectedError string
}

func TestInvalidScenarios(t *testing.T) {
	tests := []invalidScenarioTest{
		{
			testcase: "addresses must be set",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{},
				Prefix:    24,
				Gateway:   "10.0.0.1",
			},
			expectedError: "addresses is required",
		},
		{
			testcase: "invalid gateway should not be allowed",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Prefix:  24,
				Gateway: "invalid",
			},
			expectedError: "spec.gateway: Invalid value: \"invalid\": ParseAddr(\"invalid\"): unable to parse IP",
		},
		{
			testcase: "specifying an address that belongs to separate subnets should not be allowed",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.1.27",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "specifying an address that is invalid",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"garbage",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address is not a valid IP, range, nor CIDR",
		},
		{
			testcase: "negative prefix not allowed",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
				},
				Prefix:  -1,
				Gateway: "10.0.0.1",
			},
			expectedError: "a valid prefix is required",
		},
		{
			testcase: "specifying an invalid prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
				},
				Prefix:  9999,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided prefix is not valid",
		},
		{
			testcase: "address range is out of order",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.10-10.0.0.5",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address is not a valid IP, range, nor CIDR",
		},
		{
			testcase: "CIDR address has invalid prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.10/33",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address is not a valid IP, range, nor CIDR",
		},
		{
			testcase: "address range is below Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.0.10-10.0.0.20",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "address range is above Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.2.10-10.0.2.20",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "address range start is below Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.0.20-10.0.1.20",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "address range end is above Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.1.20-10.0.2.20",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "CIDR is below Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.0.1/24",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "CIDR is above Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.2.1/24",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "CIDR start is below Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.0.0/23",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "CIDR end is above Prefix",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.1.0/23",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
		{
			testcase: "Addresses are IPv4 and Gateway is IPv6",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.0.2-10.0.0.250",
				},
				Prefix:  24,
				Gateway: "fd00::1",
			},
			expectedError: "provided gateway and addresses are of mixed IP families",
		},
		{
			testcase: "Addresses are IPv6 and Gateway is IPv4",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fd00::1",
					"fd00::100-fd00::200",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided gateway and addresses are of mixed IP families",
		},
		{
			testcase: "Addresses is using mismatched IP families",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"fd00::1",
					"fd00::100-fd00::200",
					"10.0.1.0",
					"10.0.0.2-10.0.0.250",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided addresses are of mixed IP families",
		},
		{
			testcase: "Excluded addresses are using mismatched IP families",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.0.2-10.0.0.250",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
				ExcludedAddresses: []string{
					"fd00::1",
					"10.0.0.4",
				},
			},
			expectedError: "provided addresses are of mixed IP families",
		},
		{
			testcase: "Addresses and excluded addresses are using mismatched IP families",
			spec: v1alpha2.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.0.2-10.0.0.250",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
				ExcludedAddresses: []string{
					"fd00::1",
				},
			},
			expectedError: "addresses and excluded addresses are of mixed IP families",
		},
	}
	for _, tt := range tests {
		namespacedPool := &v1alpha2.InClusterIPPool{Spec: tt.spec}
		globalPool := &v1alpha2.GlobalInClusterIPPool{Spec: tt.spec}

		g := NewWithT(t)
		scheme := runtime.NewScheme()
		g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())

		webhook := InClusterIPPool{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithIndex(&ipamv1.IPAddress{}, index.IPAddressPoolRefCombinedField, index.IPAddressByCombinedPoolRef).
				Build(),
		}
		runInvalidScenarioTests(t, tt, namespacedPool, webhook)
		runInvalidScenarioTests(t, tt, globalPool, webhook)
	}
}

func TestIPPool_Prefix(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(ipamv1.AddToScheme(scheme)).To(Succeed())

	namespacedPool := &v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    0,
			Gateway:   "10.0.0.1",
		},
	}

	globalPool := &v1alpha2.GlobalInClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Prefix:    0,
			Gateway:   "10.0.0.1",
		},
	}
	emptyPrefixPool := &v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-pool",
		},
		Spec: v1alpha2.InClusterIPPoolSpec{
			Addresses: []string{"10.0.0.10-10.0.0.20"},
			Gateway:   "10.0.0.1",
		},
	}

	ips := []client.Object{
		createIP("my-ip", "10.0.0.10", namespacedPool),
		createIP("my-ip-2", "10.0.0.10", globalPool),
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ips...).
		WithIndex(&ipamv1.IPAddress{}, index.IPAddressPoolRefCombinedField, index.IPAddressByCombinedPoolRef).
		Build()

	webhook := InClusterIPPool{
		Client: fakeClient,
	}

	g.Expect(webhook.validate(&v1alpha2.InClusterIPPool{}, namespacedPool)).Error().NotTo(HaveOccurred(), "should allow /0 prefix InClusterIPPool")
	g.Expect(webhook.validate(&v1alpha2.GlobalInClusterIPPool{}, globalPool)).Error().NotTo(HaveOccurred(), "should allow /0 prefix GlobalInClusterIPPool")
	g.Expect(webhook.validate(&v1alpha2.InClusterIPPool{}, emptyPrefixPool)).Error().NotTo(HaveOccurred(), "should allow empty prefix InClusterIPPool")
}

func runInvalidScenarioTests(t *testing.T, tt invalidScenarioTest, pool types.GenericInClusterPool, webhook InClusterIPPool) {
	t.Helper()
	t.Run(tt.testcase, func(t *testing.T) {
		t.Run("create", func(t *testing.T) {
			t.Helper()

			g := NewWithT(t)
			g.Expect(testCreate(t.Context(), pool, &webhook)).
				Error().
				To(MatchError(ContainSubstring(tt.expectedError)))
		})
		t.Run("update", func(t *testing.T) {
			t.Helper()

			g := NewWithT(t)
			g.Expect(testUpdate(t.Context(), pool, &webhook)).
				Error().
				To(MatchError(ContainSubstring(tt.expectedError)))
		})
		t.Run("delete", func(t *testing.T) {
			t.Helper()

			g := NewWithT(t)
			g.Expect(testDelete(t.Context(), pool, &webhook)).
				Error().
				To(Succeed())
		})
	})
}

func testCreate(ctx context.Context, obj runtime.Object, webhook customDefaulterValidator) (admission.Warnings, error) {
	createCopy := obj.DeepCopyObject()
	if err := webhook.Default(ctx, createCopy); err != nil {
		return nil, err
	}
	return webhook.ValidateCreate(ctx, createCopy)
}

func testDelete(ctx context.Context, obj runtime.Object, webhook customDefaulterValidator) (admission.Warnings, error) {
	deleteCopy := obj.DeepCopyObject()
	if err := webhook.Default(ctx, deleteCopy); err != nil {
		return nil, err
	}
	return webhook.ValidateDelete(ctx, deleteCopy)
}

func testUpdate(ctx context.Context, obj runtime.Object, webhook customDefaulterValidator) (admission.Warnings, error) {
	updateCopy := obj.DeepCopyObject()
	updatedCopy := obj.DeepCopyObject()
	err := webhook.Default(ctx, updateCopy)
	if err != nil {
		return nil, err
	}
	err = webhook.Default(ctx, updatedCopy)
	if err != nil {
		return nil, err
	}
	return webhook.ValidateUpdate(ctx, updateCopy, updatedCopy)
}

func createIP(name string, ip string, pool types.GenericInClusterPool) *ipamv1.IPAddress {
	return &ipamv1.IPAddress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IPAddress",
			APIVersion: "ipam.cluster.x-k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: ipamv1.IPAddressSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: pool.GetObjectKind().GroupVersionKind().Group,
				Kind:     pool.GetObjectKind().GroupVersionKind().Kind,
				Name:     pool.GetName(),
			},
			Address: ip,
		},
	}
}
