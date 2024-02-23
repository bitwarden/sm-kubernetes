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
	"encoding/json"
	"fmt"
	"time"

	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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

	logger.Info("Syncing " + req.Name)

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

	ns := req.Namespace
	bwsecret := &operatorsv1.BitwardenSecret{}
	err := r.Get(ctx, req.NamespacedName, bwsecret)

	//Delete event.
	if err != nil && errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		//Other lookup error
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	authSecret := &corev1.Secret{}
	namespacedSecret := types.NamespacedName{
		Name:      bwsecret.Spec.AuthToken.SecretName,
		Namespace: ns,
	}
	err = r.Client.Get(ctx, namespacedSecret, authSecret)
	if err != nil {
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	authToken := string(authSecret.Data[bwsecret.Spec.AuthToken.SecretKey])
	orgId := bwsecret.Spec.OrganizationId

	bitwardenClient, err := sdk.NewBitwardenClient(&bwApiUrl, &identApiUrl)
	if err != nil {
		logger.Error(err, "Faled to start client")
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	err = bitwardenClient.AccessTokenLogin(authToken, &statePath)
	if err != nil {
		logger.Error(err, "Faled to authenticate")
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	secrets := map[string][]byte{}
	secretIds := []string{}

	smSecrets, err := bitwardenClient.Secrets.List(orgId)

	if err != nil {
		logger.Error(err, "Faled to list secrets")
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	for _, smSecret := range smSecrets.Data {
		secretIds = append(secretIds, smSecret.ID)
	}

	smSecretVals, err := bitwardenClient.Secrets.GetByIDS(secretIds)

	if err != nil {
		logger.Error(err, "Faled to get secrets by id")
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	for _, smSecretVal := range smSecretVals.Data {
		secrets[smSecretVal.ID] = []byte(smSecretVal.Value)
	}

	defer bitwardenClient.Close()

	//Clean-up removed secrets
	secretList := &corev1.SecretList{}
	ops := []client.ListOption{
		client.InNamespace(ns),
		client.MatchingLabels{"k8s.bitwarden.com/bw-secret": string(bwsecret.UID)},
	}

	r.List(ctx, secretList, ops...)

	secret := &corev1.Secret{}
	namespacedSecret = types.NamespacedName{
		Name:      bwsecret.Spec.SecretName,
		Namespace: ns,
	}
	err = r.Get(ctx, namespacedSecret, secret)
	//Creating new
	if err != nil && errors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        bwsecret.Spec.SecretName,
				Namespace:   ns,
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
		secret.ObjectMeta.Labels["k8s.bitwarden.com/bw-secret"] = string(bwsecret.UID)

		if err := ctrl.SetControllerReference(bwsecret, secret, r.Scheme); err != nil {
			logger.Error(err, "Failed to set controller reference")
			return ctrl.Result{
				RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
			}, err
		}

		err := r.Create(ctx, secret)
		if err != nil {
			return ctrl.Result{
				RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
			}, err
		}

	}

	for k := range secret.Data {
		delete(secret.Data, k)
	}

	for _, mappedSecret := range bwsecret.Spec.SecretMap {
		secrets[mappedSecret.SecretKeyName] = secrets[mappedSecret.BwSecretId]
		delete(secrets, mappedSecret.BwSecretId)
	}

	secret.Data = secrets

	if secret.ObjectMeta.Annotations == nil {
		secret.ObjectMeta.Annotations = map[string]string{}
	}

	secret.ObjectMeta.Annotations["k8s.bitwarden.com/sync-time"] = fmt.Sprint(time.Now().UTC())

	if bwsecret.Spec.SecretMap == nil {
		delete(secret.ObjectMeta.Annotations, "k8s.bitwarden.com/custom-map")
	} else {
		bytes, err := json.MarshalIndent(bwsecret.Spec.SecretMap, "", "  ")
		if err != nil {
			logger.Error(err, "Error recording map to attribute.")
		}
		secret.ObjectMeta.Annotations["k8s.bitwarden.com/custom-map"] = string(bytes)
	}

	secret.ObjectMeta.Annotations["k8s.bitwarden.com/sync-time"] = fmt.Sprint(time.Now().UTC())

	err = r.Update(ctx, secret)
	if err != nil {
		logger.Error(err, "Failed to update "+req.Name)
		return ctrl.Result{
			RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
		}, err
	}

	logger.Info("Completed sync for " + req.Name)
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
