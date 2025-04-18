package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackendSpec defines the backend configuration for the workspace
type BackendSpec struct {
	// Type is the type of the backend
	// +kubebuilder:validation:Enum=local;remote;s3;gcs;azurerm;oss;consul;cos;http;pg;kubernetes
	Type string `json:"type"`
	// Inputs are the inputs to the terraform module.
	Inputs *apiextensionsv1.JSON `json:"inputs,omitempty"`
}

// ModuleSpec defines the desired state of Module.
type ModuleSpec struct {
	// Name is the name of the terraform module.
	// Example:
	// name: "my-module"
	// source:  "terraform-aws-modules/vpc/aws"
	// version: "5.19.0"
	Name string `json:"name"`
	// Source is the source of the terraform module.
	Source string `json:"source"`
	// Version is the version of the terraform module.
	Version string `json:"version,omitempty"`

	// Inputs are the inputs to the terraform module.
	Inputs *apiextensionsv1.JSON `json:"inputs,omitempty"`
	// Outputs are the outputs of the terraform module.
	Outputs []ModuleOutput `json:"outputs,omitempty"`
}

// WorkspaceSpec defines the desired state of Workspace.
type WorkspaceSpec struct {
	// TerraformVersion is the version of terraform to use
	// +kubebuilder:validation:Required
	TerraformVersion string `json:"terraformVersion"`

	// Backend is the backend configuration for the workspace
	// +kubebuilder:validation:Required
	Backend BackendSpec `json:"backend"`

	// ProviderSpecs is a list of provider specifications
	// +kubebuilder:validation:Required
	ProviderSpecs []ProviderSpec `json:"providerSpecs"`

	// Module is the module configuration for the workspace
	// +kubebuilder:validation:Required
	Module *ModuleSpec `json:"module"`
}

// WorkspaceStatus defines the observed state of Workspace.
type WorkspaceStatus struct {
	// LatestPlan is the latest plan of the workspace
	LatestPlan string `json:"latestPlan"`
	// CurrentRender is the current render of the workspace
	CurrentRender string `json:"currentRender"`
	// NextRefreshTimestamp is the next time the workspace will be refreshed
	// +kubebuilder:validation:Optional
	NextRefreshTimestamp metav1.Time `json:"nextRefreshTimestamp"`
	// ObservedGeneration is the observed generation of the workspace
	ObservedGeneration int64 `json:"observedGeneration"`
}

// Workspace is the Schema for the workspaces API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

// WorkspaceList contains a list of Workspace.
// +kubebuilder:object:root=true
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	//SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
