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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	pooltypes "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

var _ = Describe("Prefix Pool Reconciler", func() {
	var namespace string

	BeforeEach(func() {
		namespace = createNamespace()
	})

	DescribeTable("reports /64 status counts", func(poolKind string) {
		pool := newPrefixPool(poolKind, "prefix-status-", namespace, v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd00::/62"},
		})
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())
		defer func() {
			deleteClaimUnchecked("prefix-status-claim", namespace)
			_ = k8sClient.Delete(context.Background(), pool)
		}()

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Total", Equal(4)))
		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Free", Equal(4)))

		claim := newClaim("prefix-status-claim", namespace, poolKind, pool.GetName())
		Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Used", Equal(1)))
		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Free", Equal(3)))
	},
		Entry(inClusterPrefixPoolKind, inClusterPrefixPoolKind),
		Entry(globalInClusterPrefixPoolKind, globalInClusterPrefixPoolKind),
	)

	It("reports status counts for custom allocationPrefixLength", func() {
		pool := &v1alpha2.InClusterPrefixPool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "prefix-status-custom-", Namespace: namespace},
			Spec: v1alpha2.InClusterPrefixPoolSpec{
				Prefixes:               []string{"fd40::/48"},
				AllocationPrefixLength: 56,
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())
		defer func() {
			deleteClaimUnchecked("prefix-status-custom-claim", namespace)
			_ = k8sClient.Delete(context.Background(), pool)
		}()

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Total", Equal(256)))
		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Free", Equal(256)))

		claim := newClaim("prefix-status-custom-claim", namespace, inClusterPrefixPoolKind, pool.GetName())
		Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Used", Equal(1)))
		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Free", Equal(255)))
	})

	It("reports out-of-range allocations after pool shrink", func() {
		pool := &v1alpha2.InClusterPrefixPool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "prefix-range-", Namespace: namespace},
			Spec: v1alpha2.InClusterPrefixPoolSpec{
				Prefixes: []string{"fd01::/62"},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())
		defer func() {
			deleteClaimUnchecked("prefix-range-claim", namespace)
			_ = k8sClient.Delete(context.Background(), pool)
		}()

		claim := newClaim("prefix-range-claim", namespace, inClusterPrefixPoolKind, pool.GetName())
		Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Used", Equal(1)))

		pool.Spec.Prefixes = []string{"fd01:0:0:1::/64"}
		Expect(k8sClient.Update(context.Background(), pool)).To(Succeed())

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.OutOfRange", Equal(1)))
	})

	It("reports out-of-range when excludedPrefixes invalidate existing allocations", func() {
		pool := &v1alpha2.InClusterPrefixPool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "prefix-policy-", Namespace: namespace},
			Spec: v1alpha2.InClusterPrefixPoolSpec{
				Prefixes: []string{"fd03::/62"},
			},
		}
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())
		defer func() {
			deleteClaimUnchecked("prefix-policy-claim", namespace)
			_ = k8sClient.Delete(context.Background(), pool)
		}()

		claim := newClaim("prefix-policy-claim", namespace, inClusterPrefixPoolKind, pool.GetName())
		Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Used", Equal(1)))

		pool.Spec.ExcludedPrefixes = []string{"fd03::/64"}
		pool.Spec.Gateway = "fd03:0:0:1::1"
		Expect(k8sClient.Update(context.Background(), pool)).To(Succeed())

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.OutOfRange", Equal(1)))
	})

	DescribeTable("protects pool deletion while allocations exist", func(poolKind string) {
		pool := newPrefixPool(poolKind, "prefix-finalizer-", namespace, v1alpha2.InClusterPrefixPoolSpec{
			Prefixes: []string{"fd02::/63"},
		})
		Expect(k8sClient.Create(context.Background(), pool)).To(Succeed())

		claimName := "prefix-finalizer-" + strings.ToLower(poolKind)
		claim := newClaim(claimName, namespace, poolKind, pool.GetName())
		Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("ObjectMeta.Finalizers", ContainElement(ProtectPoolFinalizer)))
		Eventually(Object(pool)).
			WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("Status.Addresses.Used", Equal(1)))

		Expect(k8sClient.Delete(context.Background(), pool)).To(Succeed())
		Consistently(Object(pool)).
			WithTimeout(1 * time.Second).WithPolling(100 * time.Millisecond).Should(
			HaveField("ObjectMeta.Finalizers", ContainElement(ProtectPoolFinalizer)))

		deleteClaim(claimName, namespace)
		Eventually(Get(pool)).Should(Not(Succeed()))
	},
		Entry(inClusterPrefixPoolKind, inClusterPrefixPoolKind),
		Entry(globalInClusterPrefixPoolKind, globalInClusterPrefixPoolKind),
	)
})

func newPrefixPool(poolType, generateName, namespace string, spec v1alpha2.InClusterPrefixPoolSpec) pooltypes.GenericInClusterPrefixPool {
	switch poolType {
	case inClusterPrefixPoolKind:
		return &v1alpha2.InClusterPrefixPool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: generateName, Namespace: namespace},
			Spec:       spec,
		}
	case globalInClusterPrefixPoolKind:
		return &v1alpha2.GlobalInClusterPrefixPool{
			ObjectMeta: metav1.ObjectMeta{GenerateName: generateName},
			Spec:       spec,
		}
	default:
		Fail("Unknown pool type")
	}
	return nil
}
