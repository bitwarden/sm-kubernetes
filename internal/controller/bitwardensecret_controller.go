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
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorsv1 "github.com/bitwarden/sm-kubernetes/api/v1"
	"github.com/bitwarden/sm-kubernetes/internal/sm"
)

const (
	LabelBwSecret       = "k8s.bitwarden.com/bw-secret"
	AnnotationSyncTime  = "k8s.bitwarden.com/sync-time"
	AnnotationCustomMap = "k8s.bitwarden.com/custom-map"
)

// BitwardenSecretReconciler reconciles a BitwardenSecret object
type BitwardenSecretReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	BitwardenClientFactory  sm.BitwardenClientFactory
	StatePath               string
	RefreshIntervalSeconds  int
	SetK8sSecretAnnotations func(*operatorsv1.BitwardenSecret, *corev1.Secret) error
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

	bwSecret := &operatorsv1.BitwardenSecret{}

	err := r.Get(ctx, req.NamespacedName, bwSecret) //Try to get the Bitwarden Secret from the K8s api

	// Deleted Bitwarden Secret event.
	if err != nil {

		//Error was due to missing item
		if k8serrors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("%s/%s was deleted.", req.NamespacedName.Namespace, req.Name))
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Error looking up BitwardenSecret", "namespace", req.NamespacedName.Namespace, "name", req.Name)

		//Other lookup error
		return ctrl.Result{RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second}, err
	}

	// Validate that useSecretNames and onlyMappedSecrets are not both enabled
	if bwSecret.Spec.UseSecretNames && bwSecret.Spec.OnlyMappedSecrets {
		err := fmt.Errorf("useSecretNames and onlyMappedSecrets cannot both be enabled; these options are mutually exclusive")
		logErr := r.LogError(logger, ctx, bwSecret, err, "Invalid BitwardenSecret configuration")
		return ctrl.Result{}, logErr
	}

	lastSync := bwSecret.Status.LastSuccessfulSyncTime

	// If the K8s Secret doesn't exist, force a full sync so it gets recreated
	// regardless of whether Bitwarden reports changes since the last sync.
	existingK8sSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: bwSecret.Spec.SecretName, Namespace: req.NamespacedName.Namespace}, existingK8sSecret); err != nil && k8serrors.IsNotFound(err) {
		lastSync = metav1.Time{}
	}

	nextSync := lastSync.Time.Add(time.Duration(r.RefreshIntervalSeconds) * time.Second)

	if !lastSync.IsZero() && time.Now().UTC().Before(nextSync) {
		remaining := time.Until(nextSync)
		logger.Info(fmt.Sprintf("Skipping sync for %s/%s — last sync was %s, next sync in %s",
			req.NamespacedName.Namespace, req.Name, lastSync.Time.Format("15:04:05"), remaining.Round(time.Second)))
		return ctrl.Result{
			RequeueAfter: remaining,
		}, nil
	}

	message := fmt.Sprintf("Syncing  %s/%s", req.NamespacedName.Namespace, req.Name)
	logger.Info(message)

	//Need to retrieve the Bitwarden authorization token
	authK8sSecret := &corev1.Secret{}
	namespacedAuthK8sSecret := types.NamespacedName{
		Name:      bwSecret.Spec.AuthToken.SecretName,
		Namespace: req.NamespacedName.Namespace,
	}

	err = r.Get(ctx, namespacedAuthK8sSecret, authK8sSecret)

	if err != nil {
		logErr := r.LogError(logger, ctx, bwSecret, err, "Error pulling authorization token secret")

		return ctrl.Result{
			RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
		}, logErr
	}

	data, ok := authK8sSecret.Data[bwSecret.Spec.AuthToken.SecretKey]
	if !ok || authK8sSecret.Data == nil {
		err := fmt.Errorf("auth token secret key %s not found in %s/%s", bwSecret.Spec.AuthToken.SecretKey, req.NamespacedName.Namespace, bwSecret.Spec.AuthToken.SecretName)
		logErr := r.LogError(logger, ctx, bwSecret, err, "Invalid authorization token secret")
		return ctrl.Result{RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second}, logErr
	}
	authToken := string(data)
	orgId := bwSecret.Spec.OrganizationId

	//Get the secrets from the Bitwarden API based on lastSync and organizationId
	//This will also indicate if the Bitwarden secret needs to be refreshed
	refresh, secrets, err := sm.PullSecretManagerSecretDeltas(r.BitwardenClientFactory, r.StatePath, logger, orgId, authToken, lastSync.Time, bwSecret.Spec.UseSecretNames, bwSecret.Spec.ProjectId)

	if err != nil {
		logErr := r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Error pulling Secret Manager secrets from API => API: %s -- Identity: %s -- State: %s -- OrgId: %s ", r.BitwardenClientFactory.GetApiUrl(), r.BitwardenClientFactory.GetIdentityApiUrl(), r.StatePath, orgId))

		return ctrl.Result{
			RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
		}, logErr
	}

	if refresh {
		//Get The Bitwarden Secret from the K8s api
		k8sSecret := &corev1.Secret{}

		namespacedK8sSecret := types.NamespacedName{
			Name:      bwSecret.Spec.SecretName,
			Namespace: req.NamespacedName.Namespace,
		}

		err = r.Get(ctx, namespacedK8sSecret, k8sSecret)

		//Bitwarden secret doesn't exist; need to create it
		if err != nil && k8serrors.IsNotFound(err) {
			k8sSecret = CreateK8sSecret(bwSecret)

			// Set up the controller reference; Handle any error
			if err := ctrl.SetControllerReference(bwSecret, k8sSecret, r.Scheme); err != nil {
				logError := r.LogError(logger, ctx, bwSecret, err, "Failed to set controller reference")
				return ctrl.Result{
					RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
				}, logError
			}

			// Create the new Bitwarden Secret; Handle any error
			if err := r.Create(ctx, k8sSecret); err != nil {
				logError := r.LogError(logger, ctx, bwSecret, err, "Creation of K8s secret failed.")
				return ctrl.Result{
					RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
				}, logError
			}

			r.Get(ctx, namespacedK8sSecret, k8sSecret) //Ensuring we have the latest version of the object.

		}

		secretDeepCopy := k8sSecret.DeepCopy() //Need a copy of the original

		if k8sSecret.ObjectMeta.Labels == nil {
			k8sSecret.ObjectMeta.Labels = map[string]string{}
		}
		k8sSecret.ObjectMeta.Labels[LabelBwSecret] = string(bwSecret.UID)

		sm.ApplySecretMap(secrets, bwSecret, k8sSecret)

		err = r.SetK8sSecretAnnotations(bwSecret, k8sSecret)

		if err != nil {
			r.LogWarning(logger, ctx, bwSecret, err, fmt.Sprintf("Error setting annotations for  %s/%s", req.NamespacedName.Namespace, req.Name)) //Annotation failure is not critical. Log, but don't fail the process
		}

		secretPatch := client.MergeFrom(secretDeepCopy)
		err = r.Patch(ctx, k8sSecret, secretPatch)
		if err != nil {
			logError := r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Failed to update  %s/%s", req.NamespacedName.Namespace, req.Name))
			return ctrl.Result{
				RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
			}, logError
		}

		if logError := r.LogCompletion(logger, ctx, bwSecret, fmt.Sprintf("Completed sync for %s/%s", req.NamespacedName.Namespace, req.Name)); logError != nil {
			return ctrl.Result{
				RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
			}, logError
		}
	} else {
		logger.Info(fmt.Sprintf("No changes to %s/%s.  Skipping sync.", req.NamespacedName.Namespace, req.Name))
	}

	return ctrl.Result{
		RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BitwardenSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.SetK8sSecretAnnotations == nil {
		r.SetK8sSecretAnnotations = SetK8sSecretAnnotations
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorsv1.BitwardenSecret{}).
		// All BitwardenSecret CRs share a single on-disk state file (r.StatePath) with
		// no locking. Reconciles must stay serialized to avoid concurrent
		// AccessTokenLogin calls for different orgs/tokens racing on that file.
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func (r *BitwardenSecretReconciler) LogWarning(logger logr.Logger, ctx context.Context, bwSecret *operatorsv1.BitwardenSecret, err error, message string) {
	logger.Error(err, message) // Log as warning or error
}

func (r *BitwardenSecretReconciler) LogError(logger logr.Logger, ctx context.Context, bwSecret *operatorsv1.BitwardenSecret, err error, message string) error {
	logger.Error(err, message)

	// Re-fetch to get the latest version before status update to avoid conflict errors
	if fetchErr := r.Get(ctx, types.NamespacedName{
		Name:      bwSecret.Name,
		Namespace: bwSecret.Namespace,
	}, bwSecret); fetchErr != nil {
		logger.Error(fetchErr, "Failed to re-fetch BitwardenSecret before status update")
		return fetchErr
	}

	errorCondition := metav1.Condition{
		Status:  metav1.ConditionFalse,
		Reason:  "ReconciliationFailed",
		Message: fmt.Sprintf("%s - %s", message, err.Error()),
		Type:    "FailedSync",
	}

	apimeta.SetStatusCondition(&bwSecret.Status.Conditions, errorCondition)
	if updateErr := r.Status().Update(ctx, bwSecret); updateErr != nil {
		logger.Error(updateErr, "Failed to update BitwardenSecret status")
		return updateErr
	}

	return err
}

func (r *BitwardenSecretReconciler) LogCompletion(logger logr.Logger, ctx context.Context, bwSecret *operatorsv1.BitwardenSecret, message string) error {
	logger.Info(message)

	// Re-fetch to get the latest version before status update to avoid conflict errors
	if err := r.Get(ctx, types.NamespacedName{
		Name:      bwSecret.Name,
		Namespace: bwSecret.Namespace,
	}, bwSecret); err != nil {
		logger.Error(err, "Failed to re-fetch BitwardenSecret before status update")
		return err
	}

	completeCondition := metav1.Condition{
		Status:  metav1.ConditionTrue,
		Reason:  "ReconciliationComplete",
		Message: message,
		Type:    "SuccessfulSync",
	}

	bwSecret.Status.LastSuccessfulSyncTime = metav1.Time{Time: time.Now().UTC()}

	apimeta.SetStatusCondition(&bwSecret.Status.Conditions, completeCondition)
	if updateErr := r.Status().Update(ctx, bwSecret); updateErr != nil {
		logger.Error(updateErr, "Failed to update BitwardenSecret status")
		return updateErr
	}

	return nil
}

func CreateK8sSecret(bwSecret *operatorsv1.BitwardenSecret) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        bwSecret.Spec.SecretName,
			Namespace:   bwSecret.Namespace,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}
	return secret
}

var SetK8sSecretAnnotations = func(bwSecret *operatorsv1.BitwardenSecret, secret *corev1.Secret) error {
	if secret.ObjectMeta.Annotations == nil {
		secret.ObjectMeta.Annotations = make(map[string]string)
	}

	secret.ObjectMeta.Annotations[AnnotationSyncTime] = time.Now().UTC().Format(time.RFC3339Nano)

	if bwSecret.Spec.SecretMap == nil {
		delete(secret.ObjectMeta.Annotations, AnnotationCustomMap)
	} else {
		bytes, err := json.MarshalIndent(bwSecret.Spec.SecretMap, "", "  ")
		if err != nil {
			return err
		}
		secret.ObjectMeta.Annotations[AnnotationCustomMap] = string(bytes)
	}

	return nil
}
