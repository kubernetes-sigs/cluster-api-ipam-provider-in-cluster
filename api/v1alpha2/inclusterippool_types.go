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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InClusterIPPoolSpec defines the desired state of InClusterIPPool.
type InClusterIPPoolSpec struct {
	// Addresses is a list of IP addresses that can be assigned. This set of
	// addresses can be non-contiguous.
	Addresses []string `json:"addresses"`

	// Prefix is the network prefix to use.
	// +kubebuilder:validation:Maximum=128
	Prefix int `json:"prefix"`

	// Gateway
	// +optional
	Gateway string `json:"gateway,omitempty"`

	// AllocateReservedIPAddresses causes the provider to allocate the network
	// address (the first address in the inferred subnet) and broadcast address
	// (the last address in the inferred subnet) when IPv4. The provider will
	// allocate the anycast address address (the first address in the inferred
	// subnet) when IPv6.
	// +optional
	AllocateReservedIPAddresses bool `json:"allocateReservedIPAddresses,omitempty"`

	// ExcludedAddresses contains a list of IP addresses, which will be excluded from
	// the set of assignable IP addresses.
	ExcludedAddresses []string `json:"excludedAddresses,omitempty"`
}

// InClusterIPPoolStatus defines the observed state of InClusterIPPool.
type InClusterIPPoolStatus struct {
	// Addresses reports the count of total, free, and used IPs in the pool.
	// +optional
	Addresses *InClusterIPPoolStatusIPAddresses `json:"ipAddresses,omitempty"`
}

// InClusterIPPoolStatusIPAddresses contains the count of total, free, and used IPs in a pool.
type InClusterIPPoolStatusIPAddresses struct {
	// Total is the total number of IPs configured for the pool.
	// Counts greater than int can contain will report as math.MaxInt.
	Total int `json:"total"`

	// Free is the count of unallocated IPs in the pool.
	// Counts greater than int can contain will report as math.MaxInt.
	Free int `json:"free"`

	// Used is the count of allocated IPs in the pool.
	// Counts greater than int can contain will report as math.MaxInt.
	Used int `json:"used"`

	// Out of Range is the count of allocated IPs in the pool that is not
	// contained within spec.Addresses.
	// Counts greater than int can contain will report as math.MaxInt.
	OutOfRange int `json:"outOfRange"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Addresses",type="string",JSONPath=".spec.addresses",description="List of addresses, to allocate from"
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.ipAddresses.total",description="Count of IPs configured for the pool"
// +kubebuilder:printcolumn:name="Free",type="integer",JSONPath=".status.ipAddresses.free",description="Count of unallocated IPs in the pool"
// +kubebuilder:printcolumn:name="Used",type="integer",JSONPath=".status.ipAddresses.used",description="Count of allocated IPs in the pool"

// InClusterIPPool is the Schema for the inclusterippools API.
type InClusterIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InClusterIPPoolSpec   `json:"spec,omitempty"`
	Status InClusterIPPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InClusterIPPoolList contains a list of InClusterIPPool.
type InClusterIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InClusterIPPool `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Addresses",type="string",JSONPath=".spec.addresses",description="List of addresses, to allocate from"
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.ipAddresses.total",description="Count of IPs configured for the pool"
// +kubebuilder:printcolumn:name="Free",type="integer",JSONPath=".status.ipAddresses.free",description="Count of unallocated IPs in the pool"
// +kubebuilder:printcolumn:name="Used",type="integer",JSONPath=".status.ipAddresses.used",description="Count of allocated IPs in the pool"

// GlobalInClusterIPPool is the Schema for the global inclusterippools API.
// This pool type is cluster scoped. IPAddressClaims can reference
// pools of this type from any any namespace.
type GlobalInClusterIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InClusterIPPoolSpec   `json:"spec,omitempty"`
	Status InClusterIPPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlobalInClusterIPPoolList contains a list of GlobalInClusterIPPool.
type GlobalInClusterIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalInClusterIPPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&InClusterIPPool{},
		&InClusterIPPoolList{},
		&GlobalInClusterIPPool{},
		&GlobalInClusterIPPoolList{},
	)
}

// PoolSpec implements the genericInClusterPool interface.
func (p *InClusterIPPool) PoolSpec() *InClusterIPPoolSpec {
	return &p.Spec
}

// PoolStatus implements the genericInClusterPool interface.
func (p *InClusterIPPool) PoolStatus() *InClusterIPPoolStatus {
	return &p.Status
}

// PoolSpec implements the genericInClusterPool interface.
func (p *GlobalInClusterIPPool) PoolSpec() *InClusterIPPoolSpec {
	return &p.Spec
}

// PoolStatus implements the genericInClusterPool interface.
func (p *GlobalInClusterIPPool) PoolStatus() *InClusterIPPoolStatus {
	return &p.Status
}
