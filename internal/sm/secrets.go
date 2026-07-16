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

package sm

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
)

// ValidateK8sSecretKeyName validates that a secret key name conforms to Kubernetes requirements.
// Kubernetes secret data keys must match the regex [-._a-zA-Z0-9]+
// See: https://kubernetes.io/docs/concepts/configuration/secret/#restriction-names-data
func ValidateK8sSecretKeyName(key string) error {
	if key == "" {
		return fmt.Errorf("secret key cannot be empty")
	}

	for i, char := range key {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' ||
			char == '_' ||
			char == '.') {
			return fmt.Errorf("secret key '%s' contains invalid character '%c' at position %d (Kubernetes requires alphanumeric, '-', '_', or '.')", key, char, i)
		}
	}

	return nil
}

// ValidateSecretKeyName validates that a secret key name is POSIX-compliant.
// POSIX-compliant names are recommended for maximum compatibility with environment variables:
// - Must start with a letter (a-z, A-Z) or underscore (_)
// - Can only contain letters, digits (0-9), and underscores
// - Cannot be empty
func ValidateSecretKeyName(key string) error {
	if key == "" {
		return fmt.Errorf("secret key cannot be empty")
	}

	// Check first character
	firstChar := key[0]
	if !((firstChar >= 'a' && firstChar <= 'z') ||
		(firstChar >= 'A' && firstChar <= 'Z') ||
		firstChar == '_') {
		return fmt.Errorf("secret key '%s' must start with a letter or underscore", key)
	}

	// Check remaining characters
	for i, char := range key {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_') {
			return fmt.Errorf("secret key '%s' contains invalid character '%c' at position %d (only alphanumeric and underscore allowed)", key, char, i)
		}
	}

	return nil
}

// PullSecretManagerSecretDeltas will determine if any secrets have been updated and return all secrets assigned to the machine account if so.
// First returned value is a boolean stating if something changed or not.
// The second returned value is a mapping of secret IDs (or names if useSecretNames is true) and their values from Secrets Manager
func PullSecretManagerSecretDeltas(factory BitwardenClientFactory, statePath string, logger logr.Logger, orgId string, authToken string, lastSync time.Time, useSecretNames bool, projectId string) (bool, map[string][]byte, error) {
	bitwardenClient, err := factory.GetBitwardenClient()
	if err != nil {
		logger.Error(err, "Failed to create client")
		return false, nil, err
	}

	err = bitwardenClient.AccessTokenLogin(authToken, &statePath)
	if err != nil {
		logger.Error(err, "Failed to authenticate")
		return false, nil, err
	}

	secrets := map[string][]byte{}

	smSecretResponse, err := bitwardenClient.Secrets().Sync(orgId, &lastSync)

	if err != nil {
		logger.Error(err, "Failed to get secrets since last sync.")
		return false, nil, err
	}

	if smSecretResponse == nil {
		logger.Info("No secret response from Bitwarden")
		return false, nil, nil
	}

	smSecretVals := smSecretResponse.Secrets

	// Filter by project ID if specified
	if projectId != "" {
		filtered := smSecretVals[:0]
		for _, s := range smSecretVals {
			if s.ProjectID != nil && *s.ProjectID == projectId {
				filtered = append(filtered, s)
			}
		}
		smSecretVals = filtered
	}

	// Use UUIDs as keys
	if !useSecretNames {
		for _, smSecretVal := range smSecretVals {
			secrets[smSecretVal.ID] = []byte(smSecretVal.Value)
		}
		defer bitwardenClient.Close()
		return smSecretResponse.HasChanges, secrets, nil
	}

	// Use secret names with validation and duplicate detection
	seenKeys := make(map[string][]string) // Track duplicates: key -> []secretIDs
	var k8sInvalidKeys []string

	// First pass: validate K8s compliance (error), POSIX compliance (warn), and detect duplicates (error)
	for _, smSecretVal := range smSecretVals {
		secretKey := smSecretVal.Key

		// Validate Kubernetes compliance
		if err := ValidateK8sSecretKeyName(secretKey); err != nil {
			k8sInvalidKeys = append(k8sInvalidKeys,
				fmt.Sprintf("'%s' (ID: %s): %s", secretKey, smSecretVal.ID, err.Error()))
		} else {
			// Only check POSIX compliance if K8s validation passed
			// Validate POSIX compliance - this is a soft warning for env var compatibility
			if err := ValidateSecretKeyName(secretKey); err != nil {
				logger.Info("Secret name is not POSIX-compliant and may not work as an environment variable",
					"secretId", smSecretVal.ID,
					"secretKey", secretKey,
					"warning", err.Error())
			}
		}

		// Track for duplicate detection
		seenKeys[secretKey] = append(seenKeys[secretKey], smSecretVal.ID)
	}

	// Fail if any keys are invalid for Kubernetes
	if len(k8sInvalidKeys) > 0 {
		errMsg := "Secret key names invalid for Kubernetes:\n"
		for _, key := range k8sInvalidKeys {
			errMsg += fmt.Sprintf("  - %s\n", key)
		}
		errMsg += "\nKubernetes secret data keys must consist of alphanumeric characters, '-', '_', or '.'"

		defer bitwardenClient.Close()
		return false, nil, errors.New(errMsg)
	}

	// Check for duplicates
	var duplicates []string
	for key, ids := range seenKeys {
		if len(ids) > 1 {
			duplicates = append(duplicates,
				fmt.Sprintf("'%s' (IDs: %v)", key, ids))
		}
	}

	// Fail if duplicates found
	if len(duplicates) > 0 {
		errMsg := "Duplicate secret key names detected:\n"
		for _, dup := range duplicates {
			errMsg += fmt.Sprintf("  - %s\n", dup)
		}
		errMsg += "\nMultiple secrets with the same name. Use unique names for secrets or disable useSecretNames."

		defer bitwardenClient.Close()
		return false, nil, errors.New(errMsg)
	}

	// Second pass: build the secrets map using names
	for _, smSecretVal := range smSecretVals {
		secrets[smSecretVal.Key] = []byte(smSecretVal.Value)
	}

	defer bitwardenClient.Close()
	return smSecretResponse.HasChanges, secrets, nil
}

