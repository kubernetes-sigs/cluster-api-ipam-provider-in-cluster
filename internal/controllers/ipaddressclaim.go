package controllers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/internal/index"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	ipampredicates "github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/predicates"
	pooltypes "github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/types"
)

type genericInClusterPool interface {
	client.Object
	PoolSpec() *v1alpha1.InClusterIPPoolSpec
}

type InClusterProviderIntegration struct {
	Client           client.Client
	WatchFilterValue string
}

var _ ipamutil.ProviderIntegration = &InClusterProviderIntegration{}

// IPAddressClaimHandler reconciles a InClusterIPPool object.
type IPAddressClaimHandler struct {
	client.Client
	claim *ipamv1.IPAddressClaim
	pool  genericInClusterPool
}

var _ ipamutil.ClaimHandler = &IPAddressClaimHandler{}

// SetupWithManager sets up the controller with the Manager.
func (i *InClusterProviderIntegration) SetupWithManager(ctx context.Context, b *ctrl.Builder) error {
	b.
		For(&ipamv1.IPAddressClaim{}, builder.WithPredicates(
			predicate.Or(
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha1.GroupVersion.Group,
					Kind:  inClusterIPPoolKind,
				}),
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha1.GroupVersion.Group,
					Kind:  globalInClusterIPPoolKind,
				}),
			),
		)).
		// A Watch is added for the Cluster in the case that the Cluster is
		// unpaused so that a request can be queued to re-reconcile the
		// IPAddressClaim.
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			handler.EnqueueRequestsFromMapFunc(r.clusterToIPClaims),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldCluster := e.ObjectOld.(*clusterv1.Cluster)
					newCluster := e.ObjectNew.(*clusterv1.Cluster)
					return oldCluster.Spec.Paused && !newCluster.Spec.Paused
				},
				CreateFunc: func(e event.CreateEvent) bool {
					return !annotations.HasPaused(e.Object)
				},
			}),
		).
		WithOptions(controller.Options{
			// To avoid race conditions when allocating IP Addresses, we explicitly set this to 1
			MaxConcurrentReconciles: 1,
		}).
		Watches(
			&source.Kind{Type: &v1alpha1.InClusterIPPool{}},
			handler.EnqueueRequestsFromMapFunc(i.inClusterIPPoolToIPClaims("InClusterIPPool")),
			builder.WithPredicates(resourceTransitionedToUnpaused()),
		).
		Watches(
			&source.Kind{Type: &v1alpha1.GlobalInClusterIPPool{}},
			handler.EnqueueRequestsFromMapFunc(i.inClusterIPPoolToIPClaims("GlobalInClusterIPPool")),
			builder.WithPredicates(resourceTransitionedToUnpaused()),
		).
		Owns(&ipamv1.IPAddress{}, builder.WithPredicates(
			ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
				Group: v1alpha1.GroupVersion.Group,
				Kind:  inClusterIPPoolKind,
			}),
		))
	return nil
}

func (i *InClusterProviderIntegration) inClusterIPPoolToIPClaims(kind string) func(client.Object) []reconcile.Request {
	return func(a client.Object) []reconcile.Request {
		pool := a.(pooltypes.GenericInClusterPool)
		requests := []reconcile.Request{}
		claims := &ipamv1.IPAddressClaimList{}
		err := i.Client.List(context.Background(), claims,
			client.MatchingFields{
				"index.poolRef": index.IPPoolRefValue(corev1.TypedLocalObjectReference{
					Name:     pool.GetName(),
					Kind:     kind,
					APIGroup: &v1alpha1.GroupVersion.Group,
				}),
			},
			client.InNamespace(pool.GetNamespace()),
		)
		if err != nil {
			return requests
		}
		for _, claim := range claims.Items {
			r := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			}
			requests = append(requests, r)
		}
		return requests
	}
}

func (i *InClusterProviderIntegration) clusterToIPClaims(a client.Object) []reconcile.Request {
	requests := []reconcile.Request{}
	vms := &ipamv1.IPAddressClaimList{}
	err := i.Client.List(context.Background(), vms, client.MatchingLabels(
		map[string]string{
			clusterv1.ClusterNameLabel: a.GetName(),
		},
	))
	if err != nil {
		return requests
	}
	for _, vm := range vms.Items {
		r := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      vm.Name,
				Namespace: vm.Namespace,
			},
		}
		requests = append(requests, r)
	}
	return requests
}

