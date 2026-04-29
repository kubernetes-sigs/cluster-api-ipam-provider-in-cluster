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

package webhooks

import (
	"context"
	"fmt"
	"net/netip"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/api/v1alpha2"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/internal/poolutil"
	"sigs.k8s.io/cluster-api-ipam-provider-in-cluster/pkg/types"
)

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1alpha2-inclusterprefixpool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterprefixpools,versions=v1alpha2,name=validation.inclusterprefixpool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-ipam-cluster-x-k8s-io-v1alpha2-inclusterprefixpool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=inclusterprefixpools,versions=v1alpha2,name=default.inclusterprefixpool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-ipam-cluster-x-k8s-io-v1alpha2-globalinclusterprefixpool,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=globalinclusterprefixpools,versions=v1alpha2,name=validation.globalinclusterprefixpool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-ipam-cluster-x-k8s-io-v1alpha2-globalinclusterprefixpool,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=ipam.cluster.x-k8s.io,resources=globalinclusterprefixpools,versions=v1alpha2,name=default.globalinclusterprefixpool.ipam.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

const (
	inClusterPrefixPoolKind       = "InClusterPrefixPool"
	globalInClusterPrefixPoolKind = "GlobalInClusterPrefixPool"

	errNonAllocatableAllocations = "existing allocations become non-allocatable"

	warnPrefixLength     = "spec.allocationPrefixLength=128 behaves like host-address allocation; consider InClusterIPPool for single-address semantics"
	warnGatewayCollision = "spec.gateway is within spec.prefixes and is not reserved; with /128 allocationPrefixLength it may also be allocated to claims"
)

var (
	_ webhook.CustomDefaulter = &InClusterPrefixPool{}
	_ webhook.CustomValidator = &InClusterPrefixPool{}
)

// InClusterPrefixPool implements a validating and defaulting webhook for InClusterPrefixPool and GlobalInClusterPrefixPool.
type InClusterPrefixPool struct {
	Client client.Reader
}

