package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InClusterIPPoolSpec defines the desired state of InClusterIPPool
type InClusterIPPoolSpec struct {
	// Subnet is the subnet to assign IP addresses from.
	// Can be omitted if first, last and prefix are set.
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

// InClusterIPPoolStatus defines the observed state of InClusterIPPool
type InClusterIPPoolStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// InClusterIPPool is the Schema for the inclusterippools API
type InClusterIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InClusterIPPoolSpec   `json:"spec,omitempty"`
	Status InClusterIPPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InClusterIPPoolList contains a list of InClusterIPPool
type InClusterIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InClusterIPPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InClusterIPPool{}, &InClusterIPPoolList{})
}
