package webhooks

import (
	"context"
	"fmt"
	"net/netip"

	"go4.org/netipx"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/telekom/cluster-api-ipam-provider-in-cluster/api/v1alpha1"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	"github.com/telekom/cluster-api-ipam-provider-in-cluster/pkg/types"
)

func (webhook *InClusterIPPool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.InClusterIPPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
	if err != nil {
		return err
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.GlobalInClusterIPPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1alpha1-inclusterippool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterippools,versions=v1alpha1,name=validation.inclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-ipam-cluster-x-k8s-io-v1alpha1-inclusterippool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterippools,versions=v1alpha1,name=default.inclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1alpha1-globalinclusterippool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,versions=v1alpha1,name=validation.globalinclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-ipam-cluster-x-k8s-io-v1alpha1-globalinclusterippool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,versions=v1alpha1,name=default.globalinclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// InClusterIPPool implements a validating and defaulting webhook for InClusterIPPool and GlobalInClusterIPPool.
type InClusterIPPool struct {
	Client client.Reader
}

var _ webhook.CustomDefaulter = &InClusterIPPool{}
var _ webhook.CustomValidator = &InClusterIPPool{}

// Default satisfies the defaulting webhook interface.
func (webhook *InClusterIPPool) Default(_ context.Context, obj runtime.Object) error {
	pool, ok := obj.(types.GenericInClusterPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool or an GlobalInClusterIPPool but got a %T", obj))
	}
	poolSpec := pool.PoolSpec()
	if len(poolSpec.Addresses) > 0 {
		return nil
	}

	if poolSpec.Subnet == "" {
		first, err := netip.ParseAddr(poolSpec.First)
		if err != nil {
			return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(pool.GetObjectKind().GroupVersionKind().Kind).GroupKind(), pool.GetName(), field.ErrorList{
				field.Invalid(field.NewPath("spec", "subnet"), poolSpec.Subnet, err.Error()),
			})
		}

		prefix, err := first.Prefix(poolSpec.Prefix)
		if err != nil {
			return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(pool.GetObjectKind().GroupVersionKind().Kind).GroupKind(), pool.GetName(), field.ErrorList{
				field.Invalid(field.NewPath("spec", "prefix"), poolSpec.Prefix, err.Error()),
			})
		}

		poolSpec.Subnet = prefix.String()
	} else {
		prefix, err := netip.ParsePrefix(poolSpec.Subnet)
		if err != nil {
			return apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(pool.GetObjectKind().GroupVersionKind().Kind).GroupKind(), pool.GetName(), field.ErrorList{
				field.Invalid(field.NewPath("spec", "subnet"), poolSpec.Subnet, err.Error()),
			})
		}

		prefixRange := netipx.RangeOfPrefix(prefix)
		if poolSpec.First == "" {
			poolSpec.First = prefixRange.From().Next().String() // omits the first address, the assumed gateway
		}
		if poolSpec.Last == "" {
			poolSpec.Last = prefixRange.To().Prev().String() // omits the last address, the assumed broadcast
		}
		if poolSpec.Prefix == 0 {
			poolSpec.Prefix = prefix.Bits()
		}
	}

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateCreate(_ context.Context, obj runtime.Object) error {
	pool, ok := obj.(types.GenericInClusterPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool or an GlobalInClusterIPPool but got a %T", obj))
	}
	return webhook.validate(nil, pool)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) error {
	newPool, ok := newObj.(types.GenericInClusterPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool or an GlobalInClusterIPPool but got a %T", newObj))
	}
	oldPool, ok := oldObj.(types.GenericInClusterPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool or an GlobalInClusterIPPool but got a %T", oldObj))
	}
	return webhook.validate(oldPool, newPool)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateDelete(ctx context.Context, obj runtime.Object) (reterr error) {
	pool, ok := obj.(types.GenericInClusterPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool or an GlobalInClusterIPPool but got a %T", obj))
	}

	poolTypeRef := corev1.TypedLocalObjectReference{
		APIGroup: pointer.String(pool.GetObjectKind().GroupVersionKind().Group),
		Kind:     pool.GetObjectKind().GroupVersionKind().Kind,
		Name:     pool.GetName(),
	}

	inUseAddresses, err := poolutil.ListAddressesInUse(ctx, webhook.Client, pool.GetNamespace(), poolTypeRef)
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	if len(inUseAddresses) > 0 {
		return apierrors.NewBadRequest("Pool has IPAddresses allocated. Cannot delete Pool until all IPAddresses have been removed.")
	}

	return nil
}

func (webhook *InClusterIPPool) validate(_, newPool types.GenericInClusterPool) (reterr error) {
	var allErrs field.ErrorList
	defer func() {
		if len(allErrs) > 0 {
			reterr = apierrors.NewInvalid(v1alpha1.GroupVersion.WithKind(newPool.GetObjectKind().GroupVersionKind().Kind).GroupKind(), newPool.GetName(), allErrs)
		}
	}()

	if len(newPool.PoolSpec().Addresses) > 0 {
		allErrs = append(allErrs, validateAddresses(newPool)...)
		return
	}

	prefix, err := netip.ParsePrefix(newPool.PoolSpec().Subnet)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "subnet"), newPool.PoolSpec().Subnet, err.Error()))
		return
	}

	first, err := netip.ParseAddr(newPool.PoolSpec().First)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "first"), newPool.PoolSpec().First, err.Error()))
	} else if !prefix.Contains(first) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "first"), newPool.PoolSpec().First, "address is not part of spec.subnet"))
	}

	last, err := netip.ParseAddr(newPool.PoolSpec().Last)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "last"), newPool.PoolSpec().Last, err.Error()))
	} else if !prefix.Contains(last) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "last"), newPool.PoolSpec().Last, "address is not part of spec.subnet"))
	}

	if prefix.Bits() != newPool.PoolSpec().Prefix {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefix"), newPool.PoolSpec().Prefix, "does not match prefix of spec.subnet"))
	}

	if newPool.PoolSpec().Gateway != "" {
		_, err := netip.ParseAddr(newPool.PoolSpec().Gateway)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "gateway"), newPool.PoolSpec().Gateway, err.Error()))
		}
	}

	return //nolint:nakedret
}

