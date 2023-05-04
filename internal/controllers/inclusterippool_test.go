package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	pooltypes "github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/types"
)

var _ = Describe("IP Pool Reconciler", func() {
	var namespace string
	BeforeEach(func() {
		namespace = createNamespace()
	})

	Describe("Pool usage status", func() {
		const testPool = "test-pool"
		var createdClaimNames []string
		var genericPool pooltypes.GenericInClusterPool

		BeforeEach(func() {
			createdClaimNames = nil
		})

		AfterEach(func() {
			for _, name := range createdClaimNames {
				deleteClaim(name, namespace)
			}
			Expect(k8sClient.Delete(context.Background(), genericPool)).To(Succeed())
		})

		DescribeTable("it shows the total, used, free ip addresses in the pool",
			func(poolType string, addresses []string, gateway string, expectedTotal, expectedUsed, expectedFree int) {
				poolSpec := v1alpha1.InClusterIPPoolSpec{
					Prefix:    24,
					Gateway:   gateway,
					Addresses: addresses,
				}

				switch poolType {
				case "InClusterIPPool":
					genericPool = &v1alpha1.InClusterIPPool{
						ObjectMeta: metav1.ObjectMeta{GenerateName: testPool, Namespace: namespace},
						Spec:       poolSpec,
					}
				case "GlobalInClusterIPPool":
					genericPool = &v1alpha1.GlobalInClusterIPPool{
						ObjectMeta: metav1.ObjectMeta{GenerateName: testPool, Namespace: namespace},
						Spec:       poolSpec,
					}
				default:
					Fail("Unknown pool type")
				}

				Expect(k8sClient.Create(context.Background(), genericPool)).To(Succeed())

				Eventually(Object(genericPool)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Status.Addresses.Total", Equal(expectedTotal)))

				Expect(genericPool.PoolStatus().Addresses.Used).To(Equal(0))
				Expect(genericPool.PoolStatus().Addresses.Free).To(Equal(expectedTotal))

				for i := 0; i < expectedUsed; i++ {
					claim := clusterv1.IPAddressClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test%d", i),
							Namespace: namespace,
						},
						Spec: clusterv1.IPAddressClaimSpec{
							PoolRef: corev1.TypedLocalObjectReference{
								APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
								Kind:     poolType,
								Name:     genericPool.GetName(),
							},
						},
					}
					Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
					createdClaimNames = append(createdClaimNames, claim.Name)
				}

				Eventually(Object(genericPool)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Status.Addresses.Used", Equal(expectedUsed)))
				poolStatus := genericPool.PoolStatus()
				Expect(poolStatus.Addresses.Total).To(Equal(expectedTotal))
				Expect(poolStatus.Addresses.Free).To(Equal(expectedFree))
			},

			Entry("When there is 1 claim and no gateway - InClusterIPPool",
				"InClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "", 11, 1, 10),
			Entry("When there are 2 claims and no gateway - InClusterIPPool",
				"InClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "", 11, 2, 9),
			Entry("When there is 1 claim with gateway in range - InClusterIPPool",
				"InClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 1, 9),
			Entry("When there are 2 claims with gateway in range - InClusterIPPool",
				"InClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 2, 8),
			Entry("When there is 1 claim with gateway outside of range - InClusterIPPool",
				"InClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.1", 11, 1, 10),

			Entry("When there is 1 claim and no gateway - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "", 11, 1, 10),
			Entry("When there are 2 claims and no gateway - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "", 11, 2, 9),
			Entry("When there is 1 claim with gateway in range - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 1, 9),
			Entry("When there are 2 claims with gateway in range - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 2, 8),
			Entry("When there is 1 claim with gateway outside of range - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.1", 11, 1, 10),
		)
	})
})
