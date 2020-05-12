// Code generated by go generate; DO NOT EDIT.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Get is the Schema for the gets API
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=gets,scope=Namespaced
// +genclient
type Get struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	CommandSpec   `json:"spec,omitempty"`
	CommandStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GetList contains a list of Get
type GetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Get `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Get{}, &GetList{})
}
