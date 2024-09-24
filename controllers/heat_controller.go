/*
http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"
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

	heat "github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	heatapi "github.com/openstack-k8s-operators/heat-operator/pkg/heatapi"
	heatcfnapi "github.com/openstack-k8s-operators/heat-operator/pkg/heatcfnapi"
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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// HeatReconciler reconciles a Heat object
type HeatReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
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
// +kubebuilder:rbac:groups="security.openshift.io",resourceNames=anyuid,resources=securitycontextconstraints,verbs=use
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;delete;get;list;patch;update;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *HeatReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	_ = log.FromContext(ctx)

	instance := &heatv1beta1.Heat{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
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
		r.Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	isNewInstance, savedConditions := verifyStatusConditions(instance.Status.Conditions)

	// Always patch the instance status when exiting this function so we can
	// persist any changes.
	defer func() {
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
	passwordSecretField     = ".spec.secret"
	transportURLSecretField = ".spec.transportURLSecret"
	caBundleSecretNameField = ".spec.tls.caBundleSecretName"
	tlsAPIInternalField     = ".spec.tls.api.internal.secretName"
	tlsAPIPublicField       = ".spec.tls.api.public.secretName"
)

var (
	heatWatchFields = []string{
		passwordSecretField,
	}
	heatAPIWatchFields = []string{
		passwordSecretField,
		transportURLSecretField,
		caBundleSecretNameField,
		tlsAPIInternalField,
		tlsAPIPublicField,
	}
	heatCfnWatchFields = []string{
		passwordSecretField,
		transportURLSecretField,
		caBundleSecretNameField,
		tlsAPIInternalField,
		tlsAPIPublicField,
	}
	heatEngineWatchFields = []string{
		passwordSecretField,
		transportURLSecretField,
		caBundleSecretNameField,
	}
)

// SetupWithManager sets up the controller with the Manager.
func (r *HeatReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	memcachedFn := func(_ context.Context, o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all Heat CRs
		heats := &heatv1beta1.HeatList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(context.Background(), heats, listOpts...); err != nil {
			r.Log.Error(err, "Unable to retrieve Heat CRs %w")
			return nil
		}

		for _, cr := range heats.Items {
			if o.GetName() == cr.Spec.MemcachedInstance {
				name := client.ObjectKey{
					Namespace: o.GetNamespace(),
					Name:      cr.Name,
				}
				r.Log.Info(fmt.Sprintf("Memcached %s is used by Heat CR %s", o.GetName(), cr.Name))
				result = append(result, reconcile.Request{NamespacedName: name})
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&heatv1beta1.Heat{}).
		Owns(&heatv1beta1.HeatAPI{}).
		Owns(&heatv1beta1.HeatCfnAPI{}).
		Owns(&heatv1beta1.HeatEngine{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&mariadbv1.MariaDBAccount{}).
		Owns(&batchv1.Job{}).
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
		Complete(r)
}

func (r *HeatReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("Heat")

	for _, field := range heatWatchFields {
		crList := &heatv1beta1.HeatList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(field, src.GetName()),
			Namespace:     src.GetNamespace(),
		}
		err := r.List(ctx, crList, listOps)
		if err != nil {
			return []reconcile.Request{}
		}

		for _, item := range crList.Items {
			l.Info(fmt.Sprintf("input source %s changed, reconcile: %s - %s", src.GetName(), item.GetName(), item.GetNamespace()))

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

func (r *HeatReconciler) reconcileDelete(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Heat delete")

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

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Heat delete successfully")

	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileNormal(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// Service account, role, binding
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups:     []string{"security.openshift.io"},
			ResourceNames: []string{"anyuid"},
			Resources:     []string{"securitycontextconstraints"},
			Verbs:         []string{"use"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
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
	ospSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			r.Log.Info(fmt.Sprintf("OpenStack secret %s not found", instance.Spec.Secret))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	secretVars[ospSecret.Name] = env.SetValue(hash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// run check OpenStack secret - end

	//
	// Check for required memcached used for caching
	//
	memcached, err := memcachedv1.GetMemcachedByName(ctx, helper, instance.Spec.MemcachedInstance, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			r.Log.Info(fmt.Sprintf("memcached %s not found", instance.Spec.MemcachedInstance))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.MemcachedReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
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
		r.Log.Info(fmt.Sprintf("memcached %s is not ready", memcached.Name))
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
	transportURL, op, err := r.transportURLCreateOrUpdate(instance)
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
		r.Log.Info(fmt.Sprintf("TransportURL %s successfully reconciled - operation: %s", transportURL.Name, string(op)))
	}

	instance.Status.TransportURLSecret = transportURL.Status.SecretName

	if instance.Status.TransportURLSecret == "" {
		r.Log.Info(fmt.Sprintf("Waiting for TransportURL %s secret to be created", transportURL.Name))

		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.RabbitMqTransportURLReadyRunningMessage))

		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	//
	// check for required TransportURL secret holding transport URL string
	//

	transportURLSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			r.Log.Info(fmt.Sprintf("TransportURL secret %s not found", instance.Status.TransportURLSecret))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.RabbitMqTransportURLReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.RabbitMqTransportURLReadyRunningMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.RabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.RabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	secretVars[transportURLSecret.Name] = env.SetValue(hash)

	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(condition.RabbitMqTransportURLReadyCondition, condition.RabbitMqTransportURLReadyMessage)

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
	// - %-scripts secret holding scripts to e.g. bootstrap the service
	// - %-config secret holding minimal heat config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceSecrets(ctx, instance, helper, &secretVars, memcached, db)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	_, err = r.createHashOfInputHashes(instance, secretVars)
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

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: heat.ServiceName,
	}

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil || (ctrlResult != ctrl.Result{}) {
		return ctrlResult, err
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate()
	if err != nil || (ctrlResult != ctrl.Result{}) {
		return ctrlResult, err
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade()
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

	//
	// normal reconcile tasks
	//

	// Create domain for Heat stacks
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

	// deploy heat-engine
	heatEngine, op, err := r.engineDeploymentCreateOrUpdate(ctx, instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatEngineReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			err.Error()))
		return ctrl.Result{}, err
	}

	// Check the observed Generation and mirror the condition from the
	// underlying resource reconciliation
	ngObsGen, err := r.checkHeatEngineGeneration(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatEngineReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Only mirror the underlying condition if the observedGeneration is
	// the last seen
	if !ngObsGen {
		instance.Status.Conditions.Set(condition.UnknownCondition(
			heatv1beta1.HeatEngineReadyCondition,
			condition.InitReason,
			heatv1beta1.HeatEngineReadyInitMessage,
		))
	} else {
		// Mirror HeatEngine status' ReadyCount to this parent CR
		instance.Status.HeatEngineReadyCount = heatEngine.Status.ReadyCount

		// Mirror HeatEngine's condition status
		c := heatEngine.Status.Conditions.Mirror(heatv1beta1.HeatEngineReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		if op != controllerutil.OperationResultNone {
			r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		}
	}

	// deploy heat-api
	heatAPI, op, err := r.apiDeploymentCreateOrUpdate(ctx, instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	// Check the observed Generation and mirror the condition from the
	// underlying resource reconciliation
	apiObsGen, err := r.checkHeatAPIGeneration(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Only mirror the underlying condition if the observedGeneration is
	// the last seen
	if !apiObsGen {
		instance.Status.Conditions.Set(condition.UnknownCondition(
			heatv1beta1.HeatAPIReadyCondition,
			condition.InitReason,
			heatv1beta1.HeatAPIReadyInitMessage,
		))
	} else {
		// Mirror HeatAPI status' ReadyCount to this parent CR
		instance.Status.HeatAPIReadyCount = heatAPI.Status.ReadyCount

		// Mirror HeatAPI's condition status
		c := heatAPI.Status.Conditions.Mirror(heatv1beta1.HeatAPIReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}

		if op != controllerutil.OperationResultNone {
			r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		}
	}

	// deploy heat-api-cfn
	heatCfnAPI, op, err := r.cfnapiDeploymentCreateOrUpdate(ctx, instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatCfnAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Check the observed Generation and mirror the condition from the
	// underlying resource reconciliation
	cfnObsGen, err := r.checkHeatCfnGeneration(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatCfnAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	// Only mirror the underlying condition if the observedGeneration is
	// the last seen
	if !cfnObsGen {
		instance.Status.Conditions.Set(condition.UnknownCondition(
			heatv1beta1.HeatCfnAPIReadyCondition,
			condition.InitReason,
			heatv1beta1.HeatCfnAPIReadyInitMessage,
		))
	} else {
		// Mirror HeatCfnAPI status' ReadyCount to this parent CR
		instance.Status.HeatCfnAPIReadyCount = heatCfnAPI.Status.ReadyCount
		// Mirror HeatCfnAPI's condition status
		c := heatCfnAPI.Status.Conditions.Mirror(heatv1beta1.HeatCfnAPIReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
		if op != controllerutil.OperationResultNone {
			r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
		}
	}
	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}
	r.Log.Info("Reconciled Heat successfully")

	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileInit(ctx context.Context,
	instance *heatv1beta1.Heat,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Heat init")

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
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[heatv1beta1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run heat db sync - end

	r.Log.Info("Reconciled Heat init successfully")
	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileUpdate() (ctrl.Result, error) {
	r.Log.Info("Reconciling Heat update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Heat update successfully")
	return ctrl.Result{}, nil
}

func (r *HeatReconciler) apiDeploymentCreateOrUpdate(
	ctx context.Context,
	instance *heatv1beta1.Heat,
) (*heatv1beta1.HeatAPI, controllerutil.OperationResult, error) {
	heatAPISpec := heatv1beta1.HeatAPISpec{
		HeatTemplate:       instance.Spec.HeatTemplate,
		HeatAPITemplate:    instance.Spec.HeatAPI,
		DatabaseHostname:   instance.Status.DatabaseHostname,
		TransportURLSecret: instance.Status.TransportURLSecret,
		ServiceAccount:     instance.RbacResourceName(),
	}

	deployment := &heatv1beta1.HeatAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Spec = heatAPISpec
		if len(deployment.Spec.NodeSelector) == 0 {
			deployment.Spec.NodeSelector = instance.Spec.NodeSelector
		}
		return controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	})

	return deployment, op, err
}

func (r *HeatReconciler) cfnapiDeploymentCreateOrUpdate(
	ctx context.Context,
	instance *heatv1beta1.Heat,
) (*heatv1beta1.HeatCfnAPI, controllerutil.OperationResult, error) {
	heatCfnAPISpec := heatv1beta1.HeatCfnAPISpec{
		HeatTemplate:       instance.Spec.HeatTemplate,
		HeatCfnAPITemplate: instance.Spec.HeatCfnAPI,
		DatabaseHostname:   instance.Status.DatabaseHostname,
		TransportURLSecret: instance.Status.TransportURLSecret,
		ServiceAccount:     instance.RbacResourceName(),
	}

	deployment := &heatv1beta1.HeatCfnAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cfnapi", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Spec = heatCfnAPISpec
		if len(deployment.Spec.NodeSelector) == 0 {
			deployment.Spec.NodeSelector = instance.Spec.NodeSelector
		}
		return controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	})

	return deployment, op, err
}

func (r *HeatReconciler) engineDeploymentCreateOrUpdate(
	ctx context.Context,
	instance *heatv1beta1.Heat,
) (*heatv1beta1.HeatEngine, controllerutil.OperationResult, error) {
	heatEngineSpec := heatv1beta1.HeatEngineSpec{
		HeatTemplate:       instance.Spec.HeatTemplate,
		HeatEngineTemplate: instance.Spec.HeatEngine,
		DatabaseHostname:   instance.Status.DatabaseHostname,
		TransportURLSecret: instance.Status.TransportURLSecret,
		ServiceAccount:     instance.RbacResourceName(),
		TLS:                instance.Spec.HeatAPI.TLS.Ca,
	}

	deployment := &heatv1beta1.HeatEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-engine", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		deployment.Spec = heatEngineSpec
		if len(deployment.Spec.NodeSelector) == 0 {
			deployment.Spec.NodeSelector = instance.Spec.NodeSelector
		}
		return controllerutil.SetControllerReference(instance, deployment, r.Scheme)
	})

	return deployment, op, err
}

// generateServiceSecrets - create create secrets which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
func (r *HeatReconciler) generateServiceSecrets(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	h *helper.Helper,
	envVars *map[string]env.Setter,
	mc *memcachedv1.Memcached,
	db *mariadbv1.Database,
) error {
	//
	// create Secret required for heat input
	// - %-scripts secret holding scripts to e.g. bootstrap the service
	// - %-config secret holding minimal heat config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	secretLabels := labels.GetLabels(instance, labels.GetGroupLabel(heat.ServiceName), map[string]string{})

	var tlsCfg *tls.Service
	if instance.Spec.HeatAPI.TLS.Ca.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	// customData hold any customization for the service.
	// custom.conf is going to /etc/heat/heat.conf.d
	// all other files get placed into /etc/heat to allow overwrite of e.g. policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{
		common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig,
		"my.cnf":                           db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

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
	authEncryptionKey := strings.TrimSuffix(string(ospSecret.Data[instance.Spec.PasswordSelectors.AuthEncryptionKey]), "\n")

	transportURLSecret, _, err := oko_secret.GetSecret(ctx, h, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		return err
	}
	transportURL := strings.TrimSuffix(string(transportURLSecret.Data["transport_url"]), "\n")

	databaseAccount := db.GetAccount()
	dbSecret := db.GetSecret()

	templateParameters := map[string]interface{}{
		"KeystoneInternalURL":      authURL,
		"ServiceUser":              instance.Spec.ServiceUser,
		"ServicePassword":          password,
		"StackDomainAdminUsername": heat.StackDomainAdminUsername,
		"StackDomainName":          heat.StackDomainName,
		"AuthEncryptionKey":        authEncryptionKey,
		"TransportURL":             transportURL,
		"MemcachedServers":         mc.GetMemcachedServerListString(),
		"MemcachedServersWithInet": mc.GetMemcachedServerListWithInetString(),
		"MemcachedTLS":             mc.GetMemcachedTLSSupport(),
		"DatabaseConnection": fmt.Sprintf("mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
			databaseAccount.Spec.UserName,
			string(dbSecret.Data[mariadbv1.DatabasePasswordSelector]),
			instance.Status.DatabaseHostname,
			heat.DatabaseName,
		),
	}

	// create HeatAPI httpd vhost template parameters
	httpdVhostConfig := map[string]interface{}{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]interface{}{}
		endptConfig["ServerName"] = fmt.Sprintf("%s-%s.%s.svc", heatapi.ServiceName, endpt.String(), instance.Namespace)
		endptConfig["TLS"] = false // default TLS to false, and set it bellow to true if enabled
		if instance.Spec.HeatAPI.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}
		httpdVhostConfig[endpt.String()] = endptConfig
	}
	templateParameters["APIvHosts"] = httpdVhostConfig

	// create HeatCfnAPI httpd vhost template parameters
	httpdVhostConfig = map[string]interface{}{}
	for _, endpt := range []service.Endpoint{service.EndpointInternal, service.EndpointPublic} {
		endptConfig := map[string]interface{}{}
		endptConfig["ServerName"] = fmt.Sprintf("%s-%s.%s.svc", heatcfnapi.ServiceName, endpt.String(), instance.Namespace)
		endptConfig["TLS"] = false // default TLS to false, and set it bellow to true if enabled
		if instance.Spec.HeatCfnAPI.TLS.API.Enabled(endpt) {
			endptConfig["TLS"] = true
			endptConfig["SSLCertificateFile"] = fmt.Sprintf("/etc/pki/tls/certs/%s.crt", endpt.String())
			endptConfig["SSLCertificateKeyFile"] = fmt.Sprintf("/etc/pki/tls/private/%s.key", endpt.String())
		}
		httpdVhostConfig[endpt.String()] = endptConfig
	}
	templateParameters["CfnAPIvHosts"] = httpdVhostConfig

	secrets := []util.Template{
		// ScriptsSecret
		{
			Name:               fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeScripts,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{"common.sh": "/common/common.sh"},
			Labels:             secretLabels,
		},
		// Secret
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			ConfigOptions: templateParameters,
			Labels:        secretLabels,
		},
	}
	return oko_secret.EnsureSecrets(ctx, h, instance, secrets, envVars)
}

func (r *HeatReconciler) reconcileUpgrade() (ctrl.Result, error) {
	r.Log.Info("Reconciling Heat upgrade")

	// TODO(bshephar): should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Heat upgrade successfully")
	return ctrl.Result{}, nil
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
func (r *HeatReconciler) createHashOfInputHashes(
	instance *heatv1beta1.Heat,
	envVars map[string]env.Setter,
) (string, error) {
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, nil
}

func (r *HeatReconciler) transportURLCreateOrUpdate(instance *heatv1beta1.Heat) (*rabbitmqv1.TransportURL, controllerutil.OperationResult, error) {
	transportURL := &rabbitmqv1.TransportURL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-heat-transport", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, transportURL, func() error {
		transportURL.Spec.RabbitmqClusterName = instance.Spec.RabbitMqClusterName

		return controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
	})

	return transportURL, op, err
}

// ensureStackDomain creates the OpenStack domain for Heat stacks. It then assigns the user to the Heat stacks domain.
// This function relies on the keystoneAPI variable that is set globally in generateServiceSecrets().
func (r *HeatReconciler) ensureStackDomain(
	ctx context.Context,
	helper *helper.Helper,
	instance *heatv1beta1.Heat,
	secret *corev1.Secret,
) (ctrl.Result, error) {
	val, ok := secret.Data[instance.Spec.PasswordSelectors.Service]
	if !ok {
		return ctrl.Result{}, fmt.Errorf("%s not found in secret %s", instance.Spec.PasswordSelectors.Service, instance.Spec.Secret)
	}
	password := strings.TrimSuffix(string(val), "\n")

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
	domainID, err := os.CreateDomain(r.Log, domain)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create heat_stack_user role as per:
	// https://docs.openstack.org/heat/2023.2/admin/stack-domain-users.html#usage-workflow
	_, err = os.CreateRole(r.Log, heat.HeatStackUserRole)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create Heat user
	userID, err := os.CreateUser(
		r.Log,
		openstack.User{
			Name:      heat.StackDomainAdminUsername,
			Password:  password,
			ProjectID: "service",
			DomainID:  domainID,
		})
	if err != nil {
		return ctrl.Result{}, err
	}

	// Add the user to the domain
	err = os.AssignUserDomainRole(
		r.Log,
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

// checkHeatAPIGeneration -
func (r *HeatReconciler) checkHeatAPIGeneration(
	instance *heatv1beta1.Heat,
) (bool, error) {
	api := &heatv1beta1.HeatAPIList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(context.Background(), api, listOpts...); err != nil {
		r.Log.Error(err, "Unable to retrieve HeatAPI CR %w")
		return false, err
	}
	for _, item := range api.Items {
		if item.Generation != item.Status.ObservedGeneration {
			return false, nil
		}
	}
	return true, nil
}

// checkHeatEngineGeneration -
func (r *HeatReconciler) checkHeatEngineGeneration(
	instance *heatv1beta1.Heat,
) (bool, error) {
	ng := &heatv1beta1.HeatEngineList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(context.Background(), ng, listOpts...); err != nil {
		r.Log.Error(err, "Unable to retrieve HeatEngine CR %w")
		return false, err
	}
	for _, item := range ng.Items {
		if item.Generation != item.Status.ObservedGeneration {
			return false, nil
		}
	}
	return true, nil
}

// checkHeatCfnGeneration -
func (r *HeatReconciler) checkHeatCfnGeneration(
	instance *heatv1beta1.Heat,
) (bool, error) {
	cf := &heatv1beta1.HeatCfnAPIList{}
	listOpts := []client.ListOption{
		client.InNamespace(instance.Namespace),
	}
	if err := r.Client.List(context.Background(), cf, listOpts...); err != nil {
		r.Log.Error(err, "Unable to retrieve HeatCfnApi CR %w")
		return false, err
	}
	for _, item := range cf.Items {
		if item.Generation != item.Status.ObservedGeneration {
			return false, nil
		}
	}
	return true, nil
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
