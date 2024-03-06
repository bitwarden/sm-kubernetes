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
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
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
}

type AuthToken struct {
	// The name of the secret where the authorization token is stored
	// +kubebuilder:Required
	SecretName string `json:"secretName"`
	// The key of the secret where the authorization token is stored
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

func (bwSecret *BitwardenSecret) CreateK8sSecret() *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        bwSecret.Spec.SecretName,
			Namespace:   bwSecret.Namespace,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}
	secret.ObjectMeta.Labels["k8s.bitwarden.com/bw-secret"] = string(bwSecret.UID)
	return secret
}

func (bwSecret *BitwardenSecret) ApplySecretMap(secret *corev1.Secret) {
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	if bwSecret.Spec.SecretMap != nil {
		for _, mappedSecret := range bwSecret.Spec.SecretMap {
			secret.Data[mappedSecret.SecretKeyName] = secret.Data[mappedSecret.BwSecretId]
			delete(secret.Data, mappedSecret.BwSecretId)
		}
	}
}

func (bwSecret *BitwardenSecret) SetK8sSecretAnnotations(secret *corev1.Secret) error {

	if secret.ObjectMeta.Annotations == nil {
		secret.ObjectMeta.Annotations = map[string]string{}
	}

	secret.ObjectMeta.Annotations["k8s.bitwarden.com/sync-time"] = fmt.Sprint(time.Now().UTC())

	if bwSecret.Spec.SecretMap == nil {
		delete(secret.ObjectMeta.Annotations, "k8s.bitwarden.com/custom-map")
	} else {
		bytes, err := json.MarshalIndent(bwSecret.Spec.SecretMap, "", "  ")
		if err != nil {
			return err
		}
		secret.ObjectMeta.Annotations["k8s.bitwarden.com/custom-map"] = string(bytes)
	}

	return nil
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
