package webhooks

import (
	"context"
	"fmt"

	"inet.af/netaddr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
)

func (webhook *InClusterIPPool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.InClusterIPPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-ipam-cluster-x-k8s-io-v1alpha1-inclusterippool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterippools,versions=v1alpha1,name=validation.inclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-ipam-cluster-x-k8s-io-v1alpha1-inclusterippool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterippools,versions=v1alpha1,name=default.inclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// InClusterIPPool implements a validating and defaulting webhook for InClusterIPPool.
type InClusterIPPool struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &InClusterIPPool{}
var _ webhook.CustomValidator = &InClusterIPPool{}

// Default satisfies the defaulting webhook interface.
func (webhook *InClusterIPPool) Default(ctx context.Context, obj runtime.Object) error {
	pool, ok := obj.(*v1alpha1.InClusterIPPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool but got a %T", obj))
	}

	if pool.Spec.Subnet == "" {
		first, err := netaddr.ParseIP(pool.Spec.First)
		if err != nil {
			return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("InClusterIPPool").GroupKind(), pool.Name, field.ErrorList{
				field.Invalid(field.NewPath("spec", "subnet"), pool.Spec.Subnet, err.Error()),
			})
		}

		prefix, err := first.Prefix(uint8(pool.Spec.Prefix))
		if err != nil {
			return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("InClusterIPPool").GroupKind(), pool.Name, field.ErrorList{
				field.Invalid(field.NewPath("spec", "prefix"), pool.Spec.Prefix, err.Error()),
			})
		}

		pool.Spec.Subnet = prefix.String()
	} else {
		prefix, err := netaddr.ParseIPPrefix(pool.Spec.Subnet)
		if err != nil {
			return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("InClusterIPPool").GroupKind(), pool.Name, field.ErrorList{
				field.Invalid(field.NewPath("spec", "subnet"), pool.Spec.Subnet, err.Error()),
			})
		}

		if pool.Spec.First == "" {
			pool.Spec.First = prefix.Range().From().Next().String()
		}
		if pool.Spec.Last == "" {
			pool.Spec.Last = prefix.Range().To().Prior().String()
		}
		if pool.Spec.Prefix == 0 {
			pool.Spec.Prefix = int(prefix.Bits())
		}
	}

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	pool, ok := obj.(*v1alpha1.InClusterIPPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool but got a %T", obj))
	}
	return webhook.validate(ctx, nil, pool)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) error {
	newPool, ok := newObj.(*v1alpha1.InClusterIPPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool but got a %T", newObj))
	}
	oldPool, ok := oldObj.(*v1alpha1.InClusterIPPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool but got a %T", oldObj))
	}
	return webhook.validate(ctx, oldPool, newPool)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateDelete(_ context.Context, obj runtime.Object) (reterr error) {
	return nil
}

func (webhook *InClusterIPPool) validate(ctx context.Context, oldPool, newPool *v1alpha1.InClusterIPPool) (reterr error) {
	var allErrs field.ErrorList
	defer func() {
		if len(allErrs) > 0 {
			reterr = apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("InClusterIPPool").GroupKind(), newPool.Name, allErrs)
		}
	}()

	prefix, err := netaddr.ParseIPPrefix(newPool.Spec.Subnet)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "subnet"), newPool.Spec.Subnet, err.Error()))
		return
	}

	first, err := netaddr.ParseIP(newPool.Spec.First)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "first"), newPool.Spec.First, err.Error()))
	} else if !prefix.Contains(first) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "first"), newPool.Spec.First, "address is not part of spec.subnet"))
	}

	last, err := netaddr.ParseIP(newPool.Spec.Last)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "last"), newPool.Spec.Last, err.Error()))
	} else if !prefix.Contains(last) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "last"), newPool.Spec.Last, "address is not part of spec.subnet"))
	}

	if prefix.Bits() != uint8(newPool.Spec.Prefix) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefix"), newPool.Spec.Prefix, "does not match prefix of spec.subnet"))
	}

	if first == (netaddr.IP{}) || last == (netaddr.IP{}) {
		return
	}

	return //nolint:nakedret
}
