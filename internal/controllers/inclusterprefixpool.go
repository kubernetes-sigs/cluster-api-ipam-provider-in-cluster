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

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	pooltypes "sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

const (
	inClusterPrefixPoolKind       = "InClusterPrefixPool"
	globalInClusterPrefixPoolKind = "GlobalInClusterPrefixPool"
)

// InClusterPrefixPoolReconciler reconciles an InClusterPrefixPool object.
type InClusterPrefixPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *InClusterPrefixPoolReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.InClusterPrefixPool{}).
		Watches(
			&ipamv1.IPAddress{},
			handler.EnqueueRequestsFromMapFunc(r.ipAddressToInClusterPrefixPool)).
		Complete(r)
}

func (r *InClusterPrefixPoolReconciler) ipAddressToInClusterPrefixPool(_ context.Context, clientObj client.Object) []reconcile.Request {
	ipAddress, ok := clientObj.(*ipamv1.IPAddress)
	if !ok {
		return nil
	}

	if ipAddress.Spec.PoolRef.APIGroup == v1alpha2.GroupVersion.Group &&
		ipAddress.Spec.PoolRef.Kind == inClusterPrefixPoolKind {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Namespace: ipAddress.Namespace,
				Name:      ipAddress.Spec.PoolRef.Name,
			},
		}}
	}

	return nil
}

// GlobalInClusterPrefixPoolReconciler reconciles a GlobalInClusterPrefixPool object.
type GlobalInClusterPrefixPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalInClusterPrefixPoolReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.GlobalInClusterPrefixPool{}).
		Watches(
			&ipamv1.IPAddress{},
			handler.EnqueueRequestsFromMapFunc(r.ipAddressToGlobalInClusterPrefixPool)).
		Complete(r)
}

func (r *GlobalInClusterPrefixPoolReconciler) ipAddressToGlobalInClusterPrefixPool(_ context.Context, clientObj client.Object) []reconcile.Request {
	ipAddress, ok := clientObj.(*ipamv1.IPAddress)
	if !ok {
		return nil
	}

	if ipAddress.Spec.PoolRef.APIGroup == v1alpha2.GroupVersion.Group &&
		ipAddress.Spec.PoolRef.Kind == globalInClusterPrefixPoolKind {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Namespace: ipAddress.Namespace,
				Name:      ipAddress.Spec.PoolRef.Name,
			},
		}}
	}

	return nil
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterprefixpools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterprefixpools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterprefixpools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *InClusterPrefixPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling prefix pool")

	pool := &v1alpha2.InClusterPrefixPool{}
	if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to fetch InClusterPrefixPool")
		}
		return ctrl.Result{}, nil
	}
	return genericPrefixReconcile(ctx, r.Client, pool)
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterprefixpools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterprefixpools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=globalinclusterprefixpools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GlobalInClusterPrefixPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling prefix pool")

	pool := &v1alpha2.GlobalInClusterPrefixPool{}
	if err := r.Get(ctx, req.NamespacedName, pool); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, errors.Wrap(err, "failed to fetch GlobalInClusterPrefixPool")
		}
		return ctrl.Result{}, nil
	}
	return genericPrefixReconcile(ctx, r.Client, pool)
}

func genericPrefixReconcile(ctx context.Context, c client.Client, pool pooltypes.GenericInClusterPrefixPool) (_ ctrl.Result, reterr error) {
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

	poolTypeRef := ipamv1.IPPoolReference{
		APIGroup: v1alpha2.GroupVersion.Group,
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

	config := poolutil.NewPrefixPoolConfig(pool.PoolSpec())
	total, err := poolutil.PrefixCandidateCount(config)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to count allocatable prefixes")
	}

	outOfRange := 0
	for _, inUse := range addressesInUse {
		prefix, err := poolutil.PrefixFromIPAddress(inUse)
		if err != nil || !poolutil.PrefixIsAllocatable(prefix, config) {
			outOfRange++
		}
	}

	// Prefix pool status accounting intentionally differs from IP address pool:
	// OutOfRange counts allocations whose prefix no longer matches the current spec
	// (e.g. after an excludedPrefixes change).
	// Free reports remaining allocatable capacity, so out-of-range allocations
	// are added back when computing free because they don't consume a valid slot.
	free := max(0, total-inUseCount+outOfRange)

	pool.PoolStatus().Addresses = &v1alpha2.InClusterPrefixPoolStatusAddresses{
		Total:      total,
		Used:       inUseCount,
		Free:       free,
		OutOfRange: outOfRange,
	}

	log.Info("Updating prefix pool with usage info", "statusAddresses", pool.PoolStatus().Addresses)

	return ctrl.Result{}, nil
}
