package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ipamclusterxk8siov1alpha1 "github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

// InClusterIPPoolReconciler reconciles a InClusterIPPool object
type InClusterIPPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *InClusterIPPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ipamclusterxk8siov1alpha1.InClusterIPPool{}).
		Watches(
			&source.Kind{Type: &clusterv1.IPAddressClaim{}},
			handler.EnqueueRequestsFromMapFunc(r.IPAddressClaimToPool),
		).
		Complete(r)
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=inclusterippools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the InClusterIPPool object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *InClusterIPPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	return ctrl.Result{}, nil
}

func (r *InClusterIPPoolReconciler) IPAddressClaimToPool(obj client.Object) []ctrl.Request {
	claim, ok := obj.(*clusterv1.IPAddressClaim)
	if !ok {
		return nil
	}

	if claim.Spec.PoolRef.Kind != "InClusterIPPool" {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: types.NamespacedName{Name: claim.Spec.PoolRef.Name, Namespace: claim.Namespace},
		},
	}
}
