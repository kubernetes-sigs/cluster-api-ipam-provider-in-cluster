package ipamutil

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	clusterutil "sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ReleaseAddressFinalizer is used to release an IP address before cleaning up the claim.
	ReleaseAddressFinalizer = "ipam.cluster.x-k8s.io/ReleaseAddress"

	// ProtectAddressFinalizer is used to prevent deletion of an IPAddress object while its claim is not deleted.
	ProtectAddressFinalizer = "ipam.cluster.x-k8s.io/ProtectAddress"
)

// ClaimReconciler reconciles a IPAddressClaim object using a ProviderAdapter.
// It can be used to implement custom IPAM providers without worrying about the basic lifecycle, pausing and owner
// references, which should be the same or very similar for any provider.
// The custom implementation for a specific provider is provided by implementing the ProviderAdapter interface. The
// ClaimReconciler can then be used as follows, with controllers.InClusterProviderAdapter serving
// as the provider implementation.
//
//	(&ipamutil.ClaimReconciler{
//		Client:           mgr.GetClient(),
//		Scheme:           mgr.GetScheme(),
//		WatchFilterValue: watchFilter,
//		Adapter: &controllers.InClusterProviderAdapter{},
//	}).SetupWithManager(ctx, mgr)
type ClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	WatchFilterValue string

	Adapter ProviderAdapter
}

// ProviderAdapter is an interface that must be implemented by the IPAM provider.
type ProviderAdapter interface {
	// SetupWithManager will be called during the setup of the controller for the ClaimReconciler to allow the provider
	// implementation to extend the controller configuration.
	SetupWithManager(context.Context, *ctrl.Builder) error
	// ClaimHandlerFor is called during reconciliation to get a ClaimHandler for the reconciled [ipamv1.IPAddressClaim].
	ClaimHandlerFor(client.Client, *ipamv1.IPAddressClaim) ClaimHandler
}

// ClaimHandler knows how to allocate and release IP addresses for a specific provider.
type ClaimHandler interface {
	// FetchPool is called to fetch the pool referenced by the claim. The pool needs to be stored by the handler.
	FetchPool(ctx context.Context) (client.Object, *ctrl.Result, error)
	// EnsureAddress is called to make sure that the IPAddress.Spec is correct and the address is allocated.
	EnsureAddress(ctx context.Context, address *ipamv1.IPAddress) (*ctrl.Result, error)
	// ReleaseAddress is called to release the ip address that was allocated for the claim.
	ReleaseAddress() (*ctrl.Result, error)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClaimReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if r.Adapter == nil {
		return fmt.Errorf("error setting the manager: Adapter is nil")
	}

	b := ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		// A Watch is added for the Cluster in the case that the Cluster is
		// unpaused so that a request can be queued to re-reconcile the
		// IPAddressClaim.
		Watches(
			&clusterv1.Cluster{},
			ctrlhandler.EnqueueRequestsFromMapFunc(r.clusterToIPClaims),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldCluster := e.ObjectOld.(*clusterv1.Cluster)
					newCluster := e.ObjectNew.(*clusterv1.Cluster)
					return annotations.IsPaused(oldCluster, oldCluster) && !annotations.IsPaused(newCluster, newCluster)
				},
				CreateFunc: func(e event.CreateEvent) bool {
					cluster := e.Object.(*clusterv1.Cluster)
					return !annotations.IsPaused(cluster, cluster)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					cluster := e.Object.(*clusterv1.Cluster)
					return !annotations.IsPaused(cluster, cluster)
				},
			}),
		)

	if err := r.Adapter.SetupWithManager(ctx, b); err != nil {
		return err
	}
	return b.Complete(r)
}

