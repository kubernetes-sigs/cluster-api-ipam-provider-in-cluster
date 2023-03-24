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
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/internal/poolutil"
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

	if len(pool.Spec.Addresses) > 0 {
		return nil
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
			pool.Spec.First = prefix.Range().From().Next().String() // omits the first address, the assumed gateway
		}
		if pool.Spec.Last == "" {
			pool.Spec.Last = prefix.Range().To().Prior().String() // omits the last address, the assumed broadcast
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
	return webhook.validate(nil, pool)
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
	return webhook.validate(oldPool, newPool)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateDelete(_ context.Context, obj runtime.Object) (reterr error) {
	return nil
}

func (webhook *InClusterIPPool) validate(_, newPool *v1alpha1.InClusterIPPool) (reterr error) {
	var allErrs field.ErrorList
	defer func() {
		if len(allErrs) > 0 {
			reterr = apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind("InClusterIPPool").GroupKind(), newPool.Name, allErrs)
		}
	}()

	if len(newPool.Spec.Addresses) > 0 {
		allErrs = append(allErrs, validateAddresses(newPool)...)
		return
	}

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

	if newPool.Spec.Gateway != "" {
		_, err := netaddr.ParseIP(newPool.Spec.Gateway)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "gateway"), newPool.Spec.Gateway, err.Error()))
		}
	}

	return //nolint:nakedret
}

func validateAddresses(newPool *v1alpha1.InClusterIPPool) field.ErrorList {
	var allErrs field.ErrorList

	if newPool.Spec.Subnet != "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "subnet"), newPool.Spec.Subnet, "subnet may not be used with addresses"))
	}

	if newPool.Spec.First != "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "start"), newPool.Spec.First, "start may not be used with addresses"))
	}

	if newPool.Spec.Last != "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "end"), newPool.Spec.Last, "end may not be used with addresses"))
	}

	if newPool.Spec.Prefix == 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefix"), newPool.Spec.Prefix, "a valid prefix is required when using addresses"))
	}

	if newPool.Spec.Gateway != "" {
		_, err := netaddr.ParseIP(newPool.Spec.Gateway)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "gateway"), newPool.Spec.Gateway, err.Error()))
		}
	}

	for _, address := range newPool.Spec.Addresses {
		if !poolutil.AddressStrParses(address) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addresses"), address, "provided address is not a valid IP, range, nor CIDR"))
			continue
		}
	}

	if len(allErrs) == 0 {
		errs := validateAddressesAreWithinPrefix(newPool.Spec)
		if len(errs) != 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	return allErrs
}

func validatePrefix(spec v1alpha1.InClusterIPPoolSpec) (*netaddr.IPSet, field.ErrorList) {
	var errors field.ErrorList

	addressesIPSet, err := poolutil.AddressesToIPSet(spec.Addresses)
	if err != nil {
		// this should not occur, previous validation should have caught problems here.
		errors := append(errors, field.Invalid(field.NewPath("spec", "addresses"), spec.Addresses, err.Error()))
		return &netaddr.IPSet{}, errors
	}

	firstIPInAddresses := addressesIPSet.Ranges()[0].From() // safe because of prior validation
	prefix, err := netaddr.ParseIPPrefix(fmt.Sprintf("%s/%d", firstIPInAddresses, spec.Prefix))
	if err != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec", "prefix"), spec.Prefix, "provided prefix is not valid"))
		return &netaddr.IPSet{}, errors
	}

	builder := netaddr.IPSetBuilder{}
	builder.AddPrefix(prefix)
	prefixIPSet, err := builder.IPSet()
	if err != nil {
		// This should not occur, the prefix has been validated. Converting the prefix to an IPSet
		// for it's ContainsRange function.
		errors := append(errors, field.Invalid(field.NewPath("spec", "prefix"), spec.Prefix, err.Error()))
		return &netaddr.IPSet{}, errors
	}

	return prefixIPSet, errors
}

func validateAddressesAreWithinPrefix(spec v1alpha1.InClusterIPPoolSpec) field.ErrorList {
	var errors field.ErrorList

	if len(spec.Addresses) == 0 {
		return errors
	}

	prefixIPSet, prefixErrs := validatePrefix(spec)
	if len(prefixErrs) > 0 {
		return prefixErrs
	}

	for _, addressStr := range spec.Addresses {
		addressIPSet, err := poolutil.AddressToIPSet(addressStr)
		if err != nil {
			// this should never occur, previous validations will have caught this.
			errors = append(errors, field.Invalid(field.NewPath("spec", "addresses"), addressStr, "provided address is not a valid IP, range, nor CIDR"))
			continue
		}
		// We know that each addressIPSet should be made up of only one range, it came from a single addressStr
		if !prefixIPSet.ContainsRange(addressIPSet.Ranges()[0]) {
			errors = append(errors, field.Invalid(field.NewPath("spec", "addresses"), addressStr, "provided address belongs to a different subnet than others"))
			continue
		}
	}

	return errors
}
