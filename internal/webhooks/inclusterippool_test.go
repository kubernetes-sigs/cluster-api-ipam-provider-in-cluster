package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
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
	}

	for _, tt := range tests {
		pool := &v1alpha1.InClusterIPPool{Spec: tt.spec}
		webhook := InClusterIPPool{}
		t.Run(tt.name, customDefaultValidateTest(ctx, pool.DeepCopyObject(), &webhook))

		g.Expect(webhook.Default(ctx, pool)).To(Succeed())
		g.Expect(pool.Spec).To(Equal(tt.expect))
	}
}
