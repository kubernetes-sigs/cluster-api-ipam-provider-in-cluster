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

package controllers

import (
	"context"
	"net/netip"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
)

var _ = Describe("IPAddressClaimReconciler prefix allocation", func() {
	var (
		namespace  string
		namespace2 string
	)

	BeforeEach(func() {
		namespace = createNamespace()
		namespace2 = createNamespace()
	})

	When("a namespaced InClusterPrefixPool exists", func() {
		const poolName = "test-prefix-pool"

		BeforeEach(func() {
			pool := v1alpha2.InClusterPrefixPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterPrefixPoolSpec{
					Prefixes: []string{"fd00::/62"},
					Gateway:  "fd10::1",
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaimUnchecked("prefix-claim-1", namespace)
			deleteClaimUnchecked("prefix-claim-2", namespace)
			pool := v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}
			_ = k8sClient.Delete(context.Background(), &pool)
		})

		It("allocates deterministic non-overlapping /64 prefixes", func() {
			claim1 := newClaim("prefix-claim-1", namespace, inClusterPrefixPoolKind, poolName)
			claim2 := newClaim("prefix-claim-2", namespace, inClusterPrefixPoolKind, poolName)

			Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

			Eventually(findAddress("prefix-claim-1", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd00::"))
			Eventually(findAddress("prefix-claim-1", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(64)))))
			Eventually(findAddress("prefix-claim-1", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Gateway", "fd10::1"))

			Eventually(findAddress("prefix-claim-2", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd00:0:0:1::"))
			Eventually(findAddress("prefix-claim-2", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(64)))))

			prefix1 := netip.MustParsePrefix("fd00::/64")
			prefix2 := netip.MustParsePrefix("fd00:0:0:1::/64")
			Expect(prefix1.Overlaps(prefix2)).To(BeFalse())
		})

		When("a paused InClusterPrefixPool exists", func() {
			const poolName = "paused-prefix-pool"

			BeforeEach(func() {
				pool := v1alpha2.InClusterPrefixPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
						Annotations: map[string]string{
							clusterv1.PausedAnnotation: "",
						},
					},
					Spec: v1alpha2.InClusterPrefixPoolSpec{
						Prefixes: []string{"fd04::/63"},
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				deleteClaimUnchecked("paused-prefix-claim", namespace)
				pool := v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}
				_ = k8sClient.Delete(context.Background(), &pool)
			})

			It("does not allocate until the pool is unpaused", func() {
				claim := newClaim("paused-prefix-claim", namespace, inClusterPrefixPoolKind, poolName)
				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				addresses := ipamv1.IPAddressList{}
				Consistently(ObjectList(&addresses, client.InNamespace(namespace))).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))

				pool := &v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}
				Eventually(Get(pool)).Should(Succeed())
				delete(pool.Annotations, clusterv1.PausedAnnotation)
				Expect(k8sClient.Update(context.Background(), pool)).To(Succeed())

				Eventually(findAddress("paused-prefix-claim", namespace)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Spec.Address", "fd04::"))
			})
		})
	})

	When("a namespaced InClusterPrefixPool with custom allocation prefix length exists", func() {
		const poolName = "test-prefix-pool-custom-allocation"

		BeforeEach(func() {
			pool := v1alpha2.InClusterPrefixPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterPrefixPoolSpec{
					Prefixes:               []string{"fd06::/48"},
					AllocationPrefixLength: 56,
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaimUnchecked("custom-prefix-claim-1", namespace)
			deleteClaimUnchecked("custom-prefix-claim-2", namespace)
			pool := v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}
			_ = k8sClient.Delete(context.Background(), &pool)
		})

		It("allocates deterministic non-overlapping /56 prefixes", func() {
			claim1 := newClaim("custom-prefix-claim-1", namespace, inClusterPrefixPoolKind, poolName)
			claim2 := newClaim("custom-prefix-claim-2", namespace, inClusterPrefixPoolKind, poolName)

			Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

			Eventually(findAddress("custom-prefix-claim-1", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd06::"))
			Eventually(findAddress("custom-prefix-claim-1", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(56)))))

			Eventually(findAddress("custom-prefix-claim-2", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd06:0:0:100::"))
			Eventually(findAddress("custom-prefix-claim-2", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(56)))))

			prefix1 := netip.MustParsePrefix("fd06::/56")
			prefix2 := netip.MustParsePrefix("fd06:0:0:100::/56")
			Expect(prefix1.Overlaps(prefix2)).To(BeFalse())
		})
	})

	When("a namespaced InClusterPrefixPool with mixed source prefix sizes exists", func() {
		const poolName = "test-prefix-pool-mixed-sources"

		BeforeEach(func() {
			pool := v1alpha2.InClusterPrefixPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterPrefixPoolSpec{
					Prefixes:               []string{"fd00:0:0:1::/64", "fd10::/48"},
					AllocationPrefixLength: 64,
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaimUnchecked("mixed-prefix-claim-1", namespace)
			deleteClaimUnchecked("mixed-prefix-claim-2", namespace)
			pool := v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}
			_ = k8sClient.Delete(context.Background(), &pool)
		})

		It("allocates deterministically from standalone and aggregate sources", func() {
			claim1 := newClaim("mixed-prefix-claim-1", namespace, inClusterPrefixPoolKind, poolName)
			Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())

			Eventually(findAddress("mixed-prefix-claim-1", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd00:0:0:1::"))
			Eventually(findAddress("mixed-prefix-claim-1", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(64)))))

			claim2 := newClaim("mixed-prefix-claim-2", namespace, inClusterPrefixPoolKind, poolName)
			Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

			Eventually(findAddress("mixed-prefix-claim-2", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd10::"))
			Eventually(findAddress("mixed-prefix-claim-2", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(64)))))
		})
	})

	When("a GlobalInClusterPrefixPool exists", func() {
		const poolName = "test-global-prefix-pool"

		BeforeEach(func() {
			pool := v1alpha2.GlobalInClusterPrefixPool{
				ObjectMeta: metav1.ObjectMeta{Name: poolName},
				Spec: v1alpha2.InClusterPrefixPoolSpec{
					Prefixes: []string{"fd01::/63"},
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaimUnchecked("global-prefix-claim-1", namespace)
			deleteClaimUnchecked("global-prefix-claim-2", namespace2)
			pool := v1alpha2.GlobalInClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName}}
			_ = k8sClient.Delete(context.Background(), &pool)
		})

		It("allocates prefixes across namespaces", func() {
			claim1 := newClaim("global-prefix-claim-1", namespace, globalInClusterPrefixPoolKind, poolName)
			claim2 := newClaim("global-prefix-claim-2", namespace2, globalInClusterPrefixPoolKind, poolName)

			Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

			Eventually(findAddress("global-prefix-claim-1", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd01::"))
			Eventually(findAddress("global-prefix-claim-1", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(64)))))

			Eventually(findAddress("global-prefix-claim-2", namespace2)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd01:0:0:1::"))
			Eventually(findAddress("global-prefix-claim-2", namespace2)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Prefix", Equal(ptr.To(int32(64)))))
		})
	})

	When("a prefix claim is deleted", func() {
		const poolName = "prefix-deletion-pool"

		BeforeEach(func() {
			pool := v1alpha2.InClusterPrefixPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterPrefixPoolSpec{
					Prefixes: []string{"fd05::/63"},
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaimUnchecked("prefix-del-claim", namespace)
			pool := v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}
			_ = k8sClient.Delete(context.Background(), &pool)
		})

		It("releases the allocated prefix when the claim is deleted", func() {
			claim := newClaim("prefix-del-claim", namespace, inClusterPrefixPoolKind, poolName)
			Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

			Eventually(findAddress("prefix-del-claim", namespace)).
				WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd05::"))

			deleteClaim("prefix-del-claim", namespace)

			Eventually(Get(&ipamv1.IPAddress{ObjectMeta: metav1.ObjectMeta{
				Name:      "prefix-del-claim",
				Namespace: namespace,
			}})).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(Not(Succeed()))
		})
	})

	When("the pool is exhausted", func() {
		const poolName = "prefix-exhausted-pool"

		BeforeEach(func() {
			pool := v1alpha2.InClusterPrefixPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      poolName,
					Namespace: namespace,
				},
				Spec: v1alpha2.InClusterPrefixPoolSpec{
					Prefixes: []string{"fd07::/64"},
				},
			}
			Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
			Eventually(Get(&pool)).Should(Succeed())
		})

		AfterEach(func() {
			deleteClaimUnchecked("exhausted-claim-1", namespace)
			deleteClaimUnchecked("exhausted-claim-2", namespace)
			pool := v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}
			_ = k8sClient.Delete(context.Background(), &pool)
		})

		It("does not allocate a second prefix when the single /64 is exhausted", func() {
			pool := &v1alpha2.InClusterPrefixPool{ObjectMeta: metav1.ObjectMeta{Name: poolName, Namespace: namespace}}

			claim1 := newClaim("exhausted-claim-1", namespace, inClusterPrefixPoolKind, poolName)
			claim2 := newClaim("exhausted-claim-2", namespace, inClusterPrefixPoolKind, poolName)
			Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())

			Eventually(findAddress("exhausted-claim-1", namespace)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Spec.Address", "fd07::"))

			addresses := ipamv1.IPAddressList{}
			Consistently(ObjectList(&addresses, client.InNamespace(namespace))).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Items", HaveLen(1)))

			Eventually(Object(pool)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Status.Addresses.Free", Equal(0)))
			Eventually(Object(pool)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Status.Addresses.Used", Equal(1)))
			Eventually(Object(pool)).
				WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
				HaveField("Status.Addresses.Total", Equal(1)))
		})
	})
})
