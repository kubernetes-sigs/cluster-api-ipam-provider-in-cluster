package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"inet.af/netaddr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterv1exp "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/ipamutil"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/predicates"
)

const ReleaseAddressFinalizer = "ipam.cluster.x-k8s.io/ReleaseAddress"

// IPAddressClaimReconciler reconciles a InClusterIPPool object
type IPAddressClaimReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAddressClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1exp.IPAddressClaim{}).
		WithEventFilter(predicates.ClaimReferencesPoolKind(v1.GroupKind{
			Group: v1alpha1.GroupVersion.Group,
			Kind:  reflect.TypeOf(v1alpha1.InClusterIPPool{}).Name(),
		})).
		Complete(r)
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=ipaddressclaims/status;ipaddresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *IPAddressClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling claim")

	// Fetch the IPAddressClaim
	claim := &clusterv1exp.IPAddressClaim{}
	if err := r.Client.Get(ctx, req.NamespacedName, claim); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue
		return ctrl.Result{}, err
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

	pool := &v1alpha1.InClusterIPPool{}
	if err := r.Client.Get(ctx, claim.Spec.Pool.NamespacedName(claim.Namespace), pool); err != nil {
		if !apierrors.IsNotFound(err) {
			// Error reading pool - requeue
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(claim, ReleaseAddressFinalizer) {
		controllerutil.AddFinalizer(claim, ReleaseAddressFinalizer)
	}

	if !claim.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, claim, pool)
	}

	addresses, err := poolutil.ListAddresses(ctx, claim.Spec.Pool.Name, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list addresses: %w", err)
	}

	return r.reconcile(ctx, claim, pool, addresses)
}

func (r *IPAddressClaimReconciler) reconcile(ctx context.Context, c *clusterv1exp.IPAddressClaim, pool *v1alpha1.InClusterIPPool, addresses *clusterv1exp.IPAddressList) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx, "pool name", pool.Name)

	if pool == nil {
		err := errors.New("pool not found")
		log.Error(err, "the referenced pool could not be found")
		return ctrl.Result{}, nil
	}

	address := poolutil.AddressByName(addresses, c.Name)
	if address == nil {
		var err error
		address, err = r.allocateAddress(ctx, c, pool, addresses)
		if err != nil {
			log.Error(err, "failed to allocate address")
			return ctrl.Result{}, err
		}
	}

	c.Status.Address = *clusterv1.NewPinnedLocalObjectReference(address)

	return ctrl.Result{}, nil
}

func (r *IPAddressClaimReconciler) reconcileDelete(ctx context.Context, c *clusterv1exp.IPAddressClaim, pool *v1alpha1.InClusterIPPool) (ctrl.Result, error) {
	address := &clusterv1exp.IPAddress{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: c.Name, Namespace: c.Namespace}, address); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if address.Name != "" {
		if err := r.Client.Delete(ctx, address); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(c, ReleaseAddressFinalizer)
	return ctrl.Result{}, nil
}

func (r *IPAddressClaimReconciler) allocateAddress(ctx context.Context, c *clusterv1exp.IPAddressClaim, pool *v1alpha1.InClusterIPPool, addresses *clusterv1exp.IPAddressList) (*clusterv1exp.IPAddress, error) {
	existing, err := poolutil.IPAddressListToSet(addresses, pool.Spec.Gateway)
	if err != nil {
		return nil, fmt.Errorf("failed to convert IPAddressList to set: %w", err)
	}

	iprange, err := ipPoolToRange(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to convert pool to range: %w", err)
	}

	free, err := poolutil.FindFreeAddress(iprange, existing)
	if err != nil {
		return nil, fmt.Errorf("failed to find free address: %w", err)
	}

	address := ipamutil.NewIPAddress(c, pool)
	address.Spec.Address = free.String()
	address.Spec.Gateway = pool.Spec.Gateway
	address.Spec.Prefix = pool.Spec.Prefix

	if err := r.Client.Create(ctx, &address); err != nil {
		return nil, fmt.Errorf("failed to create IPAddress: %w", err)
	}

	return &address, nil
}

func ipPoolToRange(pool *v1alpha1.InClusterIPPool) (netaddr.IPRange, error) {
	start, err := netaddr.ParseIP(pool.Spec.First)
	if err != nil {
		return netaddr.IPRange{}, err
	}
	end, err := netaddr.ParseIP(pool.Spec.Last)
	if err != nil {
		return netaddr.IPRange{}, err
	}
	return netaddr.IPRangeFrom(start, end), nil
}
