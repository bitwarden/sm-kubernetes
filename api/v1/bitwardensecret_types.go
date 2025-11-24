/*
Source code in this repository is covered by one of two licenses: (i) the
GNU General Public License (GPL) v3.0 (ii) the Bitwarden License v1.0. The
default license throughout the repository is GPL v3.0 unless the header
specifies another license. Bitwarden Licensed code is found only in the
/bitwarden_license directory.

GPL v3.0:
https://github.com/bitwarden/server/blob/main/LICENSE_GPL.txt

Bitwarden License v1.0:
https://github.com/bitwarden/server/blob/main/LICENSE_BITWARDEN.txt

No grant of any rights in the trademarks, service marks, or logos of Bitwarden is
made (except as may be necessary to comply with the notice requirements as
applicable), and use of any Bitwarden trademarks must comply with Bitwarden
Trademark Guidelines
<https://github.com/bitwarden/server/blob/main/TRADEMARK_GUIDELINES.md>.

*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BitwardenSecretSpec defines the desired state of BitwardenSecret
type BitwardenSecretSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The organization ID for your organization
	// +kubebuilder:Optional
	OrganizationId string `json:"organizationId"`
	// The name of the secret for the
	// +kubebuilder:Required
	SecretName string `json:"secretName"`
	// The mapping of organization secret IDs to K8s secret keys.  This helps improve readability and mapping to environment variables.
	// +kubebuilder:Optional
	SecretMap []SecretMap `json:"map,omitempty"`
	// The secret key reference for the authorization token used to connect to Secrets Manager
	// +kubebuilder:Required
	AuthToken AuthToken `json:"authToken"`
	// OnlyMappedSecrets, when true, restricts the Kubernetes Secret to only include secrets specified in SecretMap.
	// When false or unset, all secrets accessible by the machine account are included, with SecretMap applied for renaming.
	// Defaults to true.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	OnlyMappedSecrets bool `json:"onlyMappedSecrets"`
	// UseSecretNames, when true, uses the secret names from Bitwarden Secrets Manager as Kubernetes secret keys.
	// When false or unset (default), uses secret UUIDs as keys (preserving backward compatibility).
	// When enabled, secret names must be POSIX-compliant (start with letter/underscore, contain only alphanumeric/underscore)
	// and must be unique across all accessible secrets. Validation errors will prevent secret synchronization.
	// Defaults to false.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	UseSecretNames bool `json:"useSecretNames,omitempty"`
}

type AuthToken struct {
	// The name of the Kubernetes secret where the authorization token is stored
	// +kubebuilder:Required
	SecretName string `json:"secretName"`
	// The key of the Kubernetes secret where the authorization token is stored
	// +kubebuilder:Required
	SecretKey string `json:"secretKey"`
}

type SecretMap struct {
	// The ID of the secret in Secrets Manager
	// +kubebuilder:Required
	BwSecretId string `json:"bwSecretId"`
	// The name of the mapped key in the created Kubernetes secret
	// +kubebuilder:Required
	SecretKeyName string `json:"secretKeyName"`
}

// BitwardenSecretStatus defines the observed state of BitwardenSecret
type BitwardenSecretStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions store the status conditions of the BitwardenSecret instances
	// +operator-sdk:csv:customresourcedefinitions:type=status
	LastSuccessfulSyncTime metav1.Time `json:"lastSuccessfulSyncTime,omitempty"`

	// Conditions store the status conditions of the BitwardenSecret instances
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BitwardenSecret is the Schema for the bitwardensecrets API
type BitwardenSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BitwardenSecretSpec   `json:"spec,omitempty"`
	Status BitwardenSecretStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BitwardenSecretList contains a list of BitwardenSecret
type BitwardenSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BitwardenSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BitwardenSecret{}, &BitwardenSecretList{})
}