func validateAddresses(newPool types.GenericInClusterPool) field.ErrorList {
	var allErrs field.ErrorList

	if newPool.PoolSpec().Subnet != "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "subnet"), newPool.PoolSpec().Subnet, "subnet may not be used with addresses"))
	}

	if newPool.PoolSpec().First != "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "start"), newPool.PoolSpec().First, "start may not be used with addresses"))
	}

	if newPool.PoolSpec().Last != "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "end"), newPool.PoolSpec().Last, "end may not be used with addresses"))
	}

	if newPool.PoolSpec().Prefix == 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefix"), newPool.PoolSpec().Prefix, "a valid prefix is required when using addresses"))
	}

	var hasIPv4Addr, hasIPv6Addr bool
	for _, address := range newPool.PoolSpec().Addresses {
		ipSet, err := poolutil.AddressToIPSet(address)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addresses"), address, "provided address is not a valid IP, range, nor CIDR"))
			continue
		}
		from := ipSet.Ranges()[0].From()
		hasIPv4Addr = hasIPv4Addr || from.Is4()
		hasIPv6Addr = hasIPv6Addr || from.Is6()
	}
	if hasIPv4Addr && hasIPv6Addr {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addresses"), newPool.PoolSpec().Addresses, "provided addresses are of mixed IP families"))
	}

	if newPool.PoolSpec().Gateway != "" {
		gateway, err := netip.ParseAddr(newPool.PoolSpec().Gateway)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "gateway"), newPool.PoolSpec().Gateway, err.Error()))
		}

		if gateway.Is6() && hasIPv4Addr || gateway.Is4() && hasIPv6Addr {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "gateway"), newPool.PoolSpec().Gateway, "provided gateway and addresses are of mixed IP families"))
		}
	}

	if len(allErrs) == 0 {
		errs := validateAddressesAreWithinPrefix(newPool.PoolSpec())
		if len(errs) != 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	return allErrs
}

func validatePrefix(spec *v1alpha1.InClusterIPPoolSpec) (*netipx.IPSet, field.ErrorList) {
	var errors field.ErrorList

	addressesIPSet, err := poolutil.AddressesToIPSet(spec.Addresses)
	if err != nil {
		// this should not occur, previous validation should have caught problems here.
		errors := append(errors, field.Invalid(field.NewPath("spec", "addresses"), spec.Addresses, err.Error()))
		return &netipx.IPSet{}, errors
	}

	firstIPInAddresses := addressesIPSet.Ranges()[0].From() // safe because of prior validation
	prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", firstIPInAddresses, spec.Prefix))
	if err != nil {
		errors = append(errors, field.Invalid(field.NewPath("spec", "prefix"), spec.Prefix, "provided prefix is not valid"))
		return &netipx.IPSet{}, errors
	}

	builder := netipx.IPSetBuilder{}
	builder.AddPrefix(prefix)
	prefixIPSet, err := builder.IPSet()
	if err != nil {
		// This should not occur, the prefix has been validated. Converting the prefix to an IPSet
		// for it's ContainsRange function.
		errors := append(errors, field.Invalid(field.NewPath("spec", "prefix"), spec.Prefix, err.Error()))
		return &netipx.IPSet{}, errors
	}

	return prefixIPSet, errors
}

func validateAddressesAreWithinPrefix(spec *v1alpha1.InClusterIPPoolSpec) field.ErrorList {
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
