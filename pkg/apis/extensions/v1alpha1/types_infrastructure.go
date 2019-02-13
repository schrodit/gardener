package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Infrastructure is a specification for cloud provider infrastructure.
type Infrastructure struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InfrastructureSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InfrastructureList is a list of Infrastructure resources
type InfrastructureList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items is the list of Infrastructures.
	Items []Infrastructure `json:"items"`
}

type InfrastructureSpec struct {
	// DefaultSpec is a structure containing common fields used by all extension resources.
	DefaultSpec `json:",inline"`

	ProviderConfig runtime.RawExtension   `json:"providerConfig,omitempty"`
	SecretRef      corev1.SecretReference `json:"secretRef"`
	Region         string                 `json:"region"`
	Zones          []string               `json:"zones,omitempty"`
	SSHPublicKey   []byte                 `json:"sshPublicKey"`
}

type InfrastructureStatus struct {
	DefaultStatus  `json:",inline"`
	ProviderStatus runtime.RawExtension `json:"providerStatus,omitempty"`
}
