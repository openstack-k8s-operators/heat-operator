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
	"time"

	"github.com/go-logr/logr"

	"github.com/openstack-k8s-operators/lib-common/modules/common"
	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	configmap "github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/job"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	heat "github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	common_rbac "github.com/openstack-k8s-operators/lib-common/modules/common/rbac"
	oko_secret "github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	database "github.com/openstack-k8s-operators/lib-common/modules/database"
	"github.com/openstack-k8s-operators/lib-common/modules/openstack"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
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
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heats/finalizers,verbs=update
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapiss,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapiss/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapiss/finalizers,verbs=update
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatengines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatengines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatengines/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;

// service account, role, rolebinding
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=get;list;watch;create;update
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

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// update the Ready condition based on the sub conditions
		if instance.Status.Conditions.AllSubConditionIsTrue() {
			instance.Status.Conditions.MarkTrue(
				condition.ReadyCondition, condition.ReadyMessage)
		} else {
			// something is not ready so reset the Ready condition
			instance.Status.Conditions.MarkUnknown(
				condition.ReadyCondition, condition.InitReason, condition.ReadyInitMessage)
			// and recalculate it based on the state of the rest of the conditions
			instance.Status.Conditions.Set(
				instance.Status.Conditions.Mirror(condition.ReadyCondition))
		}

		err := helper.PatchInstance(ctx, instance)
		if err != nil {
			_err = err
			return
		}
	}()

	// If we're not deleting this and the service object doesn't have our finalizer, add it.
	if instance.DeletionTimestamp.IsZero() && controllerutil.AddFinalizer(instance, helper.GetFinalizer()) {
		return ctrl.Result{}, nil
	}

	//
	// initialize status
	//
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}

		cl := condition.CreateList(
			condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
			condition.UnknownCondition(condition.DBSyncReadyCondition, condition.InitReason, condition.DBSyncReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(heatv1beta1.HeatStackDomainReadyCondition, condition.InitReason, heatv1beta1.HeatStackDomainReadyInitMessage),
			condition.UnknownCondition(heatv1beta1.HeatAPIReadyCondition, condition.InitReason, heatv1beta1.HeatAPIReadyInitMessage),
			condition.UnknownCondition(heatv1beta1.HeatCfnAPIReadyCondition, condition.InitReason, heatv1beta1.HeatCfnAPIReadyInitMessage),
			condition.UnknownCondition(heatv1beta1.HeatEngineReadyCondition, condition.InitReason, heatv1beta1.HeatEngineReadyInitMessage),
			condition.UnknownCondition(heatv1beta1.HeatRabbitMqTransportURLReadyCondition, condition.InitReason, heatv1beta1.HeatRabbitMqTransportURLReadyInitMessage),
			// service account, role, rolebinding conditions
			condition.UnknownCondition(condition.ServiceAccountReadyCondition, condition.InitReason, condition.ServiceAccountReadyInitMessage),
			condition.UnknownCondition(condition.RoleReadyCondition, condition.InitReason, condition.RoleReadyInitMessage),
			condition.UnknownCondition(condition.RoleBindingReadyCondition, condition.InitReason, condition.RoleBindingReadyInitMessage),
		)

		instance.Status.Conditions.Init(&cl)

		// Register overall status immediately to have an early feedback e.g. in the cli
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

// SetupWithManager sets up the controller with the Manager.
func (r *HeatReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// transportURLSecretFn - Watch for changes made to the secret associated with the RabbitMQ
	// TransportURL created and used by Heat CRs.  Watch functions return a list of namespace-scoped
	// CRs that then get fed  to the reconciler.  Hence, in this case, we need to know the name of the
	// Heat CR associated with the secret we are examining in the function.  We could parse the name
	// out of the "%s-heat-transport" secret label, which would be faster than getting the list of
	// the Heat CRs and trying to match on each one.  The downside there, however, is that technically
	// someone could randomly label a secret "something-heat-transport" where "something" actually
	// matches the name of an existing Heat CR.  In that case changes to that secret would trigger
	// reconciliation for a Heat CR that does not need it.
	//
	// TODO: We also need a watch func to monitor for changes to the secret referenced by Heat.Spec.Secret
	transportURLSecretFn := func(o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		// get all Heat CRs
		heats := &heatv1beta1.HeatList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(context.Background(), heats, listOpts...); err != nil {
			r.Log.Error(err, "Unable to retrieve Heat CRs %v")
			return nil
		}

		for _, ownerRef := range o.GetOwnerReferences() {
			if ownerRef.Kind == "TransportURL" {
				for _, cr := range heats.Items {
					if ownerRef.Name == fmt.Sprintf("%s-heat-transport", cr.Name) {
						// return namespace and Name of CR
						name := client.ObjectKey{
							Namespace: o.GetNamespace(),
							Name:      cr.Name,
						}
						r.Log.Info(fmt.Sprintf("TransportURL Secret %s belongs to TransportURL belonging to Heat CR %s", o.GetName(), cr.Name))
						result = append(result, reconcile.Request{NamespacedName: name})
					}
				}
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
		Owns(&batchv1.Job{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&rabbitmqv1.TransportURL{}).
		// Watch for TransportURL Secrets which belong to any TransportURLs created by Heat CRs
		Watches(&source.Kind{Type: &corev1.Secret{}},
			handler.EnqueueRequestsFromMapFunc(transportURLSecretFn)).
		Complete(r)
}

func (r *HeatReconciler) reconcileDelete(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Heat delete")

	// remove db finalizer first
	db, err := database.GetDatabaseByName(ctx, helper, instance.Name)
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
	if err != nil {
		return rbacResult, err
	} else if (rbacResult != ctrl.Result{}) {
		return rbacResult, nil
	}

	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)

	// run check OpenStack secret - end

	//
	// create RabbitMQ transportURL CR and get the actual URL from the associated secret that is created
	//
	transportURL, op, err := r.transportURLCreateOrUpdate(instance)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatRabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatRabbitMqTransportURLReadyErrorMessage,
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
			heatv1beta1.HeatRabbitMqTransportURLReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			heatv1beta1.HeatRabbitMqTransportURLReadyRunningMessage))

		return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
	}

	//
	// check for required TransportURL secret holding transport URL string
	//

	transportURLSecret, hash, err := oko_secret.GetSecret(ctx, helper, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				heatv1beta1.HeatRabbitMqTransportURLReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				heatv1beta1.HeatRabbitMqTransportURLReadyRunningMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, fmt.Errorf("TransportURL secret %s not found", instance.Status.TransportURLSecret)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatRabbitMqTransportURLReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatRabbitMqTransportURLReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	configMapVars[transportURLSecret.Name] = env.SetValue(hash)

	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(heatv1beta1.HeatRabbitMqTransportURLReadyCondition, heatv1beta1.HeatRabbitMqTransportURLReadyMessage)

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for Heat input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal heat config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, instance, helper, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	_, err = r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	// Create ConfigMaps and Secrets - end

	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector: heat.ServiceName,
	}

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//

	// Create domain for Heat stacks
	ctrlResult, err = r.ensureStackDomain(ctx, helper, instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatStackDomainReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatStackDomainReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
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
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}
	// Mirror HeatEngine status' ReadyCount to this parent CR
	instance.Status.HeatEngineReadyCount = heatEngine.Status.ReadyCount

	// Mirror HeatEngine's condition status
	c := heatEngine.Status.Conditions.Mirror(heatv1beta1.HeatEngineReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
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
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}

	// Mirror HeatAPI status' ReadyCount to this parent CR
	instance.Status.HeatAPIReadyCount = heatAPI.Status.ReadyCount

	// Mirror HeatAPI's condition status
	c = heatAPI.Status.Conditions.Mirror(heatv1beta1.HeatAPIReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
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
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}

	// Mirror HeatCfnAPI status' ReadyCount to this parent CR
	instance.Status.HeatCfnAPIReadyCount = heatCfnAPI.Status.ReadyCount

	// Mirror HeatCfnAPI's condition status
	c = heatCfnAPI.Status.Conditions.Mirror(heatv1beta1.HeatCfnAPIReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
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
	// create service DB instance
	//
	db := database.NewDatabaseWithNamespace(
		heat.DatabaseName,
		instance.Spec.DatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.DatabaseInstance,
		},
		"heat",
		instance.Namespace,
	)
	// create or patch the DB
	ctrlResult, err := db.CreateOrPatchDBByName(
		ctx,
		helper,
		instance.Spec.DatabaseInstance,
	)

	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}
	// wait for the DB to be setup
	ctrlResult, err = db.WaitForDBCreated(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DBReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DBReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DBReadyRunningMessage))
		return ctrlResult, nil
	}
	// update Status.DatabaseHostname, used to bootstrap/config the service
	instance.Status.DatabaseHostname = db.GetDatabaseHostname()
	instance.Status.Conditions.MarkTrue(condition.DBReadyCondition, condition.DBReadyMessage)

	// create service DB - end

	//
	// run Heat db sync
	//
	dbSyncHash := instance.Status.Hash[heatv1beta1.DbSyncHash]
	jobDef := heat.DBSyncJob(instance, serviceLabels)
	dbSyncjob := job.NewJob(
		jobDef,
		heatv1beta1.DbSyncHash,
		instance.Spec.PreserveJobs,
		time.Duration(5)*time.Second,
		dbSyncHash,
	)
	ctrlResult, err = dbSyncjob.DoJob(
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
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[heatv1beta1.DbSyncHash]))
	}
	instance.Status.Conditions.MarkTrue(condition.DBSyncReadyCondition, condition.DBSyncReadyMessage)

	// run heat db sync - end

	r.Log.Info("Reconciled Heat init successfully")
	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileUpdate(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
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

// generateServiceConfigMaps - create create configmaps which hold scripts and service configuration
// TODO add DefaultConfigOverwrite
func (r *HeatReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *heatv1beta1.Heat,
	h *helper.Helper,
	envVars *map[string]env.Setter,
) error {
	//
	// create Configmap/Secret required for heat input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal heat config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(heat.ServiceName), map[string]string{})

	// customData hold any customization for the service.
	// custom.conf is going to /etc/heat/heat.conf.d
	// all other files get placed into /etc/heat to allow overwrite of e.g. policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	var err error
	keystoneAPI, err = keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return err
	}

	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return err
	}

	templateParameters := map[string]interface{}{
		"KeystonePublicURL":        authURL,
		"ServiceUser":              instance.Spec.ServiceUser,
		"StackDomainAdminUsername": heat.StackDomainAdminUsername,
		"StackDomainName":          heat.StackDomainName,
	}

	cms := []util.Template{
		// ScriptsConfigMap
		{
			Name:               fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:          instance.Namespace,
			Type:               util.TemplateTypeScripts,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{"common.sh": "/common/common.sh"},
			Labels:             cmLabels,
		},
		// ConfigMap
		{
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          util.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		},
	}
	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

