package ipamutil

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	clusterutil "sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// ReleaseAddressFinalizer is used to release an IP address before cleaning up the claim.
	ReleaseAddressFinalizer = "ipam.cluster.x-k8s.io/ReleaseAddress"

	// ProtectAddressFinalizer is used to prevent deletion of an IPAddress object while its claim is not deleted.
	ProtectAddressFinalizer = "ipam.cluster.x-k8s.io/ProtectAddress"
)

// ClaimReconciler reconciles a IPAddressClaim object using a ProviderIntegration.
type ClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	WatchFilterValue string

	Provider ProviderIntegration
}

// ProviderIntegration is an interface that must be implemented by an IPAM provider.
type ProviderIntegration interface {
	// SetupWithManager allows the integration to configure the controller.
	SetupWithManager(context.Context, *ctrl.Builder) error
	// ClaimHandlerFor needs to return a ClaimHandler for the provider.
	ClaimHandlerFor(client.Client, *ipamv1.IPAddressClaim) ClaimHandler
}

// ClaimHandler knows how to allocate and release IP addresses for a specific provider.
type ClaimHandler interface {
	FetchPool(ctx context.Context) (*ctrl.Result, error)
	EnsureAddress(ctx context.Context, address *ipamv1.IPAddress) (*ctrl.Result, error)
	ReleaseAddress() (*ctrl.Result, error)
	GetPool() client.Object
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClaimReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue))

	if err := r.Provider.SetupWithManager(ctx, b); err != nil {
		return err
	}
	return b.Complete(r)
}

// Reconcile is called by the controller to reconcile a claim.
func (r *ClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling claim")

	// Fetch the IPAddressClaim
	claim := &ipamv1.IPAddressClaim{}
	if err := r.Get(ctx, req.NamespacedName, claim); err != nil {
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

	// Ensure the claim has a finalizer.
	controllerutil.AddFinalizer(claim, ReleaseAddressFinalizer)

	// Create the provider handler and fetch the pool.
	handler := r.Provider.ClaimHandlerFor(r.Client, claim)
	if res, err := handler.FetchPool(ctx); err != nil || res != nil {
		return unwrapResult(res), err
	}

	if annotations.HasPaused(handler.GetPool()) {
		log.Info("IPAddressClaim references Pool which is paused, skipping reconciliation.", "IPAddressClaim", claim.GetName(), "Pool", handler.GetPool().GetName())
		return ctrl.Result{}, nil
	}

	address := ipamv1.IPAddress{}
	if err := r.Client.Get(ctx, req.NamespacedName, &address); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, errors.Wrap(err, "failed to fetch address")
	}

	// If the claim is marked for deletion, release the address.
	if !claim.ObjectMeta.DeletionTimestamp.IsZero() {
		if res, err := handler.ReleaseAddress(); err != nil || res != nil {
			return unwrapResult(res), err
		}
		return r.reconcileDelete(ctx, claim, &address)
	}

	// We always ensure there is a valid address object passed to the handler.
	// The handler will complete it with the ip address.
	if address.Name == "" {
		address = newIPAddress(claim, handler.GetPool())
	}

	if res, err := handler.EnsureAddress(ctx, &address); err != nil || res != nil {
		return unwrapResult(res), err
	}

	// Patch or create the address, ensuring necessary owner references are set.
	operationResult, err := controllerutil.CreateOrPatch(ctx, r.Client, &address, func() error {
		if err := ensureIPAddressOwnerReferences(r.Scheme, &address, claim, handler.GetPool()); err != nil {
			return errors.Wrap(err, "failed to ensure owner references on address")
		}

		_ = controllerutil.AddFinalizer(&address, ProtectAddressFinalizer)

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

func (r *ClaimReconciler) reconcileDelete(ctx context.Context, claim *ipamv1.IPAddressClaim, address *ipamv1.IPAddress) (ctrl.Result, error) {
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

func unwrapResult(res *ctrl.Result) ctrl.Result {
	if res == nil {
		return ctrl.Result{}
	}
	return *res
}
