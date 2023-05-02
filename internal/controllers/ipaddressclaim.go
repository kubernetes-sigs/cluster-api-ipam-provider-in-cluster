package controllers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	clusterutil "sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

const (
	// ReleaseAddressFinalizer is used to release an IP address before cleaning up the claim.
	ReleaseAddressFinalizer = "ipam.cluster.x-k8s.io/ReleaseAddress"

	// ProtectAddressFinalizer is used to prevent deletion of an IPAddress object while its claim is not deleted.
	ProtectAddressFinalizer = "ipam.cluster.x-k8s.io/ProtectAddress"
)

// IPAddressClaimReconciler reconciles a InClusterIPPool object.
type IPAddressClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	WatchFilterValue string
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAddressClaimReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ipamv1.IPAddressClaim{}, builder.WithPredicates(
			predicate.Or(
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha1.GroupVersion.Group,
					Kind:  "InClusterIPPool",
				}),
				ipampredicates.ClaimReferencesPoolKind(metav1.GroupKind{
					Group: v1alpha1.GroupVersion.Group,
					Kind:  "GlobalInClusterIPPool",
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
					if _, ok := e.Object.GetAnnotations()[clusterv1.PausedAnnotation]; !ok {
						return false
					}
					return true
				},
			}),
		).
		WithOptions(controller.Options{
			// To avoid race conditions when allocating IP Addresses, we explicitly set this to 1
			MaxConcurrentReconciles: 1,
		}).
		Watches(
			&source.Kind{Type: &v1alpha1.InClusterIPPool{}},
			handler.EnqueueRequestsFromMapFunc(r.inClusterIPPoolToIPClaims("InClusterIPPool")),
			builder.WithPredicates(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)),
		).
		Watches(
			&source.Kind{Type: &v1alpha1.GlobalInClusterIPPool{}},
			handler.EnqueueRequestsFromMapFunc(r.inClusterIPPoolToIPClaims("GlobalInClusterIPPool")),
			builder.WithPredicates(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)),
		).
		Owns(&ipamv1.IPAddress{}, builder.WithPredicates(
			ipampredicates.AddressReferencesPoolKind(metav1.GroupKind{
				Group: v1alpha1.GroupVersion.Group,
				Kind:  "InClusterIPPool",
			}),
		)).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)
}

func (r *IPAddressClaimReconciler) inClusterIPPoolToIPClaims(kind string) func(client.Object) []reconcile.Request {
	return func(a client.Object) []reconcile.Request {
		pool := a.(pooltypes.GenericInClusterPool)
		requests := []reconcile.Request{}
		claims := &ipamv1.IPAddressClaimList{}
		err := r.Client.List(context.Background(), claims,
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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IPAddressClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling claim")

	// Fetch the IPAddressClaim
	claim := &ipamv1.IPAddressClaim{}
	if err := r.Client.Get(ctx, req.NamespacedName, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	cluster, err := clusterutil.GetClusterFromMetadata(ctx, r.Client, claim.ObjectMeta)
	if err == nil {
		if annotations.IsPaused(cluster, claim) {
			log.Info("IPAddressClaim linked to a cluster that is paused")
			return reconcile.Result{}, nil
		}
	}

	patchHelper, err := patch.NewHelper(claim, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, claim); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	var pool pooltypes.GenericInClusterPool

	if claim.Spec.PoolRef.Kind == "InClusterIPPool" {
		icippool := &v1alpha1.InClusterIPPool{}
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: claim.Namespace, Name: claim.Spec.PoolRef.Name}, icippool); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to fetch pool")
		}
		pool = icippool
	} else if claim.Spec.PoolRef.Kind == "GlobalInClusterIPPool" {
		gicippool := &v1alpha1.GlobalInClusterIPPool{}
		if err := r.Client.Get(ctx, types.NamespacedName{Name: claim.Spec.PoolRef.Name}, gicippool); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to fetch pool")
		}
		pool = gicippool
	}

	if annotations.HasPaused(pool) {
		log.Info("IPAddressClaim references Pool which is paused, skipping reconciliation.", "IPAddressClaim", claim.GetName(), "Pool", pool.GetName())
		return ctrl.Result{}, nil
	}

	address := &ipamv1.IPAddress{}
	if err := r.Client.Get(ctx, req.NamespacedName, address); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrap(err, "failed to fetch address")
	}

	if !controllerutil.ContainsFinalizer(claim, ReleaseAddressFinalizer) {
		controllerutil.AddFinalizer(claim, ReleaseAddressFinalizer)
	}

	if !claim.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, claim, address)
	}
	addressesInUse, err := poolutil.ListAddressesInUse(ctx, r.Client, pool.GetNamespace(), claim.Spec.PoolRef)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list addresses: %w", err)
	}

	return r.reconcile(ctx, claim, pool, addressesInUse)
}

