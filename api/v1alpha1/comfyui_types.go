/*
Copyright 2026.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComfyUISpec defines the desired state of ComfyUI.
type ComfyUISpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Image        string                       `json:"image"`
	Replicas     *int32                       `json:"replicas,omitempty"`
	Resources    *corev1.ResourceRequirements `json:"resources,omitempty"`
	Storage      *ComfyUIStorage              `json:"storage,omitempty"`
	VolumeMounts []corev1.VolumeMount         `json:"volumeMounts,omitempty"`
	Volumes      []corev1.Volume              `json:"volumes,omitempty"`
	// ServiceType defines the Kubernetes Service type (ClusterIP, NodePort, LoadBalancer)
	// +kubebuilder:default=ClusterIP
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`
	// EnableGPU enables GPU support and adds nvidia.com/gpu resource limits
	// +optional
	EnableGPU bool `json:"enableGPU,omitempty"`
	// GPUCount specifies the number of GPUs to request (default: 1 if EnableGPU is true)
	// +optional
	// +kubebuilder:validation:Minimum=1
	GPUCount *int `json:"gpuCount,omitempty"`
	// NodeSelector constrains pods to nodes with specific labels
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Gateway configuration for Kubernetes Gateway API HTTPRoute
	// +optional
	Gateway *GatewayConfig `json:"gateway,omitempty"`
	// OAuth2 configuration for FileBrowser authentication
	// +optional
	OAuth2 *OAuth2Config `json:"oauth2,omitempty"`
	// FSGroup sets the pod's fsGroup security context for volume permissions.
	// If not specified, auto-detected from the namespace on OpenShift or defaults to 1000.
	// +optional
	FSGroup *int64 `json:"fsGroup,omitempty"`
}

type ComfyUIStorage struct {
	Size             string  `json:"size,omitempty"`
	StorageClassName *string `json:"storageClassName,omitempty"`
	// +kubebuilder:default=ReadWriteOnce
	AccessMode corev1.PersistentVolumeAccessMode `json:"accessMode,omitempty"`
}

// GatewayConfig defines configuration for Kubernetes Gateway API integration
type GatewayConfig struct {
	// Name of the Gateway to attach HTTPRoute to
	// +kubebuilder:default="default-gateway"
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace where the Gateway is located
	// +kubebuilder:default="gateway-system"
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Hostname for the HTTPRoute (e.g., "comfyui.example.com")
	// If not specified, defaults to {comfyui-name}.example.com
	// +optional
	Hostname string `json:"hostname,omitempty"`
}

// OAuth2Config defines OAuth2 authentication configuration for FileBrowser
type OAuth2Config struct {
	// Provider specifies the OAuth2 provider (github, google, or oidc)
	// +kubebuilder:validation:Enum=github;google;oidc
	// +kubebuilder:validation:Required
	Provider string `json:"provider"`

	// ClientID is the OAuth2 client ID
	// +kubebuilder:validation:Required
	ClientID string `json:"clientID"`

	// ClientSecretRef references a Secret containing the OAuth2 client secret
	// The secret must contain a key named "client-secret"
	// +kubebuilder:validation:Required
	ClientSecretRef corev1.SecretKeySelector `json:"clientSecretRef"`

	// IssuerURL is the OIDC issuer URL (required only for oidc provider)
	// +optional
	IssuerURL string `json:"issuerURL,omitempty"`

	// AllowedEmails is a list of email addresses allowed to authenticate
	// If empty, any authenticated user from the provider is allowed
	// +optional
	AllowedEmails []string `json:"allowedEmails,omitempty"`

	// AllowedDomains is a list of email domains allowed to authenticate (e.g., "example.com")
	// If empty, any authenticated user from the provider is allowed
	// +optional
	AllowedDomains []string `json:"allowedDomains,omitempty"`

	// CookieSecret is an optional reference to a secret containing the cookie secret
	// If not specified, a random secret will be generated
	// The secret must contain a key named "cookie-secret"
	// +optional
	CookieSecretRef *corev1.SecretKeySelector `json:"cookieSecretRef,omitempty"`
}

// ComfyUIStatus defines the observed state of ComfyUI.
// Currently unused but required for Kubernetes status subresource.
type ComfyUIStatus struct {
	// Reserved for future use
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="GPU",type=boolean,JSONPath=`.spec.enableGPU`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:scope=Namespaced

// ComfyUI is the Schema for the comfyuis API.
type ComfyUI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComfyUISpec   `json:"spec,omitempty"`
	Status ComfyUIStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComfyUIList contains a list of ComfyUI.
type ComfyUIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComfyUI `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComfyUI{}, &ComfyUIList{})
}
