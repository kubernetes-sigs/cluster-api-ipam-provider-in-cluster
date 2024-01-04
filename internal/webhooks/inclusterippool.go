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
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

const (
	// SkipValidateDeleteWebhookAnnotation is an annotation that can be applied
	// to the InClusterIPPool or GlobalInClusterIPPool to skip delete
	// validation. Necessary for clusterctl move to work as expected.
	SkipValidateDeleteWebhookAnnotation = "ipam.cluster.x-k8s.io/skip-validate-delete-webhook"
)

func (webhook *InClusterIPPool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha2.InClusterIPPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
	if err != nil {
		return err
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha2.GlobalInClusterIPPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1alpha2-inclusterippool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterippools,versions=v1alpha2,name=validation.inclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-ipam-cluster-x-k8s-io-v1alpha2-inclusterippool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterippools,versions=v1alpha2,name=default.inclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1alpha2-globalinclusterippool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,versions=v1alpha2,name=validation.globalinclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-ipam-cluster-x-k8s-io-v1alpha2-globalinclusterippool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=globalinclusterippools,versions=v1alpha2,name=default.globalinclusterippool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// InClusterIPPool implements a validating and defaulting webhook for InClusterIPPool and GlobalInClusterIPPool.
type InClusterIPPool struct {
	Client client.Reader
}

var (
	_ webhook.CustomDefaulter = &InClusterIPPool{}
	_ webhook.CustomValidator = &InClusterIPPool{}
)

// Default satisfies the defaulting webhook interface.
func (webhook *InClusterIPPool) Default(_ context.Context, obj runtime.Object) error {
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	pool, ok := obj.(types.GenericInClusterPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a InClusterIPPool or an GlobalInClusterIPPool but got a %T", obj))
	}
	return nil, webhook.validate(nil, pool)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newPool, ok := newObj.(types.GenericInClusterPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterIPPool or a GlobalInClusterIPPool but got a %T", newObj))
	}
	oldPool, ok := oldObj.(types.GenericInClusterPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterIPPool or a GlobalInClusterIPPool but got a %T", oldObj))
	}

	err := webhook.validate(oldPool, newPool)
	if err != nil {
		return nil, err
	}

	oldPoolRef := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(v1alpha2.GroupVersion.Group),
		Kind:     oldPool.GetObjectKind().GroupVersionKind().Kind,
		Name:     oldPool.GetName(),
	}
	inUseAddresses, err := poolutil.ListAddressesInUse(ctx, webhook.Client, oldPool.GetNamespace(), oldPoolRef)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	inUseBuilder := &netipx.IPSetBuilder{}
	for _, address := range inUseAddresses {
		ip, err := netip.ParseAddr(address.Spec.Address)
		if err != nil {
			// if an address we fetch for the pool is unparsable then it isn't in the pool ranges
			continue
		}
		inUseBuilder.Add(ip)
	}
	newPoolIPSet, err := poolutil.PoolSpecToIPSet(newPool.PoolSpec())
	if err != nil {
		// these addresses are already validated, this shouldn't happen
		return nil, apierrors.NewInternalError(err)
	}

	inUseBuilder.RemoveSet(newPoolIPSet)
	outOfRangeIPSet, err := inUseBuilder.IPSet()
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	if outOfRange := outOfRangeIPSet.Ranges(); len(outOfRange) > 0 {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("pool addresses do not contain allocated addresses: %v", outOfRange))
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterIPPool) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pool, ok := obj.(types.GenericInClusterPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterIPPool or a GlobalInClusterIPPool but got a %T", obj))
	}

	if _, ok := pool.GetAnnotations()[SkipValidateDeleteWebhookAnnotation]; ok {
		return nil, nil
	}

	poolTypeRef := corev1.TypedLocalObjectReference{
		APIGroup: ptr.To(pool.GetObjectKind().GroupVersionKind().Group),
		Kind:     pool.GetObjectKind().GroupVersionKind().Kind,
		Name:     pool.GetName(),
	}

	inUseAddresses, err := poolutil.ListAddressesInUse(ctx, webhook.Client, pool.GetNamespace(), poolTypeRef)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	if len(inUseAddresses) > 0 {
		return nil, apierrors.NewBadRequest("Pool has IPAddresses allocated. Cannot delete Pool until all IPAddresses have been removed.")
	}

	return nil, nil
}

func (webhook *InClusterIPPool) validate(_, newPool types.GenericInClusterPool) (reterr error) {
	var allErrs field.ErrorList
	defer func() {
		if len(allErrs) > 0 {
			reterr = apierrors.NewInvalid(v1alpha2.GroupVersion.WithKind(newPool.GetObjectKind().GroupVersionKind().Kind).GroupKind(), newPool.GetName(), allErrs)
		}
	}()

	if len(newPool.PoolSpec().Addresses) == 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "addresses"), newPool.PoolSpec().Addresses, "addresses is required"))
	}

	if newPool.PoolSpec().Prefix == 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefix"), newPool.PoolSpec().Prefix, "a valid prefix is required"))
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

	excludedHasIPv4Addr, excludedHasIPv6Addr := false, false
	for _, address := range newPool.PoolSpec().ExcludedAddresses {
		ipSet, err := poolutil.AddressToIPSet(address)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "excludedAddresses"), address, "provided address is not a valid IP, range, nor CIDR"))
			continue
		}
		from := ipSet.Ranges()[0].From()
		excludedHasIPv4Addr = excludedHasIPv4Addr || from.Is4()
		excludedHasIPv6Addr = excludedHasIPv6Addr || from.Is6()
	}

	if excludedHasIPv4Addr && excludedHasIPv6Addr {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "excludedAddresses"), newPool.PoolSpec().ExcludedAddresses, "provided addresses are of mixed IP families"))
	}

	if (hasIPv4Addr && excludedHasIPv6Addr) || (hasIPv6Addr && excludedHasIPv4Addr) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "excludedAddresses"), newPool.PoolSpec().ExcludedAddresses, "addresses and excluded addresses are of mixed IP families"))
	}

	if len(allErrs) == 0 {
		errs := validateAddressesAreWithinPrefix(newPool.PoolSpec())
		if len(errs) != 0 {
			allErrs = append(allErrs, errs...)
		}
	}

	return //nolint:nakedret
}

func validatePrefix(spec *v1alpha2.InClusterIPPoolSpec) (*netipx.IPSet, field.ErrorList) {
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

func validateAddressesAreWithinPrefix(spec *v1alpha2.InClusterIPPoolSpec) field.ErrorList {
	var errors field.ErrorList

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
