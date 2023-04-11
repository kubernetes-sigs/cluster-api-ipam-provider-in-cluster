package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InClusterIPPoolSpec defines the desired state of InClusterIPPool.
type InClusterIPPoolSpec struct {
	// Addresses is a list of IP addresses that can be assigned. This set of
	// addresses can be non-contiguous. Can be omitted if subnet, or first and
	// last is set.
	// +optional
	Addresses []string `json:"addresses,omitempty"`

	// Subnet is the subnet to assign IP addresses from.
	// Can be omitted if addresses or first, last and prefix are set.
	// +optional
	Subnet string `json:"subnet,omitempty"`

	// First is the first address that can be assigned.
	// If unset, the second address of subnet will be used.
	// +optional
	First string `json:"start,omitempty"`

	// Last is the last address that can be assigned.
	// Must come after first and needs to fit into a common subnet.
	// If unset, the second last address of subnet will be used.
	// +optional
	Last string `json:"end,omitempty"`

	// Prefix is the network prefix to use.
	// If unset the prefix from the subnet will be used.
	// +optional
	// +kubebuilder:validation:Maximum=128
	Prefix int `json:"prefix,omitempty"`

	// Gateway
	// +optional
	Gateway string `json:"gateway,omitempty"`
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
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Subnet",type="string",JSONPath=".spec.subnet",description="Subnet to allocate IPs from"
// +kubebuilder:printcolumn:name="First",type="string",JSONPath=".spec.first",description="First address of the range to allocate from"
// +kubebuilder:printcolumn:name="Last",type="string",JSONPath=".spec.last",description="Last address of the range to allocate from"
// +kubebuilder:printcolumn:name="Addresses",type="string",JSONPath=".spec.addresses",description="List of addresses, within the subnet, to allocate from"
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
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Subnet",type="string",JSONPath=".spec.subnet",description="Subnet to allocate IPs from"
// +kubebuilder:printcolumn:name="First",type="string",JSONPath=".spec.first",description="First address of the range to allocate from"
// +kubebuilder:printcolumn:name="Last",type="string",JSONPath=".spec.last",description="Last address of the range to allocate from"
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
