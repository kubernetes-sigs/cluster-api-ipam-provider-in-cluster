package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
)

var _ = Describe("IPAddressClaimReconciler", func() {
	var namespace string
	var namespaceObj corev1.Namespace
	BeforeEach(func() {
		namespaceObj = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-ns-",
			},
		}
		Expect(k8sClient.Create(context.Background(), &namespaceObj)).To(Succeed())
		namespace = namespaceObj.Name
	})

	Context("When a new IPAddressClaim is created", func() {
		When("the referenced pool is an unrecognized kind", func() {
			const poolName = "unknown-pool"

			BeforeEach(func() {
				pool := v1alpha1.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha1.InClusterIPPoolSpec{
						First:   "10.0.1.1",
						Last:    "10.0.1.254",
						Prefix:  24,
						Gateway: "10.0.1.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				pool := v1alpha1.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Delete(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Not(Succeed()))

				claim := clusterv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unknown-pool-test",
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Delete(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Not(Succeed()))
			})

			It("should ignore the claim", func() {
				claim := clusterv1.IPAddressClaim{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "ipam.cluster.x-k8s.io/v1alpha1",
						Kind:       "IPAddressClaim",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unknown-pool-test",
						Namespace: namespace,
					},
					Spec: clusterv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "UnknownIPPool",
							Name:     poolName,
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				// Eventually(Object(&claim)).Should(HaveField("Status.Address.Name", Equal(claim.ObjectMeta.Name)))

				addresses := clusterv1.IPAddressList{}
				Consistently(ObjectList(&addresses)).
					WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(
					HaveField("Items", HaveLen(0)))
			})
		})

		When("the referenced namespaced pool exists", func() {
			const poolName = "test-pool"

			BeforeEach(func() {
				pool := v1alpha1.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha1.InClusterIPPoolSpec{
						First:   "10.0.0.1",
						Last:    "10.0.0.254",
						Prefix:  24,
						Gateway: "10.0.0.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				pool := v1alpha1.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Delete(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Not(Succeed()))

				claim := clusterv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Delete(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Not(Succeed()))
			})

			It("should allocate an Address from the Pool", func() {
				claim := clusterv1.IPAddressClaim{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "ipam.cluster.x-k8s.io/v1alpha1",
						Kind:       "IPAddressClaim",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
					},
					Spec: clusterv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}
				address := clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
					},
					Spec: clusterv1.IPAddressSpec{},
				}

				expectedIPAddress := clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  namespace,
						Finalizers: []string{ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(true),
								Kind:               "IPAddressClaim",
								Name:               "test",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(false),
								Kind:               "InClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: clusterv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.1",
						Prefix:  24,
						Gateway: "10.0.0.2",
					},
				}

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())
				// Eventually(Object(&claim)).Should(HaveField("Status.Address.Name", Equal(claim.ObjectMeta.Name)))

				Eventually(Object(&address)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnorePaths{
						"TypeMeta",
						"ObjectMeta.OwnerReferences[0].UID",
						"ObjectMeta.OwnerReferences[1].UID",
						"Spec.Claim.UID",
						"Spec.Pool.UID",
					}),
				)
			})
		})

		When("the referenced global pool exists", func() {
			const poolName = "global-pool"

			BeforeEach(func() {
				pool := v1alpha1.GlobalInClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{ // global pool, no namespace
						Name: poolName,
					},
					Spec: v1alpha1.InClusterIPPoolSpec{
						First:   "10.0.0.1",
						Last:    "10.0.0.254",
						Prefix:  24,
						Gateway: "10.0.0.2",
					},
				}
				Expect(k8sClient.Create(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Succeed())
			})

			AfterEach(func() {
				pool := v1alpha1.GlobalInClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name: poolName,
					},
				}
				Expect(k8sClient.Delete(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Not(Succeed()))

				claim := clusterv1.IPAddressClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Delete(context.Background(), &claim)).To(Succeed())
				Eventually(Get(&claim)).Should(Not(Succeed()))
			})

			It("should allocate an Address from the Pool", func() {
				claim := clusterv1.IPAddressClaim{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "ipam.cluster.x-k8s.io/v1alpha1",
						Kind:       "IPAddressClaim",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
					},
					Spec: clusterv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "GlobalInClusterIPPool",
							Name:     poolName,
						},
					},
				}
				address := clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: namespace,
					},
					Spec: clusterv1.IPAddressSpec{},
				}

				expectedIPAddress := clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test",
						Namespace:  namespace,
						Finalizers: []string{ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(true),
								Kind:               "IPAddressClaim",
								Name:               "test",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(false),
								Kind:               "GlobalInClusterIPPool",
								Name:               poolName,
							},
						},
					},
					Spec: clusterv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "GlobalInClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.1",
						Prefix:  24,
						Gateway: "10.0.0.2",
					},
				}

				Expect(k8sClient.Create(context.Background(), &claim)).To(Succeed())

				Eventually(Object(&address)).
					WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedIPAddress, IgnoreAutogeneratedMetadata, IgnorePaths{
						"TypeMeta",
						"ObjectMeta.OwnerReferences[0].UID",
						"ObjectMeta.OwnerReferences[1].UID",
						"Spec.Claim.UID",
						"Spec.Pool.UID",
					}),
				)
			})
		})

		When("the referenced pool uses single ip addresses", func() {
			const poolName = "test-pool-single-ip-addresses"
			BeforeEach(func() {
				pool := v1alpha1.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
					Spec: v1alpha1.InClusterIPPoolSpec{
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
				pool := v1alpha1.InClusterIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      poolName,
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Delete(context.Background(), &pool)).To(Succeed())
				Eventually(Get(&pool)).Should(Not(Succeed()))
			})

			It("should allocate an Address from the Pool", func() {
				claim1 := clusterv1.IPAddressClaim{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "ipam.cluster.x-k8s.io/v1alpha1",
						Kind:       "IPAddressClaim",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-1",
						Namespace: namespace,
					},
					Spec: clusterv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}

				claim2 := clusterv1.IPAddressClaim{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "ipam.cluster.x-k8s.io/v1alpha1",
						Kind:       "IPAddressClaim",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-2",
						Namespace: namespace,
					},
					Spec: clusterv1.IPAddressClaimSpec{
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
					},
				}

				expectedAddress1 := clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-1",
						Namespace:  namespace,
						Finalizers: []string{ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(true),
								Kind:               "IPAddressClaim",
								Name:               "test-1",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(false),
								Kind:               "InClusterIPPool",
								Name:               "test-pool-single-ip-addresses",
							},
						},
					},
					Spec: clusterv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test-1",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.50",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				expectedAddress2 := clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-2",
						Namespace:  namespace,
						Finalizers: []string{ProtectAddressFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(true),
								Kind:               "IPAddressClaim",
								Name:               "test-2",
							},
							{
								APIVersion:         "ipam.cluster.x-k8s.io/v1alpha1",
								BlockOwnerDeletion: pointer.Bool(true),
								Controller:         pointer.Bool(false),
								Kind:               "InClusterIPPool",
								Name:               "test-pool-single-ip-addresses",
							},
						},
					},
					Spec: clusterv1.IPAddressSpec{
						ClaimRef: corev1.LocalObjectReference{
							Name: "test-2",
						},
						PoolRef: corev1.TypedLocalObjectReference{
							APIGroup: pointer.String("ipam.cluster.x-k8s.io"),
							Kind:     "InClusterIPPool",
							Name:     poolName,
						},
						Address: "10.0.0.128",
						Prefix:  24,
						Gateway: "10.0.0.1",
					},
				}

				Expect(k8sClient.Create(context.Background(), &claim1)).To(Succeed())
				Expect(k8sClient.Create(context.Background(), &claim2)).To(Succeed())
				// Eventually(Object(&claim)).Should(HaveField("Status.Address.Name", Equal(claim.ObjectMeta.Name)))

				Eventually(Object(&clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      expectedAddress1.GetName(),
						Namespace: namespace,
					},
				})).WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedAddress1, IgnoreAutogeneratedMetadata, IgnorePaths{
						"TypeMeta",
						"ObjectMeta.OwnerReferences[0].UID",
						"ObjectMeta.OwnerReferences[1].UID",
						"Spec.Claim.UID",
						"Spec.Pool.UID",
					}),
				)

				Eventually(Object(&clusterv1.IPAddress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      expectedAddress2.GetName(),
						Namespace: namespace,
					},
				})).WithTimeout(time.Second).WithPolling(100 * time.Millisecond).Should(
					EqualObject(&expectedAddress2, IgnoreAutogeneratedMetadata, IgnorePaths{
						"TypeMeta",
						"ObjectMeta.OwnerReferences[0].UID",
						"ObjectMeta.OwnerReferences[1].UID",
						"Spec.Claim.UID",
						"Spec.Pool.UID",
					}),
				)
			})
		})
	})
})