func (webhook *InClusterPrefixPool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha2.InClusterPrefixPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
	if err != nil {
		return err
	}
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha2.GlobalInClusterPrefixPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

// Default satisfies the defaulting webhook interface.
func (webhook *InClusterPrefixPool) Default(_ context.Context, obj runtime.Object) error {
	pool, ok := obj.(types.GenericInClusterPrefixPool)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterPrefixPool or GlobalInClusterPrefixPool but got a %T", obj))
	}
	spec := pool.PoolSpec()
	spec.AllocationPrefixLength = poolutil.EffectiveAllocationPrefixLength(spec.AllocationPrefixLength)
	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterPrefixPool) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	pool, ok := obj.(types.GenericInClusterPrefixPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterPrefixPool or GlobalInClusterPrefixPool but got a %T", obj))
	}
	if err := webhook.validate(pool); err != nil {
		return nil, err
	}
	return prefixPoolWarnings(pool.PoolSpec()), nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterPrefixPool) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newPool, ok := newObj.(types.GenericInClusterPrefixPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterPrefixPool or GlobalInClusterPrefixPool but got a %T", newObj))
	}
	oldPool, ok := oldObj.(types.GenericInClusterPrefixPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterPrefixPool or GlobalInClusterPrefixPool but got a %T", oldObj))
	}

	if err := webhook.validate(newPool); err != nil {
		return nil, err
	}

	oldSpec := oldPool.PoolSpec()
	newSpec := newPool.PoolSpec()
	oldAllocLen := poolutil.EffectiveAllocationPrefixLength(oldSpec.AllocationPrefixLength)
	newAllocLen := poolutil.EffectiveAllocationPrefixLength(newSpec.AllocationPrefixLength)
	if oldAllocLen != newAllocLen {
		return nil, apierrors.NewBadRequest("spec.allocationPrefixLength is immutable")
	}

	oldPoolRef := ipamv1.IPPoolReference{
		APIGroup: v1alpha2.GroupVersion.Group,
		Kind:     prefixPoolKind(oldPool),
		Name:     oldPool.GetName(),
	}
	inUseAddresses, err := poolutil.ListAddressesInUse(ctx, webhook.Client, oldPool.GetNamespace(), oldPoolRef)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	newConfig := poolutil.NewPrefixPoolConfig(newPool.PoolSpec())
	outOfRangePrefixes := []string{}
	for _, address := range inUseAddresses {
		prefix, err := poolutil.PrefixFromIPAddress(address)
		if err != nil || !poolutil.PrefixIsAllocatable(prefix, newConfig) {
			outOfRangePrefixes = append(outOfRangePrefixes, address.Spec.Address)
		}
	}

	if len(outOfRangePrefixes) > 0 {
		sort.Strings(outOfRangePrefixes)
		return nil, apierrors.NewBadRequest(fmt.Sprintf("%s: %v", errNonAllocatableAllocations, outOfRangePrefixes))
	}

	return prefixPoolWarnings(newSpec), nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *InClusterPrefixPool) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	pool, ok := obj.(types.GenericInClusterPrefixPool)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected an InClusterPrefixPool or GlobalInClusterPrefixPool but got a %T", obj))
	}

	if _, ok := pool.GetAnnotations()[SkipValidateDeleteWebhookAnnotation]; ok {
		return nil, nil
	}

	poolTypeRef := ipamv1.IPPoolReference{
		APIGroup: v1alpha2.GroupVersion.Group,
		Kind:     prefixPoolKind(pool),
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

func (webhook *InClusterPrefixPool) validate(pool types.GenericInClusterPrefixPool) (reterr error) {
	var allErrs field.ErrorList
	defer func() {
		if len(allErrs) > 0 {
			reterr = apierrors.NewInvalid(v1alpha2.GroupVersion.WithKind(prefixPoolKind(pool)).GroupKind(), pool.GetName(), allErrs)
		}
	}()

	spec := pool.PoolSpec()
	if len(spec.Prefixes) == 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefixes"), spec.Prefixes, "prefixes is required"))
		return nil
	}

	allocLen := poolutil.EffectiveAllocationPrefixLength(spec.AllocationPrefixLength)

	if allocLen < 1 || allocLen > 128 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "allocationPrefixLength"), allocLen, "allocationPrefixLength must be between 1 and 128"))
		return nil
	}

	for _, address := range spec.Prefixes {
		prefix, err := netip.ParsePrefix(address)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefixes"), address, "provided entry is not a valid IPv6 CIDR"))
			continue
		}
		if prefix.Addr().Is4() {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefixes"), address, "only IPv6 prefixes are supported"))
			continue
		}
		if prefix != prefix.Masked() {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefixes"), address, "CIDR must be network-aligned"))
		}
		if prefix.Bits() > allocLen {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "prefixes"), address,
				fmt.Sprintf("CIDR prefix length must be <= allocationPrefixLength (%d)", allocLen)))
		}
	}

	if spec.Gateway != "" {
		gateway, err := netip.ParseAddr(spec.Gateway)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "gateway"), spec.Gateway, err.Error()))
		} else if !gateway.Is6() {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "gateway"), spec.Gateway, "provided gateway must be IPv6"))
		}
	}

	for _, excluded := range spec.ExcludedPrefixes {
		ipSet, err := poolutil.AddressToIPSet(excluded)
		if err != nil {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "excludedPrefixes"), excluded, "provided entry is not a valid IP, range, nor CIDR"))
			continue
		}
		from := ipSet.Ranges()[0].From()
		if from.Is4() {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "excludedPrefixes"), excluded, "only IPv6 excluded prefixes are supported"))
		}
	}

	return nil
}

func prefixPoolKind(pool types.GenericInClusterPrefixPool) string {
	switch pool.(type) {
	case *v1alpha2.InClusterPrefixPool:
		return inClusterPrefixPoolKind
	case *v1alpha2.GlobalInClusterPrefixPool:
		return globalInClusterPrefixPoolKind
	default:
		return pool.GetObjectKind().GroupVersionKind().Kind
	}
}

// prefixPoolWarnings returns advisory warnings for valid but potentially
// surprising /128 configurations where prefix claims behave like host
// allocations and gateway overlap with claims is possible.
func prefixPoolWarnings(spec *v1alpha2.InClusterPrefixPoolSpec) admission.Warnings {
	if spec == nil {
		return nil
	}

	allocLen := poolutil.EffectiveAllocationPrefixLength(spec.AllocationPrefixLength)
	if allocLen != 128 {
		return nil
	}

	warnings := make(admission.Warnings, 0, 2)
	warnings = append(warnings, warnPrefixLength)
	if spec.Gateway == "" {
		return warnings
	}

	gateway, err := netip.ParseAddr(spec.Gateway)
	if err != nil || !gateway.Is6() {
		return warnings
	}

	poolIPSet, err := poolutil.PrefixesToIPSet(spec.Prefixes)
	if err != nil || !poolIPSet.Contains(gateway) {
		return warnings
	}

	return append(warnings, warnGatewayCollision)
}