// ApplySecretMap applies the fetched Secrets Manager secrets onto a K8s Secret's data,
// honoring the BitwardenSecret's mapping and filtering configuration.
func ApplySecretMap(secrets map[string][]byte, bwSecret *operatorsv1.BitwardenSecret, k8sSecret *corev1.Secret) {
	k8sSecret.Data = make(map[string][]byte)

	//If we are doing a straight up synch with no map, dump them across and return
	//useSecretNames implies onlyMappedSecrets=false when no SecretMap is provided
	if (!bwSecret.Spec.OnlyMappedSecrets || bwSecret.Spec.UseSecretNames) && len(bwSecret.Spec.SecretMap) == 0 {
		k8sSecret.Data = secrets
		return
	}

	for key, secret := range secrets {
		mapping, isThere := FindSecretMapByBwSecretId(&bwSecret.Spec, key) //see if this particular secret is in the map
		if bwSecret.Spec.OnlyMappedSecrets && !bwSecret.Spec.UseSecretNames && !isThere {
			continue //Not in map and we're only synching mapped secrets (without useSecretNames), so move on.
		}

		targetKey := key //defaulting to BwSecretId
		if isThere {
			targetKey = mapping.SecretKeyName //Found in map, so set the target key to the alias
		}

		k8sSecret.Data[targetKey] = secret
	}
}

// FindSecretMapByBwSecretId returns the SecretMap entry with the specified BwSecretId, if found.
func FindSecretMapByBwSecretId(spec *operatorsv1.BitwardenSecretSpec, bwSecretId string) (operatorsv1.SecretMap, bool) {
	if spec.SecretMap == nil {
		return operatorsv1.SecretMap{}, false
	}

	for _, sm := range spec.SecretMap {
		if sm.BwSecretId == bwSecretId {
			return sm, true
		}
	}

	return operatorsv1.SecretMap{}, false
}