func (r *HeatReconciler) reconcileUpgrade(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Heat upgrade")

	// TODO(bshephar): should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled Heat upgrade successfully")
	return ctrl.Result{}, nil
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
func (r *HeatReconciler) createHashOfInputHashes(
	ctx context.Context,
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
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return hash, err
		}
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
// This function relies on the keystoneAPI variable that is set globally in generateServiceConfigMaps().
func (r *HeatReconciler) ensureStackDomain(
	ctx context.Context,
	helper *helper.Helper,
	instance *heatv1beta1.Heat,
) (ctrl.Result, error) {
	//
	// get admin authentication OpenStack
	//
	os, ctrlResult, err := keystonev1.GetAdminServiceClient(
		ctx,
		helper,
		keystoneAPI,
	)
	if err != nil {
		return ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	domain := openstack.Domain{
		Name:        heat.StackDomainName,
		Description: "Domain for Heat stacks",
	}
	domainID, err := os.CreateDomain(r.Log, domain)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create Heat user
	userID, ctrlResult, err := r.ensureStackDomainAdminUser(ctx, helper, instance, os)
	if err != nil {
		return ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Add the user to the domain
	err = r.addUserToDomain(userID, domainID, os)
	return ctrl.Result{}, err
}

func (r *HeatReconciler) ensureStackDomainAdminUser(
	ctx context.Context,
	helper *helper.Helper,
	instance *heatv1beta1.Heat,
	os *openstack.OpenStack,
) (string, ctrl.Result, error) {

	// get the password of the service user from the secret
	password, ctrlResult, err := oko_secret.GetDataFromSecret(
		ctx,
		helper,
		instance.Spec.Secret,
		time.Duration(10),
		instance.Spec.PasswordSelectors.Service)
	if err != nil {
		return "", ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
		return "", ctrlResult, nil
	}

	userID, err := os.CreateUser(
		r.Log,
		openstack.User{
			Name:      heat.StackDomainAdminUsername,
			Password:  password,
			ProjectID: "service",
		})
	if err != nil {
		return "", ctrl.Result{}, err
	}
	return userID, ctrl.Result{}, nil
}

func (r *HeatReconciler) addUserToDomain(userID string, domainID string, os *openstack.OpenStack) error {
	//
	// add user to admin role
	//
	err := os.AssignUserDomainRole(
		r.Log,
		"admin",
		userID,
		domainID)
	return err
}
