/*
http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controller implements the heat-operator Kubernetes controllers.
package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/cronjob"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"
	"github.com/openstack-k8s-operators/lib-common/modules/common/object"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	heat "github.com/openstack-k8s-operators/heat-operator/internal/heat"
	"github.com/openstack-k8s-operators/heat-operator/internal/heatapi"
	heatcfnapi "github.com/openstack-k8s-operators/heat-operator/internal/heatcfnapi"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"github.com/openstack-k8s-operators/lib-common/modules/openstack"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	memcachedv1 "github.com/openstack-k8s-operators/infra-operator/apis/memcached/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	topologyv1 "github.com/openstack-k8s-operators/infra-operator/apis/topology/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Static errors for heat controller
var (
	ErrPasswordSelectorNotFound  = errors.New("password selector not found in secret")
	ErrAuthEncryptionKeyTooShort = errors.New("AuthEncryptionKey must be at least 32 characters")
	ErrACSecretNotFound          = errors.New("ApplicationCredential secret not found")
	ErrACSecretMissingKeys       = errors.New("ApplicationCredential secret missing required keys")
)

// HeatReconciler reconciles a Heat object
type HeatReconciler struct {
	client.Client
	APIReader client.Reader
	Kclient   kubernetes.Interface
	Scheme    *runtime.Scheme
}

// GetLogger returns a logger object with a prefix of "controller.name" and additional controller context fields
func (r *HeatReconciler) GetLogger(ctx context.Context) logr.Logger {
	return log.FromContext(ctx).WithName("Controllers").WithName("Heat")
}

type conditionUpdater interface {
	Set(c *condition.Condition)
	MarkTrue(t condition.Type, messageFormat string, messageArgs ...any)
}

type topologyHandler interface {
	GetSpecTopologyRef() *topologyv1.TopoRef
	GetLastAppliedTopology() *topologyv1.TopoRef
	SetLastAppliedTopology(t *topologyv1.TopoRef)
}

func ensureTopology(
	ctx context.Context,
	helper *helper.Helper,
	instance topologyHandler,
	finalizer string,
	conditionUpdater conditionUpdater,
	defaultLabelSelector metav1.LabelSelector,
) (*topologyv1.Topology, error) {

	topology, err := topologyv1.EnsureServiceTopology(
		ctx,
		helper,
		instance.GetSpecTopologyRef(),
		instance.GetLastAppliedTopology(),
		finalizer,
		defaultLabelSelector,
	)
	if err != nil {
		conditionUpdater.Set(condition.FalseCondition(
			condition.TopologyReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TopologyReadyErrorMessage,
			err.Error()))
		return nil, fmt.Errorf("waiting for Topology requirements: %w", err)
	}
	// update the Status with the last retrieved Topology (or set it to nil)
	instance.SetLastAppliedTopology(instance.GetSpecTopologyRef())
	// update the Topology condition only when a Topology is referenced and has
	// been retrieved (err == nil)
	if tr := instance.GetSpecTopologyRef(); tr != nil {
		// update the TopologyRef associated condition
		conditionUpdater.MarkTrue(
			condition.TopologyReadyCondition,
			condition.TopologyReadyMessage,
		)
	}
	return topology, nil
}

var keystoneAPI *keystonev1.KeystoneAPI

// +kubebuilder:rbac:groups=heat.openstack.org,resources=heats,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heats/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapis/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatengines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatengines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatengines/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=memcached.openstack.org,resources=memcacheds,verbs=get;list;watch;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbaccounts/finalizers,verbs=update;patch

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update;patch
// service account permissions that are needed to grant permission to the above
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=nonroot-v2,resources=securitycontextconstraints,verbs=use

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *HeatReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	Log := r.GetLogger(ctx)

	instance := &heatv1beta1.Heat{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	isNewInstance, savedConditions := verifyStatusConditions(instance.Status.Conditions)

	// Always patch the instance status when exiting this function so we can
	// persist any changes.
	defer func() {
		// Don't update the status, if reconciler Panics
		if rc := recover(); rc != nil {
			Log.Info(fmt.Sprintf("panic during reconcile %v\n", rc))
			panic(rc)
		}
		condition.RestoreLastTransitionTimes(
			&instance.Status.Conditions, savedConditions)
		if instance.Status.Conditions.IsUnknown(condition.ReadyCondition) {
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}
		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	//
	// initialize status
	//
	cl := instance.StatusConditionsList()
	instance.Status.Conditions.Init(&cl)
	instance.Status.ObservedGeneration = instance.Generation

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) || isNewInstance {
		return ctrl.Result{}, nil
	}

	if instance.Status.Hash == nil {
		instance.Status.Hash = make(map[string]string)
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// fields to index to reconcile when change
const (
	passwordSecretField      = ".spec.secret"
	transportURLSecretField  = ".spec.transportURLSecret"
	caBundleSecretNameField  = ".spec.tls.caBundleSecretName" // #nosec G101
	tlsAPIInternalField      = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField        = ".spec.tls.api.public.secretName"
	customServiceConfigField = ".spec.customServiceConfigSecrets"
	topologyField            = ".spec.topologyRef.Name"
	authAppCredSecretField   = ".spec.auth.applicationCredentialSecret" // #nosec G101
)

var (
	heatWatchFields = []string{
		passwordSecretField,
		customServiceConfigField,
		authAppCredSecretField,
	}
	heatAPIWatchFields = []string{
		passwordSecretField,
		transportURLSecretField,
		caBundleSecretNameField,
		tlsAPIInternalField,
		tlsAPIPublicField,
		customServiceConfigField,
		topologyField,
	}
	heatCfnWatchFields = []string{
		passwordSecretField,
		transportURLSecretField,
		caBundleSecretNameField,
		tlsAPIInternalField,
		tlsAPIPublicField,
		customServiceConfigField,
		topologyField,
	}
	heatEngineWatchFields = []string{
		passwordSecretField,
		transportURLSecretField,
		caBundleSecretNameField,
		customServiceConfigField,
		topologyField,
	}
)

// SetupWithManager sets up the controller with the Manager.
func (r *HeatReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	Log := r.GetLogger(ctx)
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.Heat{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.Heat)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index customServiceConfigSecrets
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.Heat{}, customServiceConfigField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.Heat)
		if cr.Spec.CustomServiceConfigSecrets == nil {
			return nil
		}
		return cr.Spec.CustomServiceConfigSecrets
	}); err != nil {
		return err
	}

	memcachedFn := func(_ context.Context, o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all Heat CRs
		heats := &heatv1beta1.HeatList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.List(context.Background(), heats, listOpts...); err != nil {
			Log.Error(err, "Unable to retrieve Heat CRs %w")
			return nil
		}

		for _, cr := range heats.Items {
			if o.GetName() == cr.Spec.MemcachedInstance {
				name := client.ObjectKey{
					Namespace: o.GetNamespace(),
					Name:      cr.Name,
				}
				Log.Info(fmt.Sprintf("Memcached %s is used by Heat CR %s", o.GetName(), cr.Name))
				result = append(result, reconcile.Request{NamespacedName: name})
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}

	// index authAppCredSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.Heat{}, authAppCredSecretField, func(rawObj client.Object) []string {
		// Extract the application credential secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.Heat)
		if cr.Spec.Auth.ApplicationCredentialSecret == "" {
			return nil
		}
		return []string{cr.Spec.Auth.ApplicationCredentialSecret}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&heatv1beta1.Heat{}).
		Owns(&heatv1beta1.HeatAPI{}).
		Owns(&heatv1beta1.HeatCfnAPI{}).
		Owns(&heatv1beta1.HeatEngine{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Watches(&memcachedv1.Memcached{},
			handler.EnqueueRequestsFromMapFunc(memcachedFn)).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&keystonev1.KeystoneAPI{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectForSrc),
			builder.WithPredicates(keystonev1.KeystoneAPIStatusChangedPredicate)).
		Complete(r)
}

func (r *HeatReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	for _, field := range heatWatchFields {
		crList := &heatv1beta1.HeatList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			Log.Error(err, fmt.Sprintf("listing %s for field: %s - %s", crList.GroupVersionKind().Kind, field, src.GetNamespace()))
			return requests
		}

		for _, item := range crList.Items {
			Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

			requests = append(requests,
				reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				},
			)
		}
	}

	return requests
}

func (r *HeatReconciler) findObjectForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	Log := r.GetLogger(ctx)

	crList := &heatv1beta1.HeatList{}
	listOps := &client.ListOptions{
		Namespace: src.GetNamespace(),
	}
	err := r.List(ctx, crList, listOps)
	if err != nil {
		Log.Error(err, fmt.Sprintf("listing %s for namespace: %s", crList.GroupVersionKind().Kind, src.GetNamespace()))
		return requests
	}

	for _, item := range crList.Items {
		Log.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

		requests = append(requests,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			},
		)
	}

	return requests
}

func (r *HeatReconciler) reconcileDelete(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Heat delete")

	// remove db finalizer first
	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, helper, heat.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Remove consumer finalizer from AC secrets Heat was consuming.
	// Check both status and spec to handle the edge case where the reconciler
	// crashed after adding the finalizer but before updating the status.
	for _, secretName := range []string{
		instance.Status.ApplicationCredentialSecret,
		instance.Spec.Auth.ApplicationCredentialSecret,
	} {
		if err := object.RemoveSecretConsumerFinalizer(ctx, helper, instance.Namespace,
			secretName, heat.ACConsumerFinalizer); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Remove consumer finalizer from transport secrets Heat was consuming.
	// Check both status and the TransportURL CR to handle the edge case where
	// the reconciler crashed after adding the finalizer but before updating
	// the status.
	transportSecretNames := []string{
		instance.Status.TransportURLSecret,
		instance.Status.NotificationsTransportURLSecret,
	}
	for _, tuName := range []string{
		fmt.Sprintf("%s-heat-transport", instance.Name),
		fmt.Sprintf("%s-heat-notifications-transport", instance.Name),
	} {
		tu := &rabbitmqv1.TransportURL{}
		if err := helper.GetClient().Get(ctx, types.NamespacedName{
			Name:      tuName,
			Namespace: instance.Namespace,
		}, tu); err != nil {
			if !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		} else {
			transportSecretNames = append(transportSecretNames, tu.Status.SecretName)
		}
	}
	for _, secretName := range transportSecretNames {
		if err := object.RemoveSecretConsumerFinalizer(ctx, helper, instance.Namespace,
			secretName, heat.TransportConsumerFinalizer); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	Log.Info("Reconciled Heat delete successfully")

	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileNormal(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Service")

	// Service account, role, binding
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"nonroot-v2"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
	}
	rbacResult, err := common_rbac.ReconcileRbac(ctx, helper, instance, rbacRules)
	if err != nil || (rbacResult != ctrl.Result{}) {
		return rbacResult, err
	}

	// Secret
	secretVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	// Associate to PasswordSelectors.Service field a password validator to
	// ensure pwd invalid detected patterns are rejected.
	validateFields := map[string]oko_secret.Validator{
		instance.Spec.PasswordSelectors.Service: oko_secret.PasswordValidator{},
	}
	hash, ctrlResult, err := oko_secret.VerifySecretFields(
		ctx,
		types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Secret},
		validateFields,
		helper.GetClient(),
		time.Duration(10)*time.Second,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		// Since the service secret should have been manually created by the user and referenced in the spec,
		// we treat this as a warning because it means that the service will not be able to start.
		log.FromContext(ctx).Info(fmt.Sprintf("OpenStack secret %s not found", instance.Spec.Secret))
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyWaitingMessage))
		return ctrlResult, err
	}
	secretVars[instance.Spec.Secret] = env.SetValue(hash)

	// Verify Application Credential secret if specified
	if instance.Spec.Auth.ApplicationCredentialSecret != "" {
		acSecret := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Auth.ApplicationCredentialSecret}
		acHash, _, err := oko_secret.VerifySecret(ctx, acSecret, []string{keystonev1.ACIDSecretKey, keystonev1.ACSecretSecretKey}, helper.GetClient(), 0)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				Log.Info("ApplicationCredential secret not found, waiting", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
			} else {
				Log.Error(err, "Failed to get ApplicationCredential secret", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		if acHash == "" {
			Log.Info("ApplicationCredential secret missing required keys", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				"ApplicationCredential secret %s missing required keys (AC_ID, AC_SECRET)",
				instance.Spec.Auth.ApplicationCredentialSecret))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		// AC secret exists and is valid - add to configVars for hash tracking
		secretVars[instance.Spec.Auth.ApplicationCredentialSecret] = env.SetValue(acHash)
	}

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// run check OpenStack secret - end

	//
	// Check for required memcached used for caching
	//
	memcached, err := memcachedv1.GetMemcachedByName(ctx, helper, instance.Spec.MemcachedInstance, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Memcached should be automatically created by the encompassing OpenStackControlPlane,
			// but we don't propagate its name into the "memcachedInstance" field of other sub-resources,
			// so if it is missing at this point, it *could* be because there's a mismatch between the
			// name of the Memcached CR and the name of the Memcached instance referenced by this CR.
			// Since that situation would block further reconciliation, we treat it as a warning.
			Log.Info(fmt.Sprintf("memcached %s not found", instance.Spec.MemcachedInstance))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.MemcachedReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.MemcachedReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if !memcached.IsReady() {
		Log.Info(fmt.Sprintf("memcached %s is not ready", memcached.Name))
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.MemcachedReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.MemcachedReadyWaitingMessage))
		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}
	// Mark the Memcached Service as Ready if we get to this point with no errors
	instance.Status.Conditions.MarkTrue(
		condition.MemcachedReadyCondition, condition.MemcachedReadyMessage)
	// run check memcached - end

	//
	// create RabbitMQ transportURL CR and get the actual URL from the associated secret that is created
	//
	serviceLabels := map[string]string{
		common.AppSelector: heat.ServiceName,
	}

	transportURL, op, err := r.transportURLCreateOrUpdate(
		instance,
		serviceLabels,
		instance.Spec.MessagingBus,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.RabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if op != controllerutil.OperationResultNone {
		Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	if transportURL.Status.SecretName == "" {
		Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))

		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.RabbitMqTransportURLReadyRunningMessage))

		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Set status early for first-time setup so PatchInstance persists it
	// even on early returns. During rotation (old != current), the status
	// is only updated by FinalizeSecretRotation at end of reconcile.
	if instance.Status.TransportURLSecret == "" ||
		instance.Status.TransportURLSecret == transportURL.Status.SecretName {
		instance.Status.TransportURLSecret = transportURL.Status.SecretName
	}

	if err := object.ManageSecretConsumerFinalizer(ctx, helper, instance.Namespace,
		transportURL.Status.SecretName, heat.TransportConsumerFinalizer); err != nil {
		return ctrl.Result{}, err
	}

	//
	// check for required TransportURL secret holding transport URL string
	//
	// transportURLFields are not pure password fields. We do not associate a
	// password validator and we only verify that the entry exists in the
	// secret
	transportValidateFields := map[string]oko_secret.Validator{
		"transport_url": oko_secret.NoOpValidator{},
	}
	hash, ctrlResult, err = oko_secret.VerifySecretFields(
		ctx,
		types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      transportURL.Status.SecretName,
		},
		transportValidateFields,
		helper.GetClient(),
		time.Duration(10)*time.Second)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.RabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		Log.Info(fmt.Sprintf("TransportURL secret %s not found", transportURL.Status.SecretName))
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.RabbitMqTransportURLReadyRunningMessage))
		return ctrlResult, err
	}
	secretVars[transportURL.Status.SecretName] = env.SetValue(hash)
	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(condition.RabbitMqTransportURLReadyCondition, condition.RabbitMqTransportURLReadyMessage)

	//
	// create notifications RabbitMQ transportURL if NotificationsBus is configured
	//
	var notificationsTransportURL *rabbitmqv1.TransportURL
	if instance.Spec.NotificationsBus != nil && instance.Spec.NotificationsBus.Cluster != "" {
		var notifOp controllerutil.OperationResult
		notificationsTransportURL, notifOp, err = r.notificationsTransportURLCreateOrUpdate(
			instance,
			serviceLabels,
			*instance.Spec.NotificationsBus,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NotificationBusInstanceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NotificationBusInstanceReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if notifOp != controllerutil.OperationResultNone {
			Log.Info(fmt.Sprintf("Notifications TransportURL %s successfully reconciled - operation: %s", notificationsTransportURL.Name, string(notifOp)))
		}

		if notificationsTransportURL.Status.SecretName == "" {
			Log.Info(fmt.Sprintf("Waiting for Notifications TransportURL %s secret to be created", notificationsTransportURL.Name))

			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NotificationBusInstanceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.NotificationBusInstanceReadyRunningMessage))

			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		// Set status early for first-time setup so PatchInstance persists it
		// even on early returns. During rotation (old != current), the status
		// is only updated by FinalizeSecretRotation at end of reconcile.
		if instance.Status.NotificationsTransportURLSecret == "" ||
			instance.Status.NotificationsTransportURLSecret == notificationsTransportURL.Status.SecretName {
			instance.Status.NotificationsTransportURLSecret = notificationsTransportURL.Status.SecretName
		}

		if err := object.ManageSecretConsumerFinalizer(ctx, helper, instance.Namespace,
			notificationsTransportURL.Status.SecretName, heat.TransportConsumerFinalizer); err != nil {
			return ctrl.Result{}, err
		}

		//
		// check for required Notifications TransportURL secret
		//
		// transportURLFields are not pure password fields. We do not associate a
		// password validator and we only verify that the entry exists in the
		// secret
		transportValidateFields := map[string]oko_secret.Validator{
			"transport_url": oko_secret.NoOpValidator{},
		}
		hash, ctrlResult, err := oko_secret.VerifySecretFields(
			ctx,
			types.NamespacedName{
				Namespace: instance.Namespace,
				Name:      notificationsTransportURL.Status.SecretName,
			},
			transportValidateFields,
			helper.GetClient(),
			time.Duration(10)*time.Second,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NotificationBusInstanceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.NotificationBusInstanceReadyErrorMessage,
				err.Error()))
			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			Log.Info(fmt.Sprintf("TransportURL secret %s not found", notificationsTransportURL.Status.SecretName))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.NotificationBusInstanceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.NotificationBusInstanceReadyRunningMessage))
			return ctrlResult, err
		}
		secretVars[notificationsTransportURL.Status.SecretName] = env.SetValue(hash)
		instance.Status.Conditions.MarkTrue(condition.NotificationBusInstanceReadyCondition, condition.NotificationBusInstanceReadyMessage)
	} else {
		// Notifications bus disabled. Config regenerated below no longer
		// references the notifications transport URL, so its input hash
		// changes and the Deployment rolls. Defer teardown of the
		// TransportURL and its consumer finalizer until that rollout is
		// complete (allServicesReady at end of reconcile), otherwise the
		// RabbitMQ user backing the secret would be revoked while pods still
		// use it.
		instance.Status.Conditions.Remove(condition.NotificationBusInstanceReadyCondition)
	}

	db, result, err := r.ensureDB(ctx, helper, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else if (result != ctrl.Result{}) {
		return result, nil
	}

	//
	// Create Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Secret required for Heat input
	// - %-config secret holding minimal heat config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	notificationsTransportURLSecretName := ""
	if notificationsTransportURL != nil {
		notificationsTransportURLSecretName = notificationsTransportURL.Status.SecretName
	}
	err = r.generateServiceSecrets(ctx, instance, helper, &secretVars, memcached, db, transportURL.Status.SecretName, notificationsTransportURLSecretName)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	_, err = r.createHashOfInputHashes(ctx, instance, secretVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	// Create Secrets - end

	// Add consumer finalizer to the new AC secret early, before deployment.
	// The old secret's finalizer is removed later (after all services deploy)
	// so that rapid rotations don't revoke a credential still in use by pods.
	if instance.Spec.Auth.ApplicationCredentialSecret != "" {
		if err := object.ManageSecretConsumerFinalizer(ctx, helper, instance.Namespace,
			instance.Spec.Auth.ApplicationCredentialSecret,
			heat.ACConsumerFinalizer); err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ServiceConfigReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.ServiceConfigReadyErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}
	}

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	// Handle service init
	ctrlResult, err = r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil || (ctrlResult != ctrl.Result{}) {
		return ctrlResult, err
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx)
	if err != nil || (ctrlResult != ctrl.Result{}) {
		return ctrlResult, err
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx)
	if err != nil || (ctrlResult != ctrl.Result{}) {
		return ctrlResult, err
	}

	// remove finalizers from previous MariaDBAccounts for which we have
	// switched.
	// TODO(zzzeek) - It's not clear if this is called too early here.
	// at the moment, heat_controller_test.go doesn't seem to have fixtures
	// I can use to simulate getting all the way to the end of a reconcile
	// for an instance.  Basically this should be called when any pods have
	// been restarted to run on an updated set of DB credentials, and the old
	// ones are no longer needed.  This would allow the scenario where
	// a new MariaDBAccount is created and an old MariaDBAccount is marked
	// deleted at once, where the finalizer will keep the old one around until
	// it's safe to drop.
	err = mariadbv1.DeleteUnusedMariaDBAccountFinalizers(ctx, helper, heat.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// create DBPurge CronJob
	// DBPurge is not optional and always created to purge all soft deleted
	// records. This command should be executed periodically to avoid heat
	// database becomes bigger by getting filled by soft-deleted records
	ctrlResult, err = r.ensureDBPurgeJob(ctx, helper, instance, serviceLabels)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.CronJobReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.CronJobReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}
	instance.Status.Conditions.MarkTrue(condition.CronJobReadyCondition, condition.CronJobReadyMessage)

	//
	// normal reconcile tasks
	//

	// Create domain for Heat stacks
	ospSecret, _, err := oko_secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	ctrlResult, err = r.ensureStackDomain(ctx, helper, instance, ospSecret)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatStackDomainReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatStackDomainReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatStackDomainReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			heatv1beta1.HeatStackDomainReadyRunningMessage))
		return ctrlResult, nil
	}
	instance.Status.Conditions.MarkTrue(heatv1beta1.HeatStackDomainReadyCondition, heatv1beta1.HeatStackDomainReadyMessage)

	// Compute expectedInputHash from all rotating secret names
	secretNames := []string{transportURL.Status.SecretName}
	if notificationsTransportURL != nil {
		secretNames = append(secretNames, notificationsTransportURL.Status.SecretName)
	}
	if instance.Spec.Auth.ApplicationCredentialSecret != "" {
		secretNames = append(secretNames, instance.Spec.Auth.ApplicationCredentialSecret)
	}
	expectedInputHash, err := util.ObjectHash(secretNames)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to compute expected input hash: %w", err)
	}

	// deploy heat-engine
	heatEngine, engineOp, err := r.engineDeploymentCreateOrUpdate(ctx, instance, memcached, transportURL.Status.SecretName, expectedInputHash)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatEngineReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			"%s", err.Error()))
		return ctrl.Result{}, err
	}

	if heatEngine.Generation == heatEngine.Status.ObservedGeneration &&
		heatEngine.Status.AppliedInputSecretHash == expectedInputHash {
		instance.Status.HeatEngineReadyCount = heatEngine.Status.ReadyCount
		c := heatEngine.Status.Conditions.Mirror(heatv1beta1.HeatEngineReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		if engineOp != controllerutil.OperationResultNone {
			Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(engineOp)))
		}
	} else {
		instance.Status.Conditions.Set(condition.UnknownCondition(
			heatv1beta1.HeatEngineReadyCondition,
			condition.RequestedReason,
			condition.DeploymentReadyRunningMessage))
	}

	// deploy heat-api
	heatAPI, apiOp, err := r.apiDeploymentCreateOrUpdate(ctx, instance, memcached, transportURL.Status.SecretName, expectedInputHash)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if heatAPI.Generation == heatAPI.Status.ObservedGeneration &&
		heatAPI.Status.AppliedInputSecretHash == expectedInputHash {
		instance.Status.HeatAPIReadyCount = heatAPI.Status.ReadyCount
		c := heatAPI.Status.Conditions.Mirror(heatv1beta1.HeatAPIReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		if apiOp != controllerutil.OperationResultNone {
			Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(apiOp)))
		}
	} else {
		instance.Status.Conditions.Set(condition.UnknownCondition(
			heatv1beta1.HeatAPIReadyCondition,
			condition.RequestedReason,
			condition.DeploymentReadyRunningMessage))
	}

	// deploy heat-api-cfn
	heatCfnAPI, cfnOp, err := r.cfnapiDeploymentCreateOrUpdate(ctx, instance, memcached, transportURL.Status.SecretName, expectedInputHash)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatCfnAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if heatCfnAPI.Generation == heatCfnAPI.Status.ObservedGeneration &&
		heatCfnAPI.Status.AppliedInputSecretHash == expectedInputHash {
		instance.Status.HeatCfnAPIReadyCount = heatCfnAPI.Status.ReadyCount
		c := heatCfnAPI.Status.Conditions.Mirror(heatv1beta1.HeatCfnAPIReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		if cfnOp != controllerutil.OperationResultNone {
			Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(cfnOp)))
		}
	} else {
		instance.Status.Conditions.Set(condition.UnknownCondition(
			heatv1beta1.HeatCfnAPIReadyCondition,
			condition.RequestedReason,
			condition.DeploymentReadyRunningMessage))
	}

	allServicesReady := heatEngine.Generation == heatEngine.Status.ObservedGeneration &&
		heatEngine.Status.AppliedInputSecretHash == expectedInputHash &&
		heatEngine.IsReady() &&
		heatAPI.Generation == heatAPI.Status.ObservedGeneration &&
		heatAPI.Status.AppliedInputSecretHash == expectedInputHash &&
		heatAPI.IsReady() &&
		heatCfnAPI.Generation == heatCfnAPI.Status.ObservedGeneration &&
		heatCfnAPI.Status.AppliedInputSecretHash == expectedInputHash &&
		heatCfnAPI.IsReady()

	acSecretName, err := object.FinalizeSecretRotation(
		ctx, helper, instance.Namespace,
		instance.Status.ApplicationCredentialSecret,
		instance.Spec.Auth.ApplicationCredentialSecret,
		heat.ACConsumerFinalizer,
		allServicesReady,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.ApplicationCredentialSecret = acSecretName

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}

	// Finalize transport URL rotation
	transportSecretName, err := object.FinalizeSecretRotation(
		ctx, helper, instance.Namespace,
		instance.Status.TransportURLSecret,
		transportURL.Status.SecretName,
		heat.TransportConsumerFinalizer,
		allServicesReady,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	instance.Status.TransportURLSecret = transportSecretName

	if notificationsTransportURL != nil {
		notifSecretName, err := object.FinalizeSecretRotation(
			ctx, helper, instance.Namespace,
			instance.Status.NotificationsTransportURLSecret,
			notificationsTransportURL.Status.SecretName,
			heat.TransportConsumerFinalizer,
			allServicesReady,
		)
		if err != nil {
			return ctrl.Result{}, err
		}
		instance.Status.NotificationsTransportURLSecret = notifSecretName
	} else if instance.Status.NotificationsTransportURLSecret != "" && allServicesReady {
		// Notifications bus disabled and the Deployment has rolled out a
		// config that no longer references it: now it is safe to release the
		// consumer finalizer and delete the notifications TransportURL.
		if err := object.RemoveSecretConsumerFinalizer(ctx, helper, instance.Namespace,
			instance.Status.NotificationsTransportURLSecret, heat.TransportConsumerFinalizer); err != nil {
			return ctrl.Result{}, err
		}
		notificationsTransportURLName := fmt.Sprintf("%s-heat-notifications-transport", instance.Name)
		if err := r.transportURLDeleted(ctx, instance, notificationsTransportURLName); err != nil {
			Log.Error(err, fmt.Sprintf("Could not delete notification TransportURL %s", notificationsTransportURLName))
			return ctrl.Result{}, err
		}
		instance.Status.NotificationsTransportURLSecret = ""
	}

	// Self-heal consumer finalizers stranded on secrets superseded during
	// rapid rotation (A -> B -> C before the workload became ready):
	// FinalizeSecretRotation only ever releases the single tracked "old"
	// secret, so any intermediate secret's finalizer would otherwise leak.
	// keep enumerates every secret that legitimately still holds the
	// finalizer; all others in the namespace are pruned.
	currentNotifKeep := ""
	if notificationsTransportURL != nil {
		currentNotifKeep = notificationsTransportURL.Status.SecretName
	}
	if err := object.PruneSecretConsumerFinalizers(
		ctx, helper, instance.Namespace, heat.TransportConsumerFinalizer,
		instance.Status.TransportURLSecret, transportURL.Status.SecretName,
		instance.Status.NotificationsTransportURLSecret, currentNotifKeep,
	); err != nil {
		return ctrl.Result{}, err
	}
	if err := object.PruneSecretConsumerFinalizers(
		ctx, helper, instance.Namespace, heat.ACConsumerFinalizer,
		instance.Status.ApplicationCredentialSecret,
		instance.Spec.Auth.ApplicationCredentialSecret,
	); err != nil {
		return ctrl.Result{}, err
	}

	Log.Info("Reconciled Heat successfully")

	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileInit(ctx context.Context,
	instance *heatv1beta1.Heat,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Heat init")

	//
	// run Heat db sync
	//
	dbSyncHash := instance.Status.Hash[heatv1beta1.DbSyncHash]

	jobDef := heat.DBSyncJob(instance, serviceLabels)

	dbSyncjob := job.NewJob(
		jobDef,
		heatv1beta1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Second*10,
		dbSyncHash,
	)
	ctrlResult, err := dbSyncjob.DoJob(
		ctx,
		helper,
	)

	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBSyncReadyRunningMessage))
		return ctrlResult, nil
	}
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBSyncReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBSyncReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	if dbSyncjob.HasChanged() {
		instance.Status.Hash[heatv1beta1.DbSyncHash] = dbSyncjob.GetHash()
		Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[heatv1beta1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run heat db sync - end

	Log.Info("Reconciled Heat init successfully")
	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileUpdate(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Heat update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Heat update successfully")
	return ctrl.Result{}, nil
}

func (r *HeatReconciler) apiDeploymentCreateOrUpdate(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	memcached *memcachedv1.Memcached,
	transportURLSecret string,
	expectedInputHash string,
) (*heatv1beta1.HeatAPI, controllerutil.OperationResult, error) {
	heatAPISpec := heatv1beta1.HeatAPISpec{
		HeatTemplate:       instance.Spec.HeatTemplate,
		HeatAPITemplate:    instance.Spec.HeatAPI,
		DatabaseHostname:   instance.Status.DatabaseHostname,
		TransportURLSecret: transportURLSecret,
		ServiceAccount:     instance.RbacResourceName(),
	}

	deployment := &heatv1beta1.HeatAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	if heatAPISpec.NodeSelector == nil {
		heatAPISpec.NodeSelector = instance.Spec.NodeSelector
	}

	// If topology is not present in the underlying HeatAPISpec,
	// inherit from the top-level CR
	if heatAPISpec.TopologyRef == nil {
		heatAPISpec.TopologyRef = instance.Spec.TopologyRef
	}

	if memcached.GetMemcachedMTLSSecret() != "" {
		heatAPISpec.MemcachedInstance = &instance.Spec.MemcachedInstance
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		if heatAPISpec.MemcachedInstance == nil {
			heatAPISpec.MemcachedInstance = deployment.Spec.MemcachedInstance
		}
		deployment.Spec = heatAPISpec
		if deployment.Annotations == nil {
			deployment.Annotations = map[string]string{}
		}
		deployment.Annotations["openstack.org/input-secret-hash"] = expectedInputHash
		return controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	})

	return deployment, op, err
}

func (r *HeatReconciler) cfnapiDeploymentCreateOrUpdate(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	memcached *memcachedv1.Memcached,
	transportURLSecret string,
	expectedInputHash string,
) (*heatv1beta1.HeatCfnAPI, controllerutil.OperationResult, error) {
	heatCfnAPISpec := heatv1beta1.HeatCfnAPISpec{
		HeatTemplate:       instance.Spec.HeatTemplate,
		HeatCfnAPITemplate: instance.Spec.HeatCfnAPI,
		DatabaseHostname:   instance.Status.DatabaseHostname,
		TransportURLSecret: transportURLSecret,
		ServiceAccount:     instance.RbacResourceName(),
	}

	if heatCfnAPISpec.NodeSelector == nil {
		heatCfnAPISpec.NodeSelector = instance.Spec.NodeSelector
	}

	// If topology is not present in the underlying HeatCfnAPISpec,
	// inherit from the top-level CR
	if heatCfnAPISpec.TopologyRef == nil {
		heatCfnAPISpec.TopologyRef = instance.Spec.TopologyRef
	}

	if memcached.GetMemcachedMTLSSecret() != "" {
		heatCfnAPISpec.MemcachedInstance = &instance.Spec.MemcachedInstance
	}

	deployment := &heatv1beta1.HeatCfnAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cfnapi", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		if heatCfnAPISpec.MemcachedInstance == nil {
			heatCfnAPISpec.MemcachedInstance = deployment.Spec.MemcachedInstance
		}
		deployment.Spec = heatCfnAPISpec
		if deployment.Annotations == nil {
			deployment.Annotations = map[string]string{}
		}
		deployment.Annotations["openstack.org/input-secret-hash"] = expectedInputHash
		return controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	})

	return deployment, op, err
}

func (r *HeatReconciler) engineDeploymentCreateOrUpdate(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	memcached *memcachedv1.Memcached,
	transportURLSecret string,
	expectedInputHash string,
) (*heatv1beta1.HeatEngine, controllerutil.OperationResult, error) {
	heatEngineSpec := heatv1beta1.HeatEngineSpec{
		HeatTemplate:       instance.Spec.HeatTemplate,
		HeatEngineTemplate: instance.Spec.HeatEngine,
		DatabaseHostname:   instance.Status.DatabaseHostname,
		TransportURLSecret: transportURLSecret,
		ServiceAccount:     instance.RbacResourceName(),
		TLS:                instance.Spec.HeatAPI.TLS.Ca,
	}

	if heatEngineSpec.NodeSelector == nil {
		heatEngineSpec.NodeSelector = instance.Spec.NodeSelector
	}

	// If topology is not present in the underlying HeatEngineSpec
	// inherit from the top-level CR
	if heatEngineSpec.TopologyRef == nil {
		heatEngineSpec.TopologyRef = instance.Spec.TopologyRef
	}

	if memcached.GetMemcachedMTLSSecret() != "" {
		heatEngineSpec.MemcachedInstance = &instance.Spec.MemcachedInstance
	}

	deployment := &heatv1beta1.HeatEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-engine", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		if heatEngineSpec.MemcachedInstance == nil {
			heatEngineSpec.MemcachedInstance = deployment.Spec.MemcachedInstance
		}
		deployment.Spec = heatEngineSpec
		if deployment.Annotations == nil {
			deployment.Annotations = map[string]string{}
		}
		deployment.Annotations["openstack.org/input-secret-hash"] = expectedInputHash
		return controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	})

	return deployment, op, err
}

// generateServiceSecrets - create secrets which hold service configuration
func (r *HeatReconciler) generateServiceSecrets(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	h *helper.Helper,
	envVars *map[string]env.Setter,
	mc *memcachedv1.Memcached,
	db *mariadbv1.Database,
	transportURLSecretName string,
	notificationsTransportURLSecretName string,
) error {
	//
	// create Secret required for heat input
	// - %-config secret holding minimal heat config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	secretLabels := labels.GetLabels(instance, labels.GetGroupLabel(heat.ServiceName), map[string]string{})

	var tlsCfg *tls.Service
	if instance.Spec.HeatAPI.TLS.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	customData := generateCustomData(instance, tlsCfg, db)

	customSecrets := ""
	for _, secretName := range instance.Spec.CustomServiceConfigSecrets {
		secret, _, err := oko_secret.GetSecret(ctx, h, secretName, instance.Namespace)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.InputReadyErrorMessage,
				err.Error()))
			return err
		}
		for _, data := range secret.Data {
			customSecrets += string(data) + "\n"
		}
	}
	customData[heat.CustomConfigSecretsFileName] = customSecrets

	var err error
	keystoneAPI, err = keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return err
	}

	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointInternal)
	if err != nil {
		return err
	}

	ospSecret, _, err := oko_secret.GetSecret(ctx, h, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		return err
	}
	password := strings.TrimSuffix(string(ospSecret.Data[instance.Spec.PasswordSelectors.Service]), "\n")

	domainAdminPassword := password
	val, ok := ospSecret.Data[instance.Spec.PasswordSelectors.StackDomainAdminPassword]
	if ok {
		domainAdminPassword = strings.TrimSuffix(string(val), "\n")
	}

	authEncryptionKey, err := validateAuthEncryptionKey(instance, ospSecret)
	if err != nil {
		return err
	}

	transportURLSecret, _, err := oko_secret.GetSecret(ctx, h, transportURLSecretName, instance.Namespace)
	if err != nil {
		return err
	}
	transportURL := strings.TrimSuffix(string(transportURLSecret.Data["transport_url"]), "\n")
	quorumQueues := strings.TrimSuffix(string(transportURLSecret.Data["quorumqueues"]), "\n") == "true"

	// Get notifications transport URL if configured
	var notificationsTransportURL string
	if notificationsTransportURLSecretName != "" {
		notificationsTransportURLSecret, _, err := oko_secret.GetSecret(ctx, h, notificationsTransportURLSecretName, instance.Namespace)
		if err != nil {
			return err
		}
		notificationsTransportURL = strings.TrimSuffix(string(notificationsTransportURLSecret.Data["transport_url"]), "\n")
	}

	databaseAccount := db.GetAccount()
	dbSecret := db.GetSecret()

	templateParameters := initTemplateParameters(instance, authURL, password, domainAdminPassword, authEncryptionKey, transportURL, notificationsTransportURL, mc, databaseAccount, dbSecret, quorumQueues)

	// Render vhost configuration for API and CFN
	httpdAPIVhostConfig := map[string]any{}
	httpdCfnAPIVhostConfig := map[string]any{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		var (
			apiTLSEnabled    = instance.Spec.HeatAPI.TLS.API.Enabled(endpt)
			cfnAPITLSEnabled = instance.Spec.HeatCfnAPI.TLS.API.Enabled(endpt)
		)
		renderVhost(httpdAPIVhostConfig, instance, endpt, heatapi.ServiceName, apiTLSEnabled)
		renderVhost(httpdCfnAPIVhostConfig, instance, endpt, heatcfnapi.ServiceName, cfnAPITLSEnabled)
	}

	// create HeatAPI httpd vhost template parameters
	templateParameters["APIvHosts"] = httpdAPIVhostConfig
	templateParameters["CfnAPIvHosts"] = httpdCfnAPIVhostConfig

	// MTLS
	if mc.GetMemcachedMTLSSecret() != "" {
		templateParameters["MemcachedAuthCert"] = fmt.Sprint(memcachedv1.CertMountPath())
		templateParameters["MemcachedAuthKey"] = fmt.Sprint(memcachedv1.KeyMountPath())
		templateParameters["MemcachedAuthCa"] = fmt.Sprint(memcachedv1.CaMountPath())
	}

	// Retrieve Application Credential data from Heat Auth section if specified
	Log := r.GetLogger(ctx)
	if instance.Spec.Auth.ApplicationCredentialSecret != "" {
		acSecret := &corev1.Secret{}
		key := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.Auth.ApplicationCredentialSecret}
		if err := h.GetClient().Get(ctx, key, acSecret); err != nil {
			Log.Error(err, "Failed to get ApplicationCredential secret", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
			return err
		}
		acID, okID := acSecret.Data[keystonev1.ACIDSecretKey]
		acSecretData, okSecret := acSecret.Data[keystonev1.ACSecretSecretKey]
		if okID && len(acID) > 0 && okSecret && len(acSecretData) > 0 {
			templateParameters["ApplicationCredentialID"] = string(acID)
			templateParameters["ApplicationCredentialSecret"] = string(acSecretData)
			Log.Info("Using ApplicationCredentials auth from Heat spec", "secret", instance.Spec.Auth.ApplicationCredentialSecret)
		} else {
			return fmt.Errorf("%w: %s", ErrACSecretMissingKeys, instance.Spec.Auth.ApplicationCredentialSecret)
		}
	}

	secrets := createSecretTemplates(instance, customData, templateParameters, secretLabels)
	return oko_secret.EnsureSecrets(ctx, h, instance, secrets, envVars)
}

func (r *HeatReconciler) reconcileUpgrade(ctx context.Context) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	Log.Info("Reconciling Heat upgrade")

	// TODO(bshephar): should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	Log.Info("Reconciled Heat upgrade successfully")
	return ctrl.Result{}, nil
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
func (r *HeatReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	envVars map[string]env.Setter,
) (string, error) {
	Log := r.GetLogger(ctx)
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, nil
}

func (r *HeatReconciler) transportURLCreateOrUpdate(
	instance *heatv1beta1.Heat,
	serviceLabels map[string]string,
	rabbitMqConfig rabbitmqv1.RabbitMqConfig,
) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-heat-transport", instance.Name),
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = rabbitMqConfig.Cluster

		// Always set Username and Vhost to allow clearing/resetting them
		// The infra-operator TransportURL controller handles empty values:
		// - Empty Username: uses default cluster admin credentials
		// - Empty Vhost: defaults to "/" vhost
		transportURL.Spec.Username = rabbitMqConfig.User
		transportURL.Spec.Vhost = rabbitMqConfig.Vhost

		return controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
	})

	return transportURL, op, err
}

func (r *HeatReconciler) notificationsTransportURLCreateOrUpdate(
	instance *heatv1beta1.Heat,
	serviceLabels map[string]string,
	rabbitMqConfig rabbitmqv1.RabbitMqConfig,
) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-heat-notifications-transport", instance.Name),
			Namespace: instance.Namespace,
			Labels:    serviceLabels,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = rabbitMqConfig.Cluster

		// Always set Username and Vhost to allow clearing/resetting them
		// The infra-operator TransportURL controller handles empty values:
		// - Empty Username: uses default cluster admin credentials
		// - Empty Vhost: defaults to "/" vhost
		transportURL.Spec.Username = rabbitMqConfig.User
		transportURL.Spec.Vhost = rabbitMqConfig.Vhost

		return controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
	})

	return transportURL, op, err
}

// transportURLDeleted deletes the named TransportURL CR, treating an
// already-absent object as success.
func (r *HeatReconciler) transportURLDeleted(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	transportURLName string,
) error {
	Log := r.GetLogger(ctx)
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      transportURLName,
			Namespace: instance.Namespace,
		},
	}

	err := r.Delete(ctx, transportURL)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return nil
		}
		Log.Info(fmt.Sprintf("Could not delete TransportURL %s err: %s", transportURLName, err))
		return err
	}

	Log.Info("Deleted transportURL", ":", transportURLName)

	return nil
}

// ensureStackDomain creates the OpenStack domain for Heat stacks. It then assigns the user to the Heat stacks domain.
// This function relies on the keystoneAPI variable that is set globally in generateServiceSecrets().
func (r *HeatReconciler) ensureStackDomain(
	ctx context.Context,
	helper *helper.Helper,
	instance *heatv1beta1.Heat,
	secret *corev1.Secret,
) (ctrl.Result, error) {
	Log := r.GetLogger(ctx)
	val, ok := secret.Data[instance.Spec.PasswordSelectors.Service]
	if !ok {
		return ctrl.Result{}, fmt.Errorf("%w: %s not found in secret %s", ErrPasswordSelectorNotFound, instance.Spec.PasswordSelectors.Service, instance.Spec.Secret)
	}
	password := strings.TrimSuffix(string(val), "\n")

	domainAdminPassword := password
	val, ok = secret.Data[instance.Spec.PasswordSelectors.StackDomainAdminPassword]
	if ok {
		domainAdminPassword = strings.TrimSuffix(string(val), "\n")
	}
	//
	// get admin authentication OpenStack
	//
	os, ctrlResult, err := keystonev1.GetAdminServiceClient(
		ctx,
		helper,
		keystoneAPI,
	)
	if err != nil || (ctrlResult != ctrl.Result{}) {
		return ctrlResult, err
	}

	// create domain
	domain := openstack.Domain{
		Name:        heat.StackDomainName,
		Description: "Domain for Heat stacks",
	}
	domainID, err := os.CreateDomain(ctx, Log, domain)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create heat_stack_user role as per:
	// https://docs.openstack.org/heat/2023.2/admin/stack-domain-users.html#usage-workflow
	_, err = os.CreateRole(ctx, Log, heat.HeatStackUserRole)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create Heat user
	userID, err := os.CreateUser(
		ctx,
		Log,
		openstack.User{
			Name:     heat.StackDomainAdminUsername,
			Password: domainAdminPassword,
			DomainID: domainID,
		})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Add the user to the domain
	err = os.AssignUserDomainRole(
		ctx,
		Log,
		"admin",
		userID,
		domainID)
	return ctrl.Result{}, err
}

func (r *HeatReconciler) ensureDB(
	ctx context.Context,
	h *helper.Helper,
	instance *heatv1beta1.Heat,
) (*mariadbv1.Database, ctrl.Result, error) {
	// ensure MariaDBAccount exists.  This account record may be created by
	// openstack-operator or the cloud operator up front without a specific
	// MariaDBDatabase configured yet.   Otherwise, a MariaDBAccount CR is
	// created here with a generated username as well as a secret with
	// generated password.   The MariaDBAccount is created without being
	// yet associated with any MariaDBDatabase.
	_, _, err := mariadbv1.EnsureMariaDBAccount(
		ctx, h, instance.Spec.DatabaseAccount,
		instance.Namespace, false, heat.DatabaseUsernamePrefix,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			mariadbv1.MariaDBAccountReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			mariadbv1.MariaDBAccountNotReadyMessage,
			err.Error()))

		return nil, ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(
		mariadbv1.MariaDBAccountReadyCondition,
		mariadbv1.MariaDBAccountReadyMessage,
	)

	//
	// create service DB instance
	//
	db := mariadbv1.NewDatabaseForAccount(
		instance.Spec.DatabaseInstance, // mariadb/galera service to target
		heat.DatabaseName,              // name used in CREATE DATABASE in mariadb
		heat.DatabaseCRName,            // CR name for MariaDBDatabase
		instance.Spec.DatabaseAccount,  // CR name for MariaDBAccount
		instance.Namespace,             // namespace
	)

	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchAll(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}
	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreated(ctx, h)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return db, ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return db, ctrlResult, nil
	}

	// update Status.DatabaseHostname, used to config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)
	return db, ctrlResult, nil
}

// verifyStatusConditions - Check to see if we have existing conditions.
// Return empty condition.Conditions{} if none currently exist. Otherwise,
// return a DeepCopy of the existing Conditions. If the condition state is
// unchanged, we will use this copy to restore the LastTransitinTime.
func verifyStatusConditions(conditions condition.Conditions) (bool, condition.Conditions) {
	if conditions == nil {
		return true, condition.Conditions{}
	}

	return false, conditions.DeepCopy()
}

func generateCustomData(instance *heatv1beta1.Heat, tlsCfg *tls.Service, db *mariadbv1.Database) map[string]string {
	const myCnf string = "my.cnf"

	// customData hold any customization for the service.
	// 01-custom.conf is going to /etc/heat/heat.conf.d
	// all other files get placed into /etc/heat to allow overwrite of e.g. policy.json
	// TODO: make sure 01-custom.conf can not be overwritten
	customData := map[string]string{
		heat.CustomConfigFileName: instance.Spec.CustomServiceConfig,
		myCnf:                     db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}

	maps.Copy(customData, instance.Spec.DefaultConfigOverwrite)

	return customData
}

// createSecretsTemplates - Takes inputs and renders the templates that will be used for our Secrets
func createSecretTemplates(instance *heatv1beta1.Heat, customData map[string]string, templateParameters map[string]any, secretLabels map[string]string) []util.Template {
	var (
		secretName = fmt.Sprintf("%s-config-data", instance.Name)
	)

	return []util.Template{
		// Secret
		{
			Name:            secretName,
			Namespace:       instance.Namespace,
			Type:            util.TemplateTypeConfig,
			InstanceType:    instance.Kind,
			CustomData:      customData,
			ConfigOptions:   templateParameters,
			Labels:          secretLabels,
			CommonTemplates: []string{"ssl.conf"},
		},
	}
}

// initTemplateParameters - takes inputs related to external objects in the cluster and renders the
// initial set of parameters that we will use in the heat.conf file.
func initTemplateParameters(
	instance *heatv1beta1.Heat,
	authURL string,
	password string,
	domainAdminPassword string,
	authEncryptionKey string,
	transportURL string,
	notificationsTransportURL string,
	mc *memcachedv1.Memcached,
	databaseAccount *mariadbv1.MariaDBAccount,
	dbSecret *corev1.Secret,
	quorumQueues bool,
) map[string]any {
	mysqlConnectionString := fmt.Sprintf(
		"mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
		databaseAccount.Spec.UserName,
		string(dbSecret.Data[mariadbv1.DatabasePasswordSelector]),
		instance.Status.DatabaseHostname,
		heat.DatabaseName,
	)

	params := map[string]any{
		"KeystoneInternalURL":      authURL,
		"ServiceUser":              instance.Spec.ServiceUser,
		"ServicePassword":          password,
		"StackDomainAdminUsername": heat.StackDomainAdminUsername,
		"StackDomainName":          heat.StackDomainName,
		"StackDomainAdminPassword": domainAdminPassword,
		"AuthEncryptionKey":        authEncryptionKey,
		"TransportURL":             transportURL,
		"MemcachedServers":         mc.GetMemcachedServerListString(),
		"MemcachedServersWithInet": mc.GetMemcachedServerListWithInetString(),
		"MemcachedTLS":             mc.GetMemcachedTLSSupport(),
		"DatabaseConnection":       mysqlConnectionString,
		"Timeout":                  instance.Spec.APITimeout,
		"QuorumQueues":             quorumQueues,
	}

	// Add notifications transport URL if configured
	if notificationsTransportURL != "" {
		params["NotificationsTransportURL"] = notificationsTransportURL
	}

	return params
}

func renderVhost(httpdVhostConfig map[string]any, instance *heatv1beta1.Heat, endpt service.Endpoint, serviceName string, tlsEnabled bool) {
	var (
		ServerNameString = fmt.Sprintf("%s-%s.%s.svc", serviceName, endpt.String(), instance.Namespace)
		SSLCertFilePath  = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
		SSLKeyFilePath   = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
	)

	endptConfig := map[string]any{}
	endptConfig["ServerName"] = ServerNameString
	endptConfig["TLS"] = tlsEnabled // default TLS to false, and set it bellow to true if enabled
	if tlsEnabled {
		endptConfig["SSLCertificateFile"] = SSLCertFilePath
		endptConfig["SSLCertificateKeyFile"] = SSLKeyFilePath
	}
	httpdVhostConfig[endpt.String()] = endptConfig
}

// validateAuthEncryptionKey - the heat_auth_encrption_key needs to be 32 characters long. This function validates
// the length of the user provided key and returns an error if it isn't long enough.
func validateAuthEncryptionKey(instance *heatv1beta1.Heat, ospSecret *corev1.Secret) (string, error) {
	const HeatAuthEncKeyLen int = 32

	heatAuthEncKey := strings.TrimSuffix(string(ospSecret.Data[instance.Spec.PasswordSelectors.AuthEncryptionKey]), "\n")

	if len(heatAuthEncKey) < HeatAuthEncKeyLen {
		return "", fmt.Errorf("%w: must be at least %d characters", ErrAuthEncryptionKeyTooShort, HeatAuthEncKeyLen)
	}

	return heatAuthEncKey, nil

}

// ensureDBPurgeJob - Create the CronJob to purge soft-deleted DB records
func (r *HeatReconciler) ensureDBPurgeJob(
	ctx context.Context,
	h *helper.Helper,
	instance *heatv1beta1.Heat,
	serviceLabels map[string]string,
) (ctrl.Result, error) {

	cronSpec := heat.CronJobSpec{
		Name:     fmt.Sprintf("%s-db-purge", instance.Name),
		Command:  heat.HeatManage,
		Schedule: instance.Spec.DBPurge.Schedule,
		Labels:   serviceLabels,
	}

	cronjobDef := heat.DBPurgeJob(
		instance,
		cronSpec,
	)

	dbPurgeCronJob := cronjob.NewCronJob(
		cronjobDef,
		time.Second*5,
	)
	ctrlResult, err := dbPurgeCronJob.CreateOrPatch(ctx, h)
	if err != nil {
		return ctrlResult, err
	}

	return ctrlResult, err
}
