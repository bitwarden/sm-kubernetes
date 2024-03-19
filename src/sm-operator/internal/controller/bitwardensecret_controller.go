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

package controller

import (
	"context"
	"fmt"
	"time"

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

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
)

// BitwardenSecretReconciler reconciles a BitwardenSecret object
type BitwardenSecretReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	BitwardenClientFactory BitwardenClientFactory
	StatePath              string
	RefreshIntervalSeconds int
}

//+kubebuilder:rbac:groups=k8s.bitwarden.com,resources=bitwardensecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.bitwarden.com,resources=bitwardensecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.bitwarden.com,resources=bitwardensecrets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *BitwardenSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	message := fmt.Sprintf("Syncing  %s/%s", req.Namespace, req.Name)
	ns := req.Namespace

	bwSecret := &operatorsv1.BitwardenSecret{}

	err := r.Get(ctx, req.NamespacedName, bwSecret)

	// Deleted Bitwarden Secret event.
	if err != nil && errors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("%s/%s was deleted.", req.Namespace, req.Name))
		return ctrl.Result{}, nil
	} else if err != nil {
		r.LogError(logger, ctx, bwSecret, err, "Error looking up BitwardenSecret")
		//Other lookup error
		return ctrl.Result{
			RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
		}, err
	}

	lastSync := bwSecret.Status.LastSuccessfulSyncTime

	// Reconcile was queued by last sync time status update on the BitwardenSecret.  We will ignore it.
	if time.Now().UTC().Before(lastSync.Time.Add(1 * time.Second)) {
		return ctrl.Result{}, nil
	}

	logger.Info(message)

	authK8sSecret := &corev1.Secret{}
	namespacedAuthK8sSecret := types.NamespacedName{
		Name:      bwSecret.Spec.AuthToken.SecretName,
		Namespace: ns,
	}

	k8sSecret := &corev1.Secret{}
	namespacedK8sSecret := types.NamespacedName{
		Name:      bwSecret.Spec.SecretName,
		Namespace: ns,
	}

	err = r.Client.Get(ctx, namespacedAuthK8sSecret, authK8sSecret)

	if err != nil {
		r.LogError(logger, ctx, bwSecret, err, "Error pulling authorization token secret")
		return ctrl.Result{
			RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
		}, err
	}

	authToken := string(authK8sSecret.Data[bwSecret.Spec.AuthToken.SecretKey])
	orgId := bwSecret.Spec.OrganizationId

	// Delete deltas will be handled in the future
	refresh, secrets, err := r.PullSecretManagerSecretDeltas(logger, orgId, authToken, lastSync.Time)

	if err != nil {
		r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Error pulling Secret Manager secrets from API => API: %s -- Identity: %s -- State: %s -- OrgId: %s ", r.BitwardenClientFactory.GetApiUrl(), r.BitwardenClientFactory.GetIdentityApiUrl(), r.StatePath, orgId))
		return ctrl.Result{
			RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
		}, err
	}

	if refresh {
		err = r.Get(ctx, namespacedK8sSecret, k8sSecret)

		//Creating new
		if err != nil && errors.IsNotFound(err) {
			k8sSecret = bwSecret.CreateK8sSecret()

			// Cascading delete
			if err := ctrl.SetControllerReference(bwSecret, k8sSecret, r.Scheme); err != nil {
				r.LogError(logger, ctx, bwSecret, err, "Failed to set controller reference")
				return ctrl.Result{
					RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
				}, err
			}

			err := r.Create(ctx, k8sSecret)
			if err != nil {
				r.LogError(logger, ctx, bwSecret, err, "Creation of K8s secret failed.")
				return ctrl.Result{
					RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
				}, err
			}

		}

		UpdateSecretValues(k8sSecret, secrets)

		bwSecret.ApplySecretMap(k8sSecret)

		err = bwSecret.SetK8sSecretAnnotations(k8sSecret)

		if err != nil {
			r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Error setting annotations for  %s/%s", req.Namespace, req.Name))
		}

		err = r.Update(ctx, k8sSecret)
		if err != nil {
			r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Failed to update  %s/%s", req.Namespace, req.Name))
			return ctrl.Result{
				RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
			}, err
		}

		r.LogCompletion(logger, ctx, bwSecret, fmt.Sprintf("Completed sync for %s/%s", req.Namespace, req.Name))
	} else {
		logger.Info(fmt.Sprintf("No changes to %s/%s.  Skipping sync.", req.Namespace, req.Name))
	}

	return ctrl.Result{
		RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
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
// we will have a delta pull method to only pull what has changed
// First returned value is the Adds/Updates.  The second returned value is the array of removed IDs.  As the delta call doesn't exist, this is
// included for future use
func (r *BitwardenSecretReconciler) PullSecretManagerSecretDeltas(logger logr.Logger, orgId string, authToken string, lastSync time.Time) (bool, map[string][]byte, error) {
	bitwardenClient, err := r.BitwardenClientFactory.GetBitwardenClient()
	if err != nil {
		logger.Error(err, "Failed to create client")
		return false, nil, err
	}

	err = bitwardenClient.AccessTokenLogin(authToken, &r.StatePath)
	if err != nil {
		logger.Error(err, "Failed to authenticate")
		return false, nil, err
	}

	secrets := map[string][]byte{}
	secretIds := []string{}

	// Use a deltas endpoint in the future
	smSecrets, err := bitwardenClient.GetSecrets().List(orgId)

	if err != nil {
		logger.Error(err, "Failed to list secrets")
		return false, nil, err
	}

	for _, smSecret := range smSecrets.Data {
		secretIds = append(secretIds, smSecret.ID)
	}

	// Use a deltas endpoint in the future
	smSecretVals, err := bitwardenClient.GetSecrets().GetByIDS(secretIds)

	if err != nil {
		logger.Error(err, "Failed to get secrets by id")
		return false, nil, err
	}

	for _, smSecretVal := range smSecretVals.Data {
		secrets[smSecretVal.ID] = []byte(smSecretVal.Value)
	}

	defer bitwardenClient.Close()

	// Once a new deltas endpoint is setup, the first value will be the boolean of whether something has changed.
	return true, secrets, nil
}

func UpdateSecretValues(secret *corev1.Secret, secrets map[string][]byte) {
	secret.Data = secrets
}
