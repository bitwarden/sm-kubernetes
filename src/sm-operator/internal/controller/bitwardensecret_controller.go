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

	// "github.com/bitwarden/sdk/languages/go/secrets"

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

	logger.Info("Rolling!")

	bwApiUrl := strings.TrimSpace(os.Getenv("BW_API_URL"))
	identApiUrl := strings.TrimSpace(os.Getenv("BW_IDENTITY_API_URL"))
	// orgId := strings.TrimSpace(os.Getenv("BW_SECRETS_MANAGER_ORG_ID"))

	refreshIntervalSeconds := 300

	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("BW_SECRETS_MANAGER_REFRESH_INTERVAL"))); err == nil {
		if value >= 180 {
			refreshIntervalSeconds = value
		}
	}

	if bwApiUrl == "" {
		bwApiUrl = "http://api.bitwarden.com"
	}

	if identApiUrl == "" {
		identApiUrl = "http://identity.bitwarden.com"
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

	if strings.TrimSpace(bwsecret.Spec.BitwardenApiUrl) != "" {
		bwApiUrl = strings.TrimSpace(bwsecret.Spec.BitwardenApiUrl)
	}

	if strings.TrimSpace(bwsecret.Spec.IdentityApiUrl) != "" {
		identApiUrl = strings.TrimSpace(bwsecret.Spec.IdentityApiUrl)
	}

	// if strings.TrimSpace(bwsecret.Spec.OrganizationId) != "" {
	// 	orgId = strings.TrimSpace(bwsecret.Spec.OrganizationId)
	// }

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

	// authToken := string(authSecret.Data[bwsecret.Spec.AuthToken.SecretKey])

	//Clean-up removed secrets
	secretList := &corev1.SecretList{}
	ops := []client.ListOption{
		client.InNamespace(ns),
		client.MatchingLabels{"k8s.bitwarden.com/bw-secret": string(bwsecret.UID)},
	}

	r.List(ctx, secretList, ops...)

	for _, k8sSecret := range secretList.Items {
		found := false

		for _, project := range bwsecret.Spec.Projects {
			if project.SecretName == k8sSecret.Name {
				found = true
				break
			}
		}

		if found {
			continue
		}

		r.Delete(ctx, &k8sSecret)
	}

	for _, project := range bwsecret.Spec.Projects {
		secret := &corev1.Secret{}
		namespacedSecret := types.NamespacedName{
			Name:      project.SecretName,
			Namespace: ns,
		}
		err := r.Get(ctx, namespacedSecret, secret)
		//Creating new
		if err != nil && errors.IsNotFound(err) {
			newSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      project.SecretName,
					Namespace: ns,
					Labels:    map[string]string{},
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{},
			}
			newSecret.ObjectMeta.Labels["k8s.bitwarden.com/bw-secret"] = string(bwsecret.UID)
			for _, mappedSecret := range project.SecretMap {
				newSecret.Data[mappedSecret.SecretKeyName] = []byte(mappedSecret.BwSecretId)
			}

			if err := ctrl.SetControllerReference(bwsecret, newSecret, r.Scheme); err != nil {
				logger.Error(err, "Failed to set controller reference")
				return ctrl.Result{
					RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
				}, err
			}

			err := r.Create(ctx, newSecret)
			if err != nil {
				return ctrl.Result{
					RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
				}, err
			}

		} else {
			//Update existing
			secret.ObjectMeta.Labels["k8s.bitwarden.com/bw-secret"] = string(bwsecret.UID)

			for k := range secret.Data {
				delete(secret.Data, k)
			}

			for _, mappedSecret := range project.SecretMap {
				secret.Data[mappedSecret.SecretKeyName] = []byte(mappedSecret.BwSecretId)
			}
			err := r.Update(ctx, secret)
			if err != nil {
				return ctrl.Result{
					RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
				}, err
			}
		}
	}

	return ctrl.Result{
		RequeueAfter: time.Duration(refreshIntervalSeconds) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BitwardenSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.BitwardenSecret{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
