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
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	pooltypes "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
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
			func(poolType string, prefix int, addresses []string, gateway string, expectedTotal, expectedUsed, expectedFree int) {
				genericPool = newPool(poolType, testPool, namespace, gateway, addresses, prefix)
				Expect(k8sClient.Create(context.Background(), genericPool)).To(Succeed())

				Eventually(Object(genericPool)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Status.Addresses.Total", Equal(expectedTotal)))

				Expect(genericPool.PoolStatus().Addresses.Used).To(Equal(0))
				Expect(genericPool.PoolStatus().Addresses.Free).To(Equal(expectedTotal))

				for i := 0; i < expectedUsed; i++ {
					claim := ipamv1.IPAddressClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test%d", i),
							Namespace: namespace,
						},
						Spec: ipamv1.IPAddressClaimSpec{
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
				"InClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "", 11, 1, 10),
			Entry("When there are 2 claims and no gateway - InClusterIPPool",
				"InClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "", 11, 2, 9),
			Entry("When there is 1 claim with gateway in range - InClusterIPPool",
				"InClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 1, 9),
			Entry("When there are 2 claims with gateway in range - InClusterIPPool",
				"InClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 2, 8),
			Entry("When there is 1 claim with gateway outside of range - InClusterIPPool",
				"InClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "10.0.0.1", 11, 1, 10),
			Entry("When the addresses range includes network addr, it is not available - InClusterIPPool",
				"InClusterIPPool", 24, []string{"10.0.0.0-10.0.0.1"}, "10.0.0.2", 1, 1, 0),
			Entry("When the addresses range includes broadcast, it is not available - InClusterIPPool",
				"InClusterIPPool", 24, []string{"10.0.0.254-10.0.0.255"}, "10.0.0.1", 1, 1, 0),
			Entry("When the addresses range is IPv6 and the last range in the subnet, it is available - InClusterIPPool",
				"InClusterIPPool", 120, []string{"fe80::ffff"}, "fe80::a", 1, 1, 0),

			Entry("When there is 1 claim and no gateway - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "", 11, 1, 10),
			Entry("When there are 2 claims and no gateway - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "", 11, 2, 9),
			Entry("When there is 1 claim with gateway in range - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 1, 9),
			Entry("When there are 2 claims with gateway in range - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "10.0.0.10", 10, 2, 8),
			Entry("When there is 1 claim with gateway outside of range - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 24, []string{"10.0.0.10-10.0.0.20"}, "10.0.0.1", 11, 1, 10),
			Entry("When the addresses range includes network addr, it is not available - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 24, []string{"10.0.0.0-10.0.0.1"}, "10.0.0.2", 1, 1, 0),
			Entry("When the addresses range includes broadcast, it is not available - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 24, []string{"10.0.0.254-10.0.0.255"}, "10.0.0.1", 1, 1, 0),
			Entry("When the addresses range is IPv6 and the last range in the subnet, it is available - GlobalInClusterIPPool",
				"GlobalInClusterIPPool", 120, []string{"fe80::ffff"}, "fe80::a", 1, 1, 0),
		)

		DescribeTable("it shows the out of range ips if any",
			func(poolType string, addresses []string, gateway string, updatedAddresses []string, numClaims, expectedOutOfRange int) {
				poolSpec := v1alpha2.InClusterIPPoolSpec{
					Prefix:                      24,
					Gateway:                     gateway,
					Addresses:                   addresses,
					AllocateReservedIPAddresses: true,
				}

				switch poolType {
				case "InClusterIPPool":
					genericPool = &v1alpha2.InClusterIPPool{
						ObjectMeta: metav1.ObjectMeta{GenerateName: testPool, Namespace: namespace},
						Spec:       poolSpec,
					}
				case "GlobalInClusterIPPool":
					genericPool = &v1alpha2.GlobalInClusterIPPool{
						ObjectMeta: metav1.ObjectMeta{GenerateName: testPool, Namespace: namespace},
						Spec:       poolSpec,
					}
				default:
					Fail("Unknown pool type")
				}

				Expect(k8sClient.Create(context.Background(), genericPool)).To(Succeed())

				for i := 0; i < numClaims; i++ {
					claim := ipamv1.IPAddressClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test%d", i),
							Namespace: namespace,
						},
						Spec: ipamv1.IPAddressClaimSpec{
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
					HaveField("Status.Addresses.Used", Equal(numClaims)))

				genericPool.PoolSpec().Addresses = updatedAddresses
				genericPool.PoolSpec().AllocateReservedIPAddresses = false
				Expect(k8sClient.Update(context.Background(), genericPool)).To(Succeed())

				Eventually(Object(genericPool)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Status.Addresses.OutOfRange", Equal(expectedOutOfRange)))
			},

			Entry("InClusterIPPool",
				"InClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.1", []string{"10.0.0.13-10.0.0.20"}, 5, 3),
			Entry("InClusterIPPool when removing network address",
				"InClusterIPPool", []string{"10.0.0.0-10.0.0.255"}, "10.0.0.1", []string{"10.0.0.1-10.0.0.255"}, 4, 0),
			Entry("InClusterIPPool when removing gateway address",
				"InClusterIPPool", []string{"10.0.0.0-10.0.0.255"}, "10.0.0.1", []string{"10.0.0.0", "10.0.0.2-10.0.0.255"}, 5, 1),
			Entry("InClusterIPPool when removing broadcast address",
				"InClusterIPPool", []string{"10.0.0.251-10.0.0.255"}, "10.0.0.1", []string{"10.0.0.251-10.0.0.254"}, 5, 1),
			Entry("GlobalInClusterIPPool",
				"GlobalInClusterIPPool", []string{"10.0.0.10-10.0.0.20"}, "10.0.0.1", []string{"10.0.0.13-10.0.0.20"}, 5, 3),
			Entry("GlobalInClusterIPPool when removing network address",
				"GlobalInClusterIPPool", []string{"10.0.0.0-10.0.0.255"}, "10.0.0.1", []string{"10.0.0.1-10.0.0.255"}, 4, 0),
			Entry("GlobalInClusterIPPool when removing gateway address",
				"GlobalInClusterIPPool", []string{"10.0.0.0-10.0.0.255"}, "10.0.0.1", []string{"10.0.0.0", "10.0.0.2-10.0.0.255"}, 5, 1),
			Entry("GlobalInClusterIPPool when removing broadcast address",
				"GlobalInClusterIPPool", []string{"10.0.0.251-10.0.0.255"}, "10.0.0.1", []string{"10.0.0.251-10.0.0.254"}, 5, 1),
		)
	})

	Context("when the pool has IPAddresses", func() {
		const poolName = "finalizer-pool-test"

		DescribeTable("add a finalizer to prevent pool deletion before IPAddresses are deleted", func(poolType string) {
			pool := newPool(poolType, poolName, namespace, "10.0.1.2", []string{"10.0.1.1-10.0.1.254"}, 24)
			Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())
			Eventually(Get(pool)).Should(Succeed())

			claim := ipamv1.IPAddressClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "finalizer-pool-test",
					Namespace: namespace,
				},
				Spec: ipamv1.IPAddressClaimSpec{
					PoolRef: corev1.TypedLocalObjectReference{
						APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
						Kind:     poolType,
						Name:     pool.GetName(),
					},
				},
			}

			Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

			addresses := ipamv1.IPAddressList{}
			Eventually(ObjectList(&addresses)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Items", HaveLen(1)))

			Eventually(Object(pool)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("ObjectMeta.Finalizers", ContainElement(ProtectPoolFinalizer)))

			Expect(k8sClient.Delete(context.Background(), pool)).To(Succeed())

			Consistently(Object(pool)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("ObjectMeta.Finalizers", ContainElement(ProtectPoolFinalizer)))

			deleteClaim("finalizer-pool-test", namespace)

			Eventually(Get(pool)).Should(Not(Succeed()))
		},
			Entry("validates InClusterIPPool", "InClusterIPPool"),
			Entry("validates GlobalInClusterIPPool", "GlobalInClusterIPPool"),
		)
	})
})

func newPool(poolType, generateName, namespace, gateway string, addresses []string, prefix int) pooltypes.GenericInClusterPool {
	poolSpec := v1alpha2.InClusterIPPoolSpec{
		Prefix:    prefix,
		Gateway:   gateway,
		Addresses: addresses,
	}

	switch poolType {
	case "InClusterIPPool":
		return &v1alpha2.InClusterIPPool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: generateName, Namespace: namespace},
			Spec:       poolSpec,
		}
	case "GlobalInClusterIPPool":
		return &v1alpha2.GlobalInClusterIPPool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: generateName},
			Spec:       poolSpec,
		}
	default:
		Fail("Unknown pool type")
	}

	return nil
}
