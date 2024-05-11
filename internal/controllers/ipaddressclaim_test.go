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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
)

var IgnoreUIDsOnIPAddress = IgnorePaths{
	"TypeMeta",
	"ObjectMeta.OwnerReferences[0].UID",
	"ObjectMeta.OwnerReferences[1].UID",
	"ObjectMeta.OwnerReferences[2].UID",
	"Spec.Claim.UID",
	"Spec.Pool.UID",
}

var _ = Describe("IPAddressClaimReconciler", func() {
	var (
		namespace  string
		namespace2 string
	)
	BeforeEach(func() {
		namespace = createNamespace()
		namespace2 = createNamespace()
	})

	Context("When a new IPAddressClaim is created", func() {
		When("the referenced pool is an unrecognized kind", func() {
			const poolName = "unknown-pool"

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.1.1-10.0.1.254"},
						Prefix:    24,
						Gateway:   "10.0.1.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteNamespacedPool(poolName, namespace)
				deleteClaim("unknown-pool-test", namespace)
			})

			It("should ignore the claim", func() {
				claim := newClaim("unknown-pool-test", namespace, "UnknownIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})
		})

		When("the referenced namespaced pool exists", func() {
			const (
				clusterName = "test-cluster"
				poolName    = "test-pool"
			)

			BeforeEach(func() {
				cluster := clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
					Spec: clusterv1.ClusterSpec{
						Paused: false,
					},
				}
				Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
				Eventually(Get(&cluster)).Should(Succeed())

				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.1-10.0.0.254"},
						Prefix:    24,
						Gateway:   "10.0.0.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteNamespacedPool(poolName, namespace)

				cluster := clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
				}
				ExpectWithOffset(1, k8sClient.Delete(context.Background(), &cluster)).To(Succeed())
				EventuallyWithOffset(1, Get(&cluster)).Should(Not(Succeed()))
			})

			It("should allocate an Address from the Pool", func() {
				claim := newClaim("test", namespace, "InClusterIPPool", poolName)
				claim.Labels = map[string]string{clusterv1.ClusterNameLabel: clusterName}
				expectedIPAddress := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "InClusterIPPool",
								Name:               poolName,
							},
						},
						Labels: map[string]string{clusterv1.ClusterNameLabel: clusterName},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.1",
						Prefix:  24,
						Gateway: "10.0.0.2",
					},
				}

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				Eventually(findAddress("test", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)
			})
		})

		When("the referenced namespaced pool does not exists", func() {
			const wrongPoolName = "wrong-test-pool"
			const poolName = "test-pool"

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.1-10.0.0.254"},
						Prefix:    24,
						Gateway:   "10.0.0.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("should not allocate an Address from the Pool", func() {
				claim := newClaim("test", namespace, "InClusterIPPool", wrongPoolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})
		})
		When("the referenced namespaced pool has addresses that overlap with reserved addresses", func() {
			const poolName = "test-pool"

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Prefix:  24,
						Gateway: "10.0.1.1",
						Addresses: []string{
							"10.0.1.0", // reserved network addr
							"10.0.1.1", // reserved gateway addr
							"10.0.1.2",
							"10.0.1.255", // reserved broadcast addr
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteClaim("test2", namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("does no allocate the reserved addresses", func() {
				claim := newClaim("test", namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				expectedIPAddress := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "InClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.1.2",
						Prefix:  24,
						Gateway: "10.0.1.1",
					},
				}

				Eventually(findAddress("test", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)

				claim2 := newClaim("test2", namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

				// verify none of the reserved addresses are allocated
				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(1)))
			})
		})

		When("the referenced namespaced pool exists and has excluded ip addresses", func() {
			const (
				poolName = "test-pool-excluded"
				first    = "test-excluded-first"
				second   = "test-excluded-second"
				third    = "test-excluded-third"
			)

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Prefix:  24,
						Gateway: "10.1.1.1",
						Addresses: []string{
							"10.1.1.0/24",
						},
						ExcludedAddresses: []string{
							"10.1.1.2-10.1.1.5",
							"10.1.1.7",
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim(first, namespace)
				deleteClaim(second, namespace)
				deleteClaim(third, namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("should not allocate excluded ip addresses", func() {
				claim := newClaim(first, namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				Eventually(findAddress(first, namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Spec.Address", "10.1.1.6"),
				)

				claim1 := newClaim(second, namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())

				Eventually(findAddress(second, namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Spec.Address", "10.1.1.8"),
				)

				claim2 := newClaim(third, namespace, "InClusterIPPool", poolName)

				Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())
				Eventually(findAddress(third, namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Spec.Address", "10.1.1.9"),
				)
			})
		})

		When("the referenced namespaced pool has out of order Addresses", func() {
			const poolName = "test-pool"

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Prefix:  24,
						Gateway: "10.0.1.1",
						Addresses: []string{
							"10.0.1.3",
							"10.0.1.2",
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("should allocate the lowest available Address from the Pool, regardless of Addresses order", func() {
				claim := newClaim("test", namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				Eventually(findAddress("test", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Spec.Address", "10.0.1.2"),
				)
			})
		})

		When("the referenced namespaced pool does not contain a gateway", func() {
			const poolName = "test-pool"

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.1-10.0.0.254"},
						Prefix:    24,
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("should allocate an Address from the Pool", func() {
				claim := newClaim("test", namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				expectedIPAddress := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "InClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.1",
						Prefix:  24,
					},
				}

				Eventually(findAddress("test", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)
			})
		})

		When("the referenced global pool exists", func() {
			const poolName = "global-pool"
			var secondNamespace string
			BeforeEach(func() {
				pool := v1alpha2.GlobalInClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{ // global pool, no namespace
						Name: poolName,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.2-10.0.0.254"},
						Prefix:    24,
						Gateway:   "10.0.0.1",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
				secondNamespace = createNamespace()
			})

			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteClaim("test-second-namespace", secondNamespace)
				deleteClusterScopedPool(poolName)
			})

			It("should allocate an Address from the Pool, no matter the claim's namespace", func() {
				claim := newClaim("test", namespace, "GlobalInClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				claimFromSecondNamespace := newClaim("test-second-namespace", secondNamespace, "GlobalInClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claimFromSecondNamespace)).To(Succeed())

				expectedIPAddress := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "GlobalInClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "GlobalInClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.2",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				expectedIPAddressInSecondNamespace := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-second-namespace",
						Namespace:  secondNamespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test-second-namespace",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "GlobalInClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test-second-namespace",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "GlobalInClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.3",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				Eventually(findAddress("test", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)

				Eventually(findAddress("test-second-namespace", secondNamespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedIPAddressInSecondNamespace, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)
			})
		})

		When("the referenced global pool does not exists", func() {
			const wrongPoolName = "wrong-test-pool"
			const poolName = "test-pool"

			BeforeEach(func() {
				pool := v1alpha2.GlobalInClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name: poolName,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.2-10.0.0.254"},
						Prefix:    24,
						Gateway:   "10.0.0.1",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteClusterScopedPool(poolName)
			})

			It("should not allocate an Address from the Pool", func() {
				claim := newClaim("test", namespace, "GlobalInClusterIPPool", wrongPoolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})
		})

		When("the referenced pool uses single ip addresses", func() {
			const poolName = "test-pool-single-ip-addresses"
			var claim1, claim2 ipamv1.IPAddressClaim

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{
							"10.0.0.50",
							"10.0.0.128",
						},
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim(claim1.Name, claim1.Namespace)
				deleteClaim(claim2.Name, claim2.Namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("should allocate an Address from the Pool", func() {
				claim1 = newClaim("test-1", namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())

				claim2 = newClaim("test-2", namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

				Eventually(findAddress("test-1", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Spec.Address", "10.0.0.50"))

				Eventually(findAddress("test-2", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Spec.Address", "10.0.0.128"))
			})
		})

		When("there are two pools with the same name in different namespaces", func() {
			const commonPoolName = "common-pool-name"
			var secondNamespace string
			var claim1, claim2 ipamv1.IPAddressClaim

			BeforeEach(func() {
				poolA := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      commonPoolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.50"},
						Prefix:    24,
						Gateway:   "10.0.0.1",
					},
				}
				Expect(k8sClient.Create(context.Background(), &poolA)).To(Succeed())
				Eventually(Get(&poolA)).Should(Succeed())

				secondNamespace = createNamespace()

				poolB := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      commonPoolName,
						Namespace: secondNamespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.50"},
						Prefix:    24,
						Gateway:   "10.0.0.1",
					},
				}
				Expect(k8sClient.Create(context.Background(), &poolB)).To(Succeed())
				Eventually(Get(&poolB)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim(claim1.Name, claim1.Namespace)
				deleteClaim(claim2.Name, claim2.Namespace)
				deleteNamespacedPool(commonPoolName, namespace)
				deleteNamespacedPool(commonPoolName, secondNamespace)
			})

			It("should allocate Addresses from each Pool independently", func() {
				claim1 = newClaim("test-1", namespace, "InClusterIPPool", commonPoolName)
				claim2 = newClaim("test-2", secondNamespace, "InClusterIPPool", commonPoolName)
				Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())
				Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

				expectedAddress1 := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-1",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test-1",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "InClusterIPPool",
								Name:               commonPoolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test-1",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     commonPoolName,
						},
						Address: "10.0.0.50",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				expectedAddress2 := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-2",
						Namespace:  secondNamespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test-2",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "InClusterIPPool",
								Name:               commonPoolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test-2",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     commonPoolName,
						},
						Address: "10.0.0.50",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				Eventually(findAddress("test-1", namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedAddress1, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))

				Eventually(findAddress("test-2", secondNamespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedAddress2, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))
			})
		})

		When("two pools with the same name, one in a namespace and one cluster-scoped, exist", func() {
			const commonPoolName = "comomon-pool-name"
			var claimFromNamespacedPool, claimFromGlobalPool ipamv1.IPAddressClaim

			BeforeEach(func() {
				namespacedPool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      commonPoolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.50"},
						Prefix:    24,
						Gateway:   "10.0.0.1",
					},
				}
				Expect(k8sClient.Create(context.Background(), &namespacedPool)).To(Succeed())
				Eventually(Get(&namespacedPool)).Should(Succeed())

				globalPool := v1alpha2.GlobalInClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{ // global pool, no namespace
						Name: commonPoolName,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.50"},
						Prefix:    24,
						Gateway:   "10.0.0.1",
					},
				}

				Expect(k8sClient.Create(context.Background(), &globalPool)).To(Succeed())
				Eventually(Get(&globalPool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaim(claimFromNamespacedPool.Name, claimFromNamespacedPool.Namespace)
				deleteClaim(claimFromGlobalPool.Name, claimFromGlobalPool.Namespace)
				deleteNamespacedPool(commonPoolName, namespace)
				deleteClusterScopedPool(commonPoolName)
			})

			It("should allocate Addresses from each Pool independently", func() {
				claimFromNamespacedPool = newClaim("test-1", namespace, "InClusterIPPool", commonPoolName)
				claimFromGlobalPool = newClaim("test-2", namespace, "GlobalInClusterIPPool", commonPoolName)

				expectedAddress1 := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-1",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test-1",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "InClusterIPPool",
								Name:               commonPoolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test-1",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     commonPoolName,
						},
						Address: "10.0.0.50",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				expectedAddress2 := ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-2",
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               "test-2",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "GlobalInClusterIPPool",
								Name:               commonPoolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test-2",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "GlobalInClusterIPPool",
							Name:     commonPoolName,
						},
						Address: "10.0.0.50",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				Expect(k8sClient.Create(context.Background(), &claimFromNamespacedPool)).To(Succeed())
				Expect(k8sClient.Create(context.Background(), &claimFromGlobalPool)).To(Succeed())

				Eventually(findAddress(expectedAddress1.GetName(), namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedAddress1, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))

				Eventually(findAddress(expectedAddress2.GetName(), namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedAddress2, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))
			})
		})

		When("the pool is paused", func() {
			When("a claim is created", func() {
				const poolName = "paused-pool"
				var pool v1alpha2.InClusterIPPool

				BeforeEach(func() {
					pool = v1alpha2.InClusterIPPool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      poolName,
							Namespace: namespace,
							Annotations: map[string]string{
								clusterv1.PausedAnnotation: "",
							},
						},
						Spec: v1alpha2.InClusterIPPoolSpec{
							Addresses: []string{"10.0.0.50"},
							Prefix:    24,
							Gateway:   "10.0.1.1",
						},
					}
					Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
					Eventually(Get(&pool)).Should(Succeed())
				})

				AfterEach(func() {
					deleteClaim("paused-pool-test", namespace)
					deleteNamespacedPool(poolName, namespace)
				})

				It("should not create an IPAddress for claims until the pool is unpaused", func() {
					claim := newClaim("paused-pool-test", namespace, "InClusterIPPool", poolName)
					Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

					addresses := ipamv1.IPAddressList{}
					Consistently(ObjectList(&addresses)).
						WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
						HaveField("Items", HaveLen(0)))

					patchHelper, err := patch.NewHelper(&pool, k8sClient)
					Expect(err).NotTo(HaveOccurred())
					delete(pool.Annotations, clusterv1.PausedAnnotation)
					err = patchHelper.Patch(ctx, &pool)
					Expect(err).NotTo(HaveOccurred())

					Eventually(ObjectList(&addresses)).
						WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(
						HaveField("Items", HaveLen(1)))
				})
			})

			When("a claim is deleted", func() {
				const poolName = "paused-delete-claim-pool" // #nosec G101
				var pool v1alpha2.InClusterIPPool

				BeforeEach(func() {
					pool = v1alpha2.InClusterIPPool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      poolName,
							Namespace: namespace,
						},
						Spec: v1alpha2.InClusterIPPoolSpec{
							Addresses: []string{"10.0.20.51"},
							Prefix:    24,
							Gateway:   "10.0.20.1",
						},
					}
					Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
					Eventually(Get(&pool)).Should(Succeed())
				})

				AfterEach(func() {
					deleteNamespacedPool(poolName, namespace)
				})

				It("should prevent deletion of claims", func() {
					claim := newClaim("paused-pool-delete-claim-test", namespace, "InClusterIPPool", poolName)
					Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

					claims := ipamv1.IPAddressClaimList{}
					Eventually(ObjectList(&claims)).
						WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(
						HaveField("Items", HaveLen(1)))

					patchHelper, err := patch.NewHelper(&pool, k8sClient)
					Expect(err).NotTo(HaveOccurred())
					pool.Annotations = map[string]string{
						clusterv1.PausedAnnotation: "",
					}
					err = patchHelper.Patch(ctx, &pool)
					Expect(err).NotTo(HaveOccurred())

					time.Sleep(1 * time.Second)

					Expect(k8sClient.Delete(context.Background(), &claim)).To(Succeed())
					Consistently(ObjectList(&claims)).
						WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
						HaveField("Items", HaveLen(1)))

					patchHelper, err = patch.NewHelper(&pool, k8sClient)
					Expect(err).NotTo(HaveOccurred())
					delete(pool.Annotations, clusterv1.PausedAnnotation)
					err = patchHelper.Patch(ctx, &pool)
					Expect(err).NotTo(HaveOccurred())

					Eventually(ObjectList(&claims)).
						WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(
						HaveField("Items", HaveLen(0)))
				})
			})
		})

		When("the pool is out of available addresses", func() {
			const (
				poolName            = "full-pool"
				existingAddressName = "existing-address"
				newAddressName      = "new-address"
			)

			var expectedIPAddress *ipamv1.IPAddress

			BeforeEach(func() {
				pool := v1alpha2.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.1"},
						Prefix:    24,
						Gateway:   "10.0.0.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())

				expectedIPAddress = &ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       existingAddressName,
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               existingAddressName,
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "InClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: existingAddressName,
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.1",
						Prefix:  24,
						Gateway: "10.0.0.2",
					},
				}

				claim := newClaim(existingAddressName, namespace, "InClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				Eventually(findAddress(existingAddressName, namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)
				addresses := ipamv1.IPAddressList{}
				Expect(ObjectList(&addresses)()).To(HaveField("Items", HaveLen(1)))
			})

			AfterEach(func() {
				deleteClaimUnchecked(existingAddressName, namespace)
				deleteClaim(newAddressName, namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("should not allocate an Address from the Pool", func() {
				claim := newClaim(newAddressName, namespace, "InClusterIPPool", poolName)

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(1)))
			})

			It("should allow new claims to progress once capacity becomes available", func() {
				claim := newClaim(newAddressName, namespace, "InClusterIPPool", poolName)

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(1)))

				// Sleep 5 seconds to give the reconciler time to stop retrying
				// on the newly created claim.
				time.Sleep(5 * time.Second)

				// At this point the claim is still unfulfilled since the
				// existing address is still claimed
				Expect(findAddress(existingAddressName, namespace)()).To(
					EqualObject(expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))

				// Deleting the existing claim should allow the new claim to progress
				deleteClaim(existingAddressName, namespace)

				expectedIPAddress.Name = newAddressName
				expectedIPAddress.OwnerReferences[0].Name = newAddressName
				expectedIPAddress.Spec.ClaimRef.Name = newAddressName

				Eventually(findAddress(newAddressName, namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)
			})
		})

		When("the referenced global pool is out of available addresses", func() {
			const (
				poolName            = "full-global-pool"
				existingAddressName = "existing-address"
				newAddressName      = "new-address"
			)

			var expectedIPAddress *ipamv1.IPAddress

			BeforeEach(func() {
				pool := v1alpha2.GlobalInClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{ // global pool, no namespace
						Name: poolName,
					},
					Spec: v1alpha2.InClusterIPPoolSpec{
						Addresses: []string{"10.0.0.1"},
						Prefix:    24,
						Gateway:   "10.0.0.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())

				expectedIPAddress = &ipamv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       existingAddressName,
						Namespace:  namespace,
						Finalizers: []string{ipamutil.ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(true),
								Kind:               "IPAddressClaim",
								Name:               existingAddressName,
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
								BlockOwnerDeletion: ptr.To(true),
								Controller:         ptr.To(false),
								Kind:               "GlobalInClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: ipamv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: existingAddressName,
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "GlobalInClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.1",
						Prefix:  24,
						Gateway: "10.0.0.2",
					},
				}

				claim := newClaim(existingAddressName, namespace, "GlobalInClusterIPPool", poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				Eventually(findAddress(existingAddressName, namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)
				addresses := ipamv1.IPAddressList{}
				Expect(ObjectList(&addresses)()).To(HaveField("Items", HaveLen(1)))
			})

			AfterEach(func() {
				deleteClaimUnchecked(existingAddressName, namespace)
				deleteClaim(newAddressName, namespace)
				deleteClusterScopedPool(poolName)
			})

			It("should not allocate an Address from the Pool", func() {
				claim := newClaim(newAddressName, namespace, "GlobalInClusterIPPool", poolName)

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(1)))
			})

			It("should allow new claims to progress once capacity becomes available", func() {
				claim := newClaim(newAddressName, namespace, "GlobalInClusterIPPool", poolName)

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(1)))

				// Sleep 5 seconds to give the reconciler time to stop retrying
				// on the newly created claim.
				time.Sleep(5 * time.Second)

				// At this point the claim is still unfulfilled since the
				// existing address is still claimed
				Expect(findAddress(existingAddressName, namespace)()).To(
					EqualObject(expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))

				// Deleting the existing claim should allow the new claim to progress
				deleteClaim(existingAddressName, namespace)

				expectedIPAddress.Name = newAddressName
				expectedIPAddress.OwnerReferences[0].Name = newAddressName
				expectedIPAddress.Spec.ClaimRef.Name = newAddressName

				Eventually(findAddress(newAddressName, namespace)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
				)
			})
		})
	})

	Context("When an existing IPAddress with no ownerReferences is missing finalizers and owner references", func() {
		const poolName = "test-pool"

		BeforeEach(func() {
			pool := v1alpha2.InClusterIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterIPPoolSpec{
					Addresses: []string{"10.0.0.1-10.0.0.254"},
					Prefix:    24,
					Gateway:   "10.0.0.2",
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaim("test", namespace)
			deleteNamespacedPool(poolName, namespace)
		})

		It("should add the owner references and finalizer", func() {
			addressSpec := ipamv1.IPAddressSpec{
				ClaimRef: corev1.LocalObjectReference{
					Name: "test",
				},
				PoolRef: corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
					Kind:     "InClusterIPPool",
					Name:     poolName,
				},
				Address: "10.0.0.1",
				Prefix:  24,
				Gateway: "10.0.0.2",
			}

			address := ipamv1.IPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: namespace,
				},
				Spec: addressSpec,
			}

			Expect(k8sClient.Create(context.Background(), &address)).To(Succeed())

			claim := newClaim("test", namespace, "InClusterIPPool", poolName)
			Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

			expectedIPAddress := ipamv1.IPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  namespace,
					Finalizers: []string{ipamutil.ProtectAddressFinalizer},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(true),
							Kind:               "IPAddressClaim",
							Name:               "test",
						},
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(false),
							Kind:               "InClusterIPPool",
							Name:               poolName,
						},
					},
				},
				Spec: addressSpec,
			}

			Eventually(findAddress("test", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))
		})
	})

	Context("When an existing IPAddress with an unrelated ownerRef is missing finalizers and IPAM owner references", func() {
		const poolName = "test-pool"

		BeforeEach(func() {
			pool := v1alpha2.InClusterIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterIPPoolSpec{
					Addresses: []string{"10.0.0.1-10.0.0.254"},
					Prefix:    24,
					Gateway:   "10.0.0.2",
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaim("test", namespace)
			deleteNamespacedPool(poolName, namespace)
		})

		It("should add the owner references and finalizer", func() {
			addressSpec := ipamv1.IPAddressSpec{
				ClaimRef: corev1.LocalObjectReference{
					Name: "test",
				},
				PoolRef: corev1.TypedLocalObjectReference{
					APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
					Kind:     "InClusterIPPool",
					Name:     poolName,
				},
				Address: "10.0.0.1",
				Prefix:  24,
				Gateway: "10.0.0.2",
			}
			address := ipamv1.IPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "alpha-dummy",
							Kind:       "dummy-kind",
							Name:       "dummy-name",
							UID:        "abc-dummy-123",
						},
					},
				},
				Spec: addressSpec,
			}

			Expect(k8sClient.Create(context.Background(), &address)).To(Succeed())

			claim := newClaim("test", namespace, "InClusterIPPool", poolName)
			Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

			expectedIPAddress := ipamv1.IPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  namespace,
					Finalizers: []string{ipamutil.ProtectAddressFinalizer},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "alpha-dummy",
							Kind:       "dummy-kind",
							Name:       "dummy-name",
							UID:        "abc-dummy-123",
						},
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(true),
							Kind:               "IPAddressClaim",
							Name:               "test",
						},
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(false),
							Kind:               "InClusterIPPool",
							Name:               poolName,
						},
					},
				},
				Spec: addressSpec,
			}

			Eventually(findAddress("test", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))
		})
	})

	Context("When a GlobalInClusterIPPool has two claims with the same name in two different namespaces", func() {
		const poolName = "test-pool"

		BeforeEach(func() {
			pool := v1alpha2.GlobalInClusterIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: poolName,
				},
				Spec: v1alpha2.InClusterIPPoolSpec{
					Addresses: []string{
						"10.0.0.2-10.0.0.254",
					},
					Prefix:  24,
					Gateway: "10.0.0.1",
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaim("test", namespace)
			deleteClaim("test", namespace2)
			deleteClusterScopedPool(poolName)
		})

		It("should successfully create different ip addresses for both claims", func() {
			claim := newClaim("test", namespace, "GlobalInClusterIPPool", poolName)
			claim2 := newClaim("test", namespace2, "GlobalInClusterIPPool", poolName)

			expectedIPAddress := ipamv1.IPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  namespace,
					Finalizers: []string{ipamutil.ProtectAddressFinalizer},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(true),
							Kind:               "IPAddressClaim",
							Name:               "test",
						},
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(false),
							Kind:               "GlobalInClusterIPPool",
							Name:               poolName,
						},
					},
				},
				Spec: ipamv1.IPAddressSpec{
					ClaimRef: corev1.LocalObjectReference{
						Name: "test",
					},
					PoolRef: corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "GlobalInClusterIPPool",
						Name:     poolName,
					},
					Address: "10.0.0.2",
					Prefix:  24,
					Gateway: "10.0.0.1",
				},
			}

			expectedIPAddress2 := ipamv1.IPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  namespace2,
					Finalizers: []string{ipamutil.ProtectAddressFinalizer},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(true),
							Kind:               "IPAddressClaim",
							Name:               "test",
						},
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(false),
							Kind:               "GlobalInClusterIPPool",
							Name:               poolName,
						},
					},
				},
				Spec: ipamv1.IPAddressSpec{
					ClaimRef: corev1.LocalObjectReference{
						Name: "test",
					},
					PoolRef: corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "GlobalInClusterIPPool",
						Name:     poolName,
					},
					Address: "10.0.0.3",
					Prefix:  24,
					Gateway: "10.0.0.1",
				},
			}

			Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
			Eventually(findAddress("test", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
			)

			Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())
			Eventually(findAddress("test", namespace2)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				EqualObject(&expectedIPAddress2, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress),
			)
		})
	})

	Context("When the cluster is spec.paused true and the ipaddressclaim has the cluster-name label", func() {
		const (
			clusterName = "test-cluster"
			poolName    = "test-pool"
		)

		var cluster clusterv1.Cluster

		BeforeEach(func() {
			pool := v1alpha2.InClusterIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterIPPoolSpec{
					Addresses: []string{"10.0.0.1-10.0.0.254"},
					Prefix:    24,
					Gateway:   "10.0.0.2",
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		Context("When the cluster can be retrieved", func() {
			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteCluster(clusterName, namespace)
				deleteNamespacedPool(poolName, namespace)
			})

			It("does not allocate an ipaddress upon creating a cluster when the cluster has paused annotation", func() {
				claim := ipamv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: clusterName,
						},
					},
					Spec: ipamv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Succeed())

				cluster = clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
						Annotations: map[string]string{
							clusterv1.PausedAnnotation: "",
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
				Eventually(Get(&cluster)).Should(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})

			It("does not allocate an ipaddress upon creating a cluster when the cluster has spec.Paused", func() {
				claim := ipamv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: clusterName,
						},
					},
					Spec: ipamv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Succeed())

				cluster = clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
					Spec: clusterv1.ClusterSpec{
						Paused: true,
					},
				}

				Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
				Eventually(Get(&cluster)).Should(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})

			It("does not allocate an ipaddress upon updating a cluster when the cluster has spec.paused", func() {
				cluster = clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
					Spec: clusterv1.ClusterSpec{
						Paused: true,
					},
				}

				Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
				Eventually(Get(&cluster)).Should(Succeed())

				claim := ipamv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: clusterName,
						},
					},
					Spec: ipamv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Succeed())

				// update the cluster
				cluster.Annotations = map[string]string{"superficial": "change"}
				Expect(k8sClient.Update(context.Background(), &cluster)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})

			It("does not allocate an ipaddress upon updating a cluster when the cluster has paused annotation", func() {
				cluster = clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
						Annotations: map[string]string{
							clusterv1.PausedAnnotation: "",
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
				Eventually(Get(&cluster)).Should(Succeed())

				claim := ipamv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: clusterName,
						},
					},
					Spec: ipamv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Succeed())

				// update the cluster
				cluster.Annotations["superficial"] = "change"
				Expect(k8sClient.Update(context.Background(), &cluster)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})

			It("allocates an ipaddress upon updating a cluster when removing spec.paused", func() {
				cluster = clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
					},
					Spec: clusterv1.ClusterSpec{
						Paused: true,
					},
				}

				Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
				Eventually(Get(&cluster)).Should(Succeed())

				claim := newClaim("test", namespace, "InClusterIPPool", poolName)
				claim.ObjectMeta.Labels = map[string]string{
					clusterv1.ClusterNameLabel: cluster.GetName(),
				}
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))

				// update the cluster
				cluster.Spec.Paused = false
				Expect(k8sClient.Update(context.Background(), &cluster)).To(Succeed())

				Eventually(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(1)))
			})

			It("allocates an ipaddress upon updating a cluster when removing the paused annotation", func() {
				cluster = clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: namespace,
						Annotations: map[string]string{
							clusterv1.PausedAnnotation: "",
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &cluster)).To(Succeed())
				Eventually(Get(&cluster)).Should(Succeed())

				claim := ipamv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: clusterName,
						},
					},
					Spec: ipamv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))

				// update the cluster
				delete(cluster.Annotations, clusterv1.PausedAnnotation)
				Expect(k8sClient.Update(context.Background(), &cluster)).To(Succeed())

				Eventually(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(1)))
			})
		})

		Context("When the cluster cannot be retrieved", func() {
			AfterEach(func() {
				deleteClaim("test", namespace)
				deleteNamespacedPool(poolName, namespace)
			})
			It("does not allocate an ipaddress for the claim", func() {
				claim := newClaim("test", namespace, "InClusterIPPool", poolName)
				claim.ObjectMeta.Labels = map[string]string{
					clusterv1.ClusterNameLabel: "an-unfindable-cluster",
				}
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})
		})
	})

	Context("When the ipaddressclaim is paused", func() {
		const (
			poolName = "test-pool"
		)

		BeforeEach(func() {
			pool := v1alpha2.InClusterIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterIPPoolSpec{
					Addresses: []string{"10.0.0.1-10.0.0.254"},
					Prefix:    24,
					Gateway:   "10.0.0.2",
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaim("test", namespace)
			deleteNamespacedPool(poolName, namespace)
		})

		It("does not allocate an ipaddress for the claim until the ip address claim is unpaused", func() {
			claim := newClaim("test", namespace, "InClusterIPPool", poolName)
			claim.ObjectMeta.Annotations = map[string]string{
				clusterv1.PausedAnnotation: "",
			}
			Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
			Eventually(Get(&claim)).Should(Succeed())

			addresses := ipamv1.IPAddressList{}
			Consistently(ObjectList(&addresses)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Items", HaveLen(0)))

			// Unpause the IPAddressClaim
			patchHelper, err := patch.NewHelper(&claim, k8sClient)
			Expect(err).NotTo(HaveOccurred())
			delete(claim.Annotations, clusterv1.PausedAnnotation)
			Expect(patchHelper.Patch(context.Background(), &claim)).To(Succeed())

			expectedIPAddress := ipamv1.IPAddress{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  namespace,
					Finalizers: []string{ipamutil.ProtectAddressFinalizer},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1beta1",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(true),
							Kind:               "IPAddressClaim",
							Name:               "test",
						},
						{
							APIVersion:         "ipam.cluster.x-k8s.io/v1alpha2",
							BlockOwnerDeletion: ptr.To(true),
							Controller:         ptr.To(false),
							Kind:               "InClusterIPPool",
							Name:               poolName,
						},
					},
				},
				Spec: ipamv1.IPAddressSpec{
					ClaimRef: corev1.LocalObjectReference{
						Name: "test",
					},
					PoolRef: corev1.TypedLocalObjectReference{
						APIGroup: ptr.To("ipam.cluster.x-k8s.io"),
						Kind:     "InClusterIPPool",
						Name:     poolName,
					},
					Address: "10.0.0.1",
					Prefix:  24,
					Gateway: "10.0.0.2",
				},
			}

			Eventually(findAddress("test", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnoreUIDsOnIPAddress))
		})
	})
})