// ClaimHandlerFor returns a claim handler for a specific claim.
func (i *InClusterProviderIntegration) ClaimHandlerFor(_ client.Client, claim *ipamv1.IPAddressClaim) ipamutil.ClaimHandler {
	return &IPAddressClaimHandler{
		Client: i.Client,
		claim:  claim,
	}
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/finalizers,verbs=update
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools/finalizers,verbs=update
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch

// FetchPool fetches the (Global)InClusterIPPool.
func (h *IPAddressClaimHandler) FetchPool(ctx context.Context) (*ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	if h.claim.Spec.PoolRef.Kind == inClusterIPPoolKind {
		icippool := &v1alpha1.InClusterIPPool{}
		if err := h.Client.Get(ctx, types.NamespacedName{Namespace: h.claim.Namespace, Name: h.claim.Spec.PoolRef.Name}, icippool); err != nil && !apierrors.IsNotFound(err) {
			return nil, errors.Wrap(err, "failed to fetch pool")
		}
		h.pool = icippool
	} else if h.claim.Spec.PoolRef.Kind == globalInClusterIPPoolKind {
		gicippool := &v1alpha1.GlobalInClusterIPPool{}
		if err := h.Client.Get(ctx, types.NamespacedName{Name: h.claim.Spec.PoolRef.Name}, gicippool); err != nil && !apierrors.IsNotFound(err) {
			return nil, errors.Wrap(err, "failed to fetch pool")
		}
		h.pool = gicippool
	}

	if h.pool == nil {
		err := errors.New("pool not found")
		log.Error(err, "the referenced pool could not be found")
		return nil, nil
	}

	return nil, nil
}

// EnsureAddress ensures that the IPAddress contains a valid address.
func (h *IPAddressClaimHandler) EnsureAddress(ctx context.Context, address *ipamv1.IPAddress) (*ctrl.Result, error) {
	addressesInUse, err := poolutil.ListAddressesInUse(ctx, h.Client, h.pool.GetNamespace(), h.claim.Spec.PoolRef)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses: %w", err)
	}

	allocated := slices.ContainsFunc(addressesInUse, func(a ipamv1.IPAddress) bool {
		return a.Name == address.Name
	})

	if !allocated {
		poolSpec := h.pool.PoolSpec()
		inUseIPSet, err := poolutil.AddressesToIPSet(buildAddressList(addressesInUse, poolSpec.Gateway))
		if err != nil {
			return nil, fmt.Errorf("failed to convert IPAddressList to IPSet: %w", err)
		}

		poolIPSet, err := poolutil.IPPoolSpecToIPSet(poolSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to convert pool to range: %w", err)
		}

		freeIP, err := poolutil.FindFreeAddress(poolIPSet, inUseIPSet)
		if err != nil {
			return nil, fmt.Errorf("failed to find free address: %w", err)
		}

		address.Spec.Address = freeIP.String()
		address.Spec.Gateway = poolSpec.Gateway
		address.Spec.Prefix = poolSpec.Prefix
	}

	return nil, nil
}

// ReleaseAddress releases the ip address.
func (h *IPAddressClaimHandler) ReleaseAddress() (*ctrl.Result, error) {
	// We don't need to do anything here, since the ip address is released when the IPAddress is deleted
	return nil, nil
}

// GetPool returns the pool referenced by the claim that is processed.
// Will panic if called before FetchPool().
func (h *IPAddressClaimHandler) GetPool() client.Object {
	if h.pool == nil {
		panic("cannot return pool before fetching it")
	}
	return h.pool
}

func buildAddressList(addressesInUse []ipamv1.IPAddress, gateway string) []string {
	// Add extra capacity for the case that the pool's gateway is specified
	addrStrings := make([]string, len(addressesInUse), len(addressesInUse)+1)
	for i, address := range addressesInUse {
		addrStrings[i] = address.Spec.Address
	}

	if gateway != "" {
		addrStrings = append(addrStrings, gateway)
	}

	return addrStrings
}

func resourceTransitionedToUnpaused() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return annotations.HasPaused(e.ObjectOld) && !annotations.HasPaused(e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return !annotations.HasPaused(e.Object)
		},
	}
}