func (r *IPAddressClaimReconciler) reconcile(ctx context.Context, claim *ipamv1.IPAddressClaim, pool pooltypes.GenericInClusterPool, addressesInUse []ipamv1.IPAddress) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if pool == nil {
		err := errors.New("pool not found")
		log.Error(err, "the referenced pool could not be found")
		return ctrl.Result{}, nil
	}

	log = log.WithValues(pool.GetObjectKind().GroupVersionKind().Kind, fmt.Sprintf("%s/%s", pool.GetNamespace(), pool.GetName()))

	address := poolutil.AddressByNamespacedName(addressesInUse, claim.Namespace, claim.Name)
	if address == nil {
		var err error
		address, err = r.allocateAddress(claim, pool, addressesInUse)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to allocate address")
		}
	}

	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, address, func() error {
		if err := ipamutil.EnsureIPAddressOwnerReferences(r.Scheme, address, claim, pool); err != nil {
			return errors.Wrap(err, "failed to ensure owner references on address")
		}

		_ = controllerutil.AddFinalizer(address, ProtectAddressFinalizer)

		return nil
	})
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to create or patch address")
	}

	log.Info(fmt.Sprintf("IPAddress %s/%s (%s) has been %s", address.Namespace, address.Name, address.Spec.Address, operationResult),
		"IPAddressClaim", fmt.Sprintf("%s/%s", claim.Namespace, claim.Name))

	if !address.DeletionTimestamp.IsZero() {
		// We prevent deleting IPAddresses while their corresponding IPClaim still exists since we cannot guarantee that the IP
		// wil remain the same when we recreate it.
		log.Info("Address is marked for deletion, but deletion is prevented until the claim is deleted as well.", "address", address.Name)
	}

	claim.Status.AddressRef = corev1.LocalObjectReference{Name: address.Name}

	return ctrl.Result{}, nil
}

func (r *IPAddressClaimReconciler) reconcileDelete(ctx context.Context, claim *ipamv1.IPAddressClaim, address *ipamv1.IPAddress) (ctrl.Result, error) {
	if address.Name != "" {
		var err error
		if controllerutil.RemoveFinalizer(address, ProtectAddressFinalizer) {
			if err = r.Client.Update(ctx, address); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, errors.Wrap(err, "failed to remove address finalizer")
			}
		}

		if err == nil {
			if err := r.Client.Delete(ctx, address); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
	}

	controllerutil.RemoveFinalizer(claim, ReleaseAddressFinalizer)
	return ctrl.Result{}, nil
}

func (r *IPAddressClaimReconciler) allocateAddress(claim *ipamv1.IPAddressClaim, pool pooltypes.GenericInClusterPool, addressesInUse []ipamv1.IPAddress) (*ipamv1.IPAddress, error) {
	poolSpec := pool.PoolSpec()

	inUseIPSet, err := poolutil.AddressesToIPSet(buildAddressList(addressesInUse, poolSpec.Gateway))
	if err != nil {
		return nil, fmt.Errorf("failed to convert IPAddressList to IPSet: %w", err)
	}

	poolIPSet, err := poolutil.IPPoolSpecToIPSet(pool.PoolSpec())
	if err != nil {
		return nil, fmt.Errorf("failed to convert pool to range: %w", err)
	}

	freeIP, err := poolutil.FindFreeAddress(poolIPSet, inUseIPSet)
	if err != nil {
		return nil, fmt.Errorf("failed to find free address: %w", err)
	}

	address := ipamutil.NewIPAddress(claim, pool)
	address.Spec.Address = freeIP.String()
	address.Spec.Gateway = poolSpec.Gateway
	address.Spec.Prefix = poolSpec.Prefix

	return &address, nil
}

func (r *IPAddressClaimReconciler) clusterToIPClaims(a client.Object) []reconcile.Request {
	requests := []reconcile.Request{}
	vms := &ipamv1.IPAddressClaimList{}
	err := r.Client.List(context.Background(), vms, client.MatchingLabels(
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