func createNamespace() string {
	namespaceObj := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-ns-",
		},
	}
	ExpectWithOffset(1, k8sClient.Create(context.Background(), &namespaceObj)).To(Succeed())
	return namespaceObj.Name
}

func deleteCluster(name, namespace string) {
	cluster := clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	ExpectWithOffset(1, k8sClient.Delete(context.Background(), &cluster)).To(Succeed())
	EventuallyWithOffset(1, Get(&cluster)).Should(Not(Succeed()))
}

func deleteClusterScopedPool(name string) {
	pool := v1alpha2.GlobalInClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	ExpectWithOffset(1, k8sClient.Delete(context.Background(), &pool)).To(Succeed())
	EventuallyWithOffset(1, Get(&pool)).Should(Not(Succeed()))
}

func deleteNamespacedPool(name, namespace string) {
	pool := v1alpha2.InClusterIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	ExpectWithOffset(1, k8sClient.Delete(context.Background(), &pool)).To(Succeed())
	EventuallyWithOffset(1, Get(&pool)).Should(Not(Succeed()))
}

func deleteClaim(name, namespace string) {
	claim := ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	ExpectWithOffset(1, k8sClient.Delete(context.Background(), &claim)).To(Succeed())
	EventuallyWithOffset(1, Get(&claim)).Should(Not(Succeed()))
}

func deleteClaimUnchecked(name, namespace string) {
	claim := ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	k8sClient.Delete(context.Background(), &claim)
}

func findAddress(name, namespace string) func() (client.Object, error) {
	address := ipamv1.IPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ipamv1.IPAddressSpec{},
	}
	return Object(&address)
}
