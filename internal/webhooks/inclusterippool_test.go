package webhooks

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/types"
)

func TestInClusterIPPoolDefaulting(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name   string
		spec   v1alpha1.InClusterIPPoolSpec
		expect v1alpha1.InClusterIPPoolSpec
		errors bool
	}{
		{
			name: "infer prefix, first and last from subnet",
			spec: v1alpha1.InClusterIPPoolSpec{
				Subnet: "10.0.0.0/24",
			},
			expect: v1alpha1.InClusterIPPoolSpec{
				Subnet: "10.0.0.0/24",
				Prefix: 24,
				First:  "10.0.0.1",
				Last:   "10.0.0.254",
			},
		},
		{
			name: "derive subnet from prefix and first",
			spec: v1alpha1.InClusterIPPoolSpec{
				First:  "10.0.0.25",
				Last:   "10.0.0.30",
				Prefix: 28,
			},
			expect: v1alpha1.InClusterIPPoolSpec{
				First:  "10.0.0.25",
				Last:   "10.0.0.30",
				Prefix: 28,
				Subnet: "10.0.0.16/28",
			},
		},
		{
			name: "addresses with gateway and prefix",
			spec: v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Gateway: "10.0.0.24",
				Prefix:  28,
			},
			expect: v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Gateway: "10.0.0.24",
				Prefix:  28,
			},
		},
	}

	for _, tt := range tests {
		namespacedPool := &v1alpha1.InClusterIPPool{Spec: tt.spec}
		globalPool := &v1alpha1.GlobalInClusterIPPool{Spec: tt.spec}
		webhook := InClusterIPPool{}

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
	spec          v1alpha1.InClusterIPPoolSpec
	expectedError string
}

func TestInvalidScenarios(t *testing.T) {
	tests := []invalidScenarioTest{
		{
			testcase: "specifying addresses and subnet should not allow subnet",
			spec: v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Subnet: "10.0.0.0/24",
			},
			expectedError: "subnet may not be used with addresses",
		},
		{
			testcase: "specifying addresses should not allow first",
			spec: v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				First: "10.0.0.2",
			},
			expectedError: "start may not be used with addresses",
		},
		{
			testcase: "specifying addresses should not allow last",
			spec: v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
					"10.0.0.27",
				},
				Last: "10.0.0.2",
			},
			expectedError: "end may not be used with addresses",
		},
		{
			testcase: "invalid gateway should not be allowed",
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			testcase: "omitting a prefix should not be allowed",
			spec: v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.0.25",
					"10.0.0.26",
				},
				Gateway: "10.0.0.1",
			},
			expectedError: "a valid prefix is required when using addresses",
		},
		{
			testcase: "specifying an invalid prefix",
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
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
			spec: v1alpha1.InClusterIPPoolSpec{
				Addresses: []string{
					"10.0.1.0",
					"10.0.1.0/23",
				},
				Prefix:  24,
				Gateway: "10.0.0.1",
			},
			expectedError: "provided address belongs to a different subnet than others",
		},
	}
	for _, tt := range tests {
		namespacedPool := &v1alpha1.InClusterIPPool{Spec: tt.spec}
		globalPool := &v1alpha1.GlobalInClusterIPPool{Spec: tt.spec}
		webhook := InClusterIPPool{}
		runInvalidScenarioTests(t, tt, namespacedPool, webhook)
		runInvalidScenarioTests(t, tt, globalPool, webhook)
	}
}

func runInvalidScenarioTests(t *testing.T, tt invalidScenarioTest, pool types.GenericInClusterPool, webhook InClusterIPPool) {
	t.Helper()
	t.Run(tt.testcase, func(t *testing.T) {
		t.Run("create", func(t *testing.T) {
			t.Helper()

			g := NewWithT(t)
			g.Expect(testCreate(context.Background(), pool, &webhook)).
				To(MatchError(ContainSubstring(tt.expectedError)))
		})
		t.Run("update", func(t *testing.T) {
			t.Helper()

			g := NewWithT(t)
			g.Expect(testUpdate(context.Background(), pool, &webhook)).
				To(MatchError(ContainSubstring(tt.expectedError)))
		})
		t.Run("delete", func(t *testing.T) {
			t.Helper()

			g := NewWithT(t)
			g.Expect(testDelete(context.Background(), pool, &webhook)).
				To(Succeed())
		})
	})
}

func testCreate(ctx context.Context, obj runtime.Object, webhook customDefaulterValidator) error {
	createCopy := obj.DeepCopyObject()
	if err := webhook.Default(ctx, createCopy); err != nil {
		return err
	}
	return webhook.ValidateCreate(ctx, createCopy)
}

func testDelete(ctx context.Context, obj runtime.Object, webhook customDefaulterValidator) error {
	deleteCopy := obj.DeepCopyObject()
	if err := webhook.Default(ctx, deleteCopy); err != nil {
		return err
	}
	return webhook.ValidateDelete(ctx, deleteCopy)
}

func testUpdate(ctx context.Context, obj runtime.Object, webhook customDefaulterValidator) error {
	updateCopy := obj.DeepCopyObject()
	updatedCopy := obj.DeepCopyObject()
	err := webhook.Default(ctx, updateCopy)
	if err != nil {
		return err
	}
	err = webhook.Default(ctx, updatedCopy)
	if err != nil {
		return err
	}
	return webhook.ValidateUpdate(ctx, updateCopy, updatedCopy)
}