// Reconcile is called by the controller to reconcile a claim.
func (r *ClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the IPAddressClaim
	claim := &ipamv1.IPAddressClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if the owning cluster is paused
	if _, ok := claim.GetLabels()[clusterv1.ClusterNameLabel]; ok {
		cluster, err := clusterutil.GetClusterFromMetadata(ctx, r.Client, claim.ObjectMeta)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("IPAddressClaim linked to a cluster that is not found, unable to determine cluster's paused state, skipping reconciliation")
				return ctrl.Result{}, nil
			}

			log.Error(err, "error fetching cluster linked to IPAddressClaim")
			return ctrl.Result{}, err
		}

		if annotations.IsPaused(cluster, cluster) {
			log.Info("IPAddressClaim linked to a cluster that is paused, skipping reconciliation")
			return ctrl.Result{}, nil
		}
	}

	// Create a patch helper for the claim.
	patchHelper, err := patch.NewHelper(claim, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, claim); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	controllerutil.AddFinalizer(claim, ReleaseAddressFinalizer)

	var res *reconcile.Result
	var pool client.Object

	// Create the provider handler and fetch the pool.
	handler := r.Adapter.ClaimHandlerFor(r.Client, claim)
	if pool, res, err = handler.FetchPool(ctx); err != nil || res != nil {
		if apierrors.IsNotFound(err) {
			err := errors.New("pool not found")
			log.Error(err, "the referenced pool could not be found")
			if !claim.ObjectMeta.DeletionTimestamp.IsZero() {
				return r.reconcileDelete(ctx, claim)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "failed to fetch pool")
	}

	if pool == nil {
		err = fmt.Errorf("pool is nil")
		log.Error(err, "pool error")
		return ctrl.Result{}, errors.Wrap(err, "reconciliation failed")
	}

	if annotations.HasPaused(pool) {
		log.Info("IPAddressClaim references Pool which is paused, skipping reconciliation.", "IPAddressClaim", claim.GetName(), "Pool", pool.GetName())
		return ctrl.Result{}, nil
	}

	// If the claim is marked for deletion, release the address.
	if !claim.ObjectMeta.DeletionTimestamp.IsZero() {
		if res, err := handler.ReleaseAddress(); err != nil {
			return unwrapResult(res), err
		}
		return r.reconcileDelete(ctx, claim)
	}

	// We always ensure there is a valid address object passed to the handler.
	// The handler will complete it with the ip address.
	address := NewIPAddress(claim, pool)

	// Patch or create the address, ensuring necessary owner references are set.
	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &address, func() error {
		if res, err = handler.EnsureAddress(ctx, &address); err != nil {
			return err
		}

		if err = ensureIPAddressOwnerReferences(r.Scheme, &address, claim, pool); err != nil {
			return errors.Wrap(err, "failed to ensure owner references on address")
		}

		_ = controllerutil.AddFinalizer(&address, ProtectAddressFinalizer)

		return nil
	})

	if res != nil || err != nil {
		if err != nil {
			err = errors.Wrap(err, "failed to create or patch address")
		}
		return unwrapResult(res), err
	}

	log.Info(fmt.Sprintf("IPAddress %s/%s (%s) has been %s", address.Namespace, address.Name, address.Spec.Address, operationResult),
		"IPAddressClaim", fmt.Sprintf("%s/%s", claim.Namespace, claim.Name))

	if !address.DeletionTimestamp.IsZero() {
		// We prevent deleting IPAddresses while their corresponding IPClaim still exists since we cannot guarantee that the IP
		// wil remain the same when we recreate it.
		log.Info("Address is marked for deletion, but deletion is prevented until the claim is deleted as well", "address", address.Name)
	}

	claim.Status.AddressRef = corev1.LocalObjectReference{Name: address.Name}

	return ctrl.Result{}, nil
}

func (r *ClaimReconciler) reconcileDelete(ctx context.Context, claim *ipamv1.IPAddressClaim) (ctrl.Result, error) {
	address := &ipamv1.IPAddress{}
	namespacedName := types.NamespacedName{
		Namespace: claim.Namespace,
		Name:      claim.Name,
	}
	if err := r.Client.Get(ctx, namespacedName, address); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrap(err, "failed to fetch address")
	}

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

func (r *ClaimReconciler) clusterToIPClaims(_ context.Context, a client.Object) []reconcile.Request {
	requests := []reconcile.Request{}
	claims := &ipamv1.IPAddressClaimList{}
	err := r.Client.List(context.Background(), claims, client.MatchingLabels(
		map[string]string{
			clusterv1.ClusterNameLabel: a.GetName(),
		},
	))
	if err != nil {
		return requests
	}
	for _, c := range claims.Items {
		r := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      c.Name,
				Namespace: c.Namespace,
			},
		}
		requests = append(requests, r)
	}
	return requests
}

func unwrapResult(res *ctrl.Result) ctrl.Result {
	if res == nil {
		return ctrl.Result{}
	}
	return *res
}
