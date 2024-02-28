/*
Copyright 2024 Bitwarden, Inc..

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

package controller

import (
	"context"
	"fmt"
	"time"

	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sdk "github.com/bitwarden/sdk/languages/go"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
)

// BitwardenSecretReconciler reconciles a BitwardenSecret object
type BitwardenSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=k8s.bitwarden.com,resources=bitwardensecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.bitwarden.com,resources=bitwardensecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.bitwarden.com,resources=bitwardensecrets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BitwardenSecret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *BitwardenSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	message := fmt.Sprintf("Syncing  %s/%s", req.Namespace, req.Name)

	logger.Info(message)

	bwApiUrl, identApiUrl, statePath, refreshIntervalSeconds := GetSettings()

	ns := req.Namespace
	bwSecret := &operatorsv1.BitwardenSecret{}

	err := r.Get(ctx, req.NamespacedName, bwSecret)

	//Deleted Bitwarden Secret event.
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		r.LogError(logger, ctx, bwSecret, err, "Error looking up BitwardenSecret")
		//Other lookup error
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	authSecret := &corev1.Secret{}
	namespacedAuthSecret := types.NamespacedName{
		Name:      bwSecret.Spec.AuthToken.SecretName,
		Namespace: ns,
	}

	secret := &corev1.Secret{}
	namespacedSecret := types.NamespacedName{
		Name:      bwSecret.Spec.SecretName,
		Namespace: ns,
	}

	err = r.Client.Get(ctx, namespacedAuthSecret, authSecret)

	if err != nil {
		r.LogError(logger, ctx, bwSecret, err, "Error pulling authorization token secret")
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	authToken := string(authSecret.Data[bwSecret.Spec.AuthToken.SecretKey])
	orgId := bwSecret.Spec.OrganizationId

	// Delete deltas will be handled in the future
	secrets, deletes, err := PullSecretManagerSecretDeltas(logger, bwApiUrl, identApiUrl, statePath, orgId, authToken)

	if err != nil {
		r.LogError(logger, ctx, bwSecret, err, "Error pulling Secret Manager secrets from API")
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	err = r.Get(ctx, namespacedSecret, secret)

	//Creating new
	if err != nil && errors.IsNotFound(err) {
		secret = bwSecret.CreateK8sSecret()

		// Cascading delete
		if err := ctrl.SetControllerReference(bwSecret, secret, r.Scheme); err != nil {
			r.LogError(logger, ctx, bwSecret, err, "Failed to set controller reference")
			return ctrl.Result{
				RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
			}, err
		}

		err := r.Create(ctx, secret)
		if err != nil {
			r.LogError(logger, ctx, bwSecret, err, "Creation of K8s secret failed.")
			return ctrl.Result{
				RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
			}, err
		}

	}

	RevertMap(secret)

	//######################################
	// Temp need until we get delete deltas
	// Just rebuilding for now
	for k := range secret.Data {
		deletes = append(deletes, k)
	}
	//######################################

	RemoveDeletedSecrets(secret, deletes)

	UpdateAddSecretValues(secret, secrets)

	bwSecret.ApplySecretMap(secret)

	err = bwSecret.SetK8sSecretAnnotations(secret)
	if err != nil {
		r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Error setting annotations for  %s/%s", req.Namespace, req.Name))
	}

	err = r.Update(ctx, secret)
	if err != nil {
		r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Failed to update  %s/%s", req.Namespace, req.Name))
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	r.LogCompletion(logger, ctx, bwSecret, fmt.Sprintf("Completed sync for %s/%s", req.Namespace, req.Name))
	return ctrl.Result{
		RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BitwardenSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.BitwardenSecret{}).
		Complete(r)
}

func (r *BitwardenSecretReconciler) LogError(logger logr.Logger, ctx context.Context, bwSecret *operatorsv1.BitwardenSecret, err error, message string) {
	logger.Error(err, message)

	if bwSecret != nil {
		errorCondition := metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  "ReconciliationFailed",
			Message: fmt.Sprintf("%s - %s", message, err.Error()),
			Type:    "FailedSync",
		}

		apimeta.SetStatusCondition(&bwSecret.Status.Conditions, errorCondition)
		r.Status().Update(ctx, bwSecret)
	}

}

func (r *BitwardenSecretReconciler) LogCompletion(logger logr.Logger, ctx context.Context, bwSecret *operatorsv1.BitwardenSecret, message string) {
	logger.Info(message)

	if bwSecret != nil {
		completeCondition := metav1.Condition{
			Status:  metav1.ConditionTrue,
			Reason:  "ReconciliationComplete",
			Message: message,
			Type:    "SuccessfulSync",
		}

		bwSecret.Status.LastSuccessfulSyncTime = metav1.Time{Time: time.Now().UTC()}

		apimeta.SetStatusCondition(&bwSecret.Status.Conditions, completeCondition)
		r.Status().Update(ctx, bwSecret)
	}
}

// This is currently pulling all secrets for a complete refresh.  In the future
// we will have a delta plull method to only pull what has changed
// First returned value is the Adds/Updates.  The second returned value is the array of removed IDs.  As the delta call doesn't exist, this is
// included for future use
func PullSecretManagerSecretDeltas(logger logr.Logger, bwApiUrl string, identApiUrl string, statePath string, orgId string, authToken string) (map[string][]byte, []string, error) {
	bitwardenClient, err := sdk.NewBitwardenClient(&bwApiUrl, &identApiUrl)
	if err != nil {
		logger.Error(err, "Faled to start client")
		return nil, nil, err
	}

	err = bitwardenClient.AccessTokenLogin(authToken, &statePath)
	if err != nil {
		logger.Error(err, "Faled to authenticate")
		return nil, nil, err
	}

	secrets := map[string][]byte{}
	secretIds := []string{}

	// Use a deltas endpoint in the future
	smSecrets, err := bitwardenClient.Secrets.List(orgId)

	if err != nil {
		logger.Error(err, "Faled to list secrets")
		return nil, nil, err
	}

	for _, smSecret := range smSecrets.Data {
		secretIds = append(secretIds, smSecret.ID)
	}

	// Use a deltas endpoint in the future
	smSecretVals, err := bitwardenClient.Secrets.GetByIDS(secretIds)

	if err != nil {
		logger.Error(err, "Faled to get secrets by id")
		return nil, nil, err
	}

	for _, smSecretVal := range smSecretVals.Data {
		secrets[smSecretVal.ID] = []byte(smSecretVal.Value)
	}

	defer bitwardenClient.Close()

	return secrets, nil, nil
}

func RevertMap(secret *corev1.Secret) error {
	if val, found := secret.ObjectMeta.Annotations["k8s.bitwarden.com/custom-map"]; found {
		var array []operatorsv1.SecretMap
		err := json.Unmarshal([]byte(val), &array)

		if err != nil {
			return err
		}

		for _, mappedSecret := range array {
			secret.Data[mappedSecret.BwSecretId] = secret.Data[mappedSecret.SecretKeyName]
			delete(secret.Data, mappedSecret.SecretKeyName)
		}
	}

	return nil
}

func RemoveDeletedSecrets(secret *corev1.Secret, secretKeys []string) {
	for _, k := range secretKeys {
		delete(secret.Data, k)
	}
}

func UpdateAddSecretValues(secret *corev1.Secret, secrets map[string][]byte) {
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	for k, v := range secrets {
		secret.Data[k] = v
	}
}

func GetSettings() (string, string, string, int) {
	bwApiUrl := strings.TrimSpace(os.Getenv("BW_API_URL"))
	identApiUrl := strings.TrimSpace(os.Getenv("BW_IDENTITY_API_URL"))
	statePath := strings.TrimSpace(os.Getenv("BW_SECRETS_MANAGER_STATE_PATH"))

	refreshIntervalSeconds := 300

	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL"))); err == nil {
		if value >= 180 {
			refreshIntervalSeconds = value
		}
	}

	if bwApiUrl == "" {
		bwApiUrl = "https://api.bitwarden.com"
	}

	if identApiUrl == "" {
		identApiUrl = "https://identity.bitwarden.com"
	}

	if statePath == "" {
		statePath = "/var/bitwarden/state"
	}

	return bwApiUrl, identApiUrl, statePath, refreshIntervalSeconds
}
