package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EvictionLog defines a log entry for pod evictions
type Eviction struct {
	PodName      string `json:"podName"`
	EvictionTime string `json:"evictionTime"`
}

// PDBWatcherSpec defines the desired state of PDBWatcher
type PDBWatcherSpec struct {
	PDBName        string   `json:"pdbName"`
	DeploymentName string   `json:"deploymentName"`
	LastEviction   Eviction `json:"lastEviction,omitempty"`
}

// PDBWatcherStatus defines the observed state of PDBWatcher
type PDBWatcherStatus struct {
	LastEviction         Eviction `json:"lastEviction,omitempty"` //this is the last one the controller has processed.
	MinReplicas          int32    `json:"minReplicas"`            // Minimum number of replicas to maintain
	DeploymentGeneration int64    `json:"deploymentGeneration"`   // generation (spec hash) of deployment.
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PDBWatcher is the Schema for the pdbwatchers API
type PDBWatcher struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PDBWatcherSpec   `json:"spec,omitempty"`
	Status PDBWatcherStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PDBWatcherList contains a list of PDBWatcher
type PDBWatcherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PDBWatcher `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PDBWatcher{}, &PDBWatcherList{})
}
