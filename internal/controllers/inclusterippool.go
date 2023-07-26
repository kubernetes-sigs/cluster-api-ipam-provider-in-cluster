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
	"net/netip"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	pooltypes "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

const (
	// ProtectPoolFinalizer is used to prevent deletion of a Pool object while its addresses have not been deleted.
	ProtectPoolFinalizer = "ipam.cluster.x-k8s.io/ProtectPool"
)

// InClusterIPPoolReconciler reconciles a InClusterIPPool object.
type InClusterIPPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *InClusterIPPoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.InClusterIPPool{}).
		Watches(&source.Kind{Type: &ipamv1.IPAddress{}},
			handler.EnqueueRequestsFromMapFunc(r.ipAddressToInClusterIPPool)).
		Complete(r)
}

func (r *InClusterIPPoolReconciler) ipAddressToInClusterIPPool(clientObj client.Object) []reconcile.Request {
	ipAddress, ok := clientObj.(*ipamv1.IPAddress)
	if !ok {
		return nil
	}

	if ipAddress.Spec.PoolRef.APIGroup != nil &&
		*ipAddress.Spec.PoolRef.APIGroup == v1alpha2.GroupVersion.Group &&
		ipAddress.Spec.PoolRef.Kind == poolutil.InClusterIPPoolKind {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Namespace: ipAddress.Namespace,
				Name:      ipAddress.Spec.PoolRef.Name,
			},
		}}
	}

	return nil
}

// GlobalInClusterIPPoolReconciler reconciles a GlobalInClusterIPPool object.
type GlobalInClusterIPPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalInClusterIPPoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.GlobalInClusterIPPool{}).
		Watches(&source.Kind{Type: &ipamv1.IPAddress{}},
			handler.EnqueueRequestsFromMapFunc(r.ipAddressToGlobalInClusterIPPool)).
		Complete(r)
}

func (r *GlobalInClusterIPPoolReconciler) ipAddressToGlobalInClusterIPPool(clientObj client.Object) []reconcile.Request {
	ipAddress, ok := clientObj.(*ipamv1.IPAddress)
	if !ok {
		return nil
	}

	if ipAddress.Spec.PoolRef.APIGroup != nil &&
		*ipAddress.Spec.PoolRef.APIGroup == v1alpha2.GroupVersion.Group &&
		ipAddress.Spec.PoolRef.Kind == poolutil.GlobalInClusterIPPoolKind {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Namespace: ipAddress.Namespace,
				Name:      ipAddress.Spec.PoolRef.Name,
			},
		}}
	}

	return nil
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *InClusterIPPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling pool")

	pool := &v1alpha2.InClusterIPPool{}
	if err := r.Client.Get(ctx, req.NamespacedName, pool); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to fetch InClusterIPPool")
		}
		return ctrl.Result{}, nil
	}
	return genericReconcile(ctx, r.Client, pool)
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GlobalInClusterIPPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling pool")

	pool := &v1alpha2.GlobalInClusterIPPool{}
	if err := r.Client.Get(ctx, req.NamespacedName, pool); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to fetch GlobalInClusterIPPool")
		}
		return ctrl.Result{}, nil
	}
	return genericReconcile(ctx, r.Client, pool)
}

func genericReconcile(ctx context.Context, c client.Client, pool pooltypes.GenericInClusterPool) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	patchHelper, err := patch.NewHelper(pool, c)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, pool); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	poolTypeRef := corev1.TypedLocalObjectReference{
		APIGroup: pointer.String(v1alpha2.GroupVersion.Group),
		Kind:     pool.GetObjectKind().GroupVersionKind().Kind,
		Name:     pool.GetName(),
	}

	addressesInUse, err := poolutil.ListAddressesInUse(ctx, c, pool.GetNamespace(), poolTypeRef)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to list addresses")
	}

	inUseCount := len(addressesInUse)

	if !controllerutil.ContainsFinalizer(pool, ProtectPoolFinalizer) {
		controllerutil.AddFinalizer(pool, ProtectPoolFinalizer)
	}

	if !pool.GetDeletionTimestamp().IsZero() {
		if inUseCount == 0 {
			controllerutil.RemoveFinalizer(pool, ProtectPoolFinalizer)
		}
		return ctrl.Result{}, nil
	}

	poolIPSet, err := poolutil.AddressesToIPSet(pool.PoolSpec().Addresses)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to build ip set from pool spec")
	}

	poolCount := poolutil.IPSetCount(poolIPSet)
	if pool.PoolSpec().Gateway != "" {
		gatewayAddr, err := netip.ParseAddr(pool.PoolSpec().Gateway)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to parse pool gateway")
		}

		if poolIPSet.Contains(gatewayAddr) {
			poolCount--
		}
	}

	free := poolCount - inUseCount
	outOfRangeIPSet, err := poolutil.AddressesOutOfRangeIPSet(addressesInUse, poolIPSet)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to build out of range ip set")
	}

	pool.PoolStatus().Addresses = &v1alpha2.InClusterIPPoolStatusIPAddresses{
		Total:      poolCount,
		Used:       inUseCount,
		Free:       free,
		OutOfRange: poolutil.IPSetCount(outOfRangeIPSet),
	}

	log.Info("Updating pool with usage info", "statusAddresses", pool.PoolStatus().Addresses)

	return ctrl.Result{}, nil
}
