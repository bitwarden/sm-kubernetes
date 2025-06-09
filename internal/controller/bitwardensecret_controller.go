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

	"encoding/json"

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

const (
	LabelBwSecret       = "k8s.bitwarden.com/bw-secret"
	AnnotationSyncTime  = "k8s.bitwarden.com/sync-time"
	AnnotationCustomMap = "k8s.bitwarden.com/custom-map"
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

	bwSecret := &operatorsv1.BitwardenSecret{}

	err := r.Get(ctx, req.NamespacedName, bwSecret) //Try to get the Bitwarden Secret from the K8s api

	// Deleted Bitwarden Secret event.
	if err != nil {

		//Error was due to missing item
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("%s/%s was deleted.", req.Namespace, req.Name))
			return ctrl.Result{}, nil
		}

		logErr := r.LogError(logger, ctx, bwSecret, err, "Error looking up BitwardenSecret")

		//Other lookup error
		return ctrl.Result{RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second}, logErr
	}

	lastSync := bwSecret.Status.LastSuccessfulSyncTime

	// Reconcile was queued by last sync time status update on the BitwardenSecret.  We will ignore it.
	if !lastSync.IsZero() && time.Now().UTC().Before(lastSync.Time.Add(time.Duration(r.RefreshIntervalSeconds)*time.Second)) {
		return ctrl.Result{}, nil
	}

	message := fmt.Sprintf("Syncing  %s/%s", req.Namespace, req.Name)
	logger.Info(message)

	//Need to retrieve the Bitwarden authorization token
	authK8sSecret := &corev1.Secret{}
	namespacedAuthK8sSecret := types.NamespacedName{
		Name:      bwSecret.Spec.AuthToken.SecretName,
		Namespace: req.Namespace,
	}

	err = r.Client.Get(ctx, namespacedAuthK8sSecret, authK8sSecret)

	if err != nil {
		logErr := r.LogError(logger, ctx, bwSecret, err, "Error pulling authorization token secret")

		return ctrl.Result{
			RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
		}, logErr
	}

	data, ok := authK8sSecret.Data[bwSecret.Spec.AuthToken.SecretKey]
	if !ok || authK8sSecret.Data == nil {
		err := fmt.Errorf("auth token secret key %s not found in %s/%s", bwSecret.Spec.AuthToken.SecretKey, req.Namespace, bwSecret.Spec.AuthToken.SecretName)
		logErr := r.LogError(logger, ctx, bwSecret, err, "Invalid authorization token secret")
		return ctrl.Result{RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second}, logErr
	}
	authToken := string(data)
	orgId := bwSecret.Spec.OrganizationId

	//Get the secrets from the Bitwarden API based on lastSync and organizationId
	//This will also indicate if the Bitwarden variable needs to be refreshed
	refresh, secrets, err := r.PullSecretManagerSecretDeltas(logger, orgId, authToken, lastSync.Time)

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
			Namespace: req.Namespace,
		}

		err = r.Get(ctx, namespacedK8sSecret, k8sSecret)

		//Bitwarden secret doesn't exist.  need to create it
		if err != nil && errors.IsNotFound(err) {
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
		}

		ApplySecretMap(secrets, bwSecret, k8sSecret)

		err = SetK8sSecretAnnotations(bwSecret, k8sSecret)

		if err != nil {
			r.LogWarning(logger, ctx, bwSecret, err, fmt.Sprintf("Error setting annotations for  %s/%s", req.Namespace, req.Name)) //Annotation failure is not critical.  Log, but don't fail the process
		}

		err = r.Patch(ctx, k8sSecret, client.MergeFrom(k8sSecret.DeepCopy()))
		if err != nil {
			logError := r.LogError(logger, ctx, bwSecret, err, fmt.Sprintf("Failed to update  %s/%s", req.Namespace, req.Name))
			return ctrl.Result{
				RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
			}, logError
		}

		if logError := r.LogCompletion(logger, ctx, bwSecret, fmt.Sprintf("Completed sync for %s/%s", req.Namespace, req.Name)); logError != nil {
			return ctrl.Result{
				RequeueAfter: time.Duration(r.RefreshIntervalSeconds) * time.Second,
			}, logError
		}
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

func (r *BitwardenSecretReconciler) LogWarning(logger logr.Logger, ctx context.Context, bwSecret *operatorsv1.BitwardenSecret, err error, message string) {
	logger.Error(err, message) // Log as warning or error
}

func (r *BitwardenSecretReconciler) LogError(logger logr.Logger, ctx context.Context, bwSecret *operatorsv1.BitwardenSecret, err error, message string) error {
	logger.Error(err, message)

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

// This function will determine if any secrets have been updated and return all secrets assigned to the machine account if so.
// First returned value is a boolean stating if something changed or not.
// The second returned value is a mapping of secret IDs and their values from Secrets Manager
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

	for _, smSecretVal := range smSecretVals {
		secrets[smSecretVal.ID] = []byte(smSecretVal.Value)
	}

	defer bitwardenClient.Close()

	return smSecretResponse.HasChanges, secrets, nil
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
	secret.ObjectMeta.Labels[LabelBwSecret] = string(bwSecret.UID)
	return secret
}

func ApplySecretMap(secrets map[string][]byte, bwSecret *operatorsv1.BitwardenSecret, k8sSecret *corev1.Secret) {
	if k8sSecret.Data == nil {
		k8sSecret.Data = make(map[string][]byte)
	}

	//If we are doing a straight up synch with no map, dump them across and return
	if !bwSecret.Spec.OnlyMappedSecrets && len(bwSecret.Spec.SecretMap) == 0 {
		k8sSecret.Data = secrets
		return
	}

	for key, secret := range secrets {
		mapping, isThere := FindSecretMapByBwSecretId(&bwSecret.Spec, key) //see if this particular secret is in the map
		if bwSecret.Spec.OnlyMappedSecrets && !isThere {
			continue //Not in map and we're only synching mapped secrets, so move on.
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

func SetK8sSecretAnnotations(bwSecret *operatorsv1.BitwardenSecret, secret *corev1.Secret) error {
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
