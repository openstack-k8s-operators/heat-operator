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
	"sigs.k8s.io/controller-runtime/pkg/log"

	heat "github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	labels "github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
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
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=rabbitmq.openstack.org,resources=transporturls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Heat object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *HeatReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	instance := &heatv1beta1.Heat{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("Error getting instance: %w", err)
	}

	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}

		cl := condition.CreateList(
			condition.UnknownCondition(condition.DBReadyCondition, condition.InitReason, condition.DBReadyInitMessage),
			condition.UnknownCondition(condition.DBSyncReadyCondition, condition.InitReason, condition.DBSyncReadyInitMessage),
			condition.UnknownCondition(condition.ExposeServiceReadyCondition, condition.InitReason, condition.ExposeServiceReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
			// right now we have no dedicated KeystoneServiceReadyInitMessage
			condition.UnknownCondition(condition.KeystoneServiceReadyCondition, condition.InitReason, ""),
		)

		instance.Status.Conditions.Init(&cl)

		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, fmt.Errorf("Error updating status: %w", err)
		}
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]map[string]string{}
	}

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("Error initialising new helper: %w", err)
	}
	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		// update the overall status condition if service is ready
		if instance.IsReady() {
			instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
		}

		if err := helper.SetAfter(instance); err != nil {
			util.LogErrorForObject(helper, err, "Set after and calc patch/diff", instance)
		}

		if changed := helper.GetChanges()["status"]; changed {
			patch := client.MergeFrom(helper.GetBeforeObject())

			if err := r.Status().Patch(ctx, instance, patch); err != nil && !k8s_errors.IsNotFound(err) {
				util.LogErrorForObject(helper, err, "Update status", instance)
			}
		}
	}()
	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)

}

// SetupWithManager sets up the controller with the Manager.
func (r *HeatReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&heatv1beta1.Heat{}).
		Owns(&heatv1beta1.HeatAPI{}).
		Owns(&heatv1beta1.HeatEngine{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&rabbitmqv1.TransportURL{}).
		Complete(r)
}

func (r *HeatReconciler) reconcileDelete(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Heat delete")

	// remove db finalizer first
	db, err := database.GetDatabaseByName(ctx, helper, instance.Name)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("Error retrieving database: %w", err)
	}

	if !k8s_errors.IsNotFound(err) {
		if err := db.DeleteFinalizer(ctx, helper); err != nil {
			return ctrl.Result{}, fmt.Errorf("Error removing DB finalizer: %w", err)
		}
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Heat delete successfully")
	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("Error removing finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *HeatReconciler) reconcileNormal(ctx context.Context, instance *heatv1beta1.Heat, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// ConfigMap
	configMapVars := make(map[string]env.Setter)
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
		return ctrl.Result{}, fmt.Errorf("Error creating transportURL: %w", err)
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

	transportURLSecret, hash, err := secret.GetSecret(ctx, helper, instance.Status.TransportURLSecret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, fmt.Errorf("TransportURL secret %s not found", instance.Status.TransportURLSecret)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("Error looking up transportURL secret: %w", err)
	}
	configMapVars[transportURLSecret.Name] = env.SetValue(hash)

	// run check TransportURL secret - end

	instance.Status.Conditions.MarkTrue(heatv1beta1.HeatRabbitMqTransportURLReadyCondition, heatv1beta1.HeatRabbitMqTransportURLReadyMessage)

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, helper.GetFinalizer())
	// Register the finalizer immediately to avoid orphaning resources on delete
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("Error adding finalizer: %w", err)
	}

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
		return ctrl.Result{}, fmt.Errorf("Error looking up OpenStack Secret: %w", err)
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check OpenStack secret - end

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
		return ctrl.Result{}, fmt.Errorf("Error creating ConfigMap for service: %w", err)
	}

	_, err = r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("Error updating status with config hash: %w", err)
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
		return ctrlResult, fmt.Errorf("Error initializing service: %w", err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, fmt.Errorf("Error updating service: %w", err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, fmt.Errorf("Error upgrading service: %w", err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//
	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, helper, instance.Namespace, map[string]string{})
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// get admin authentication OpenStack
	//
	os, _, err := keystonev1.GetAdminServiceClient(
		ctx,
		helper,
		keystoneAPI,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create Heat user
	userID, err := r.ensureHeatUser(ctx, helper, instance, keystoneAPI, os)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create domain for Heat stacks
	heatDomain := openstack.Domain{
		Name:        "heat_stack",
		Description: "Domain for Heat stacks",
	}
	domainID, err := r.ensureHeatDomain(heatDomain, os)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Add the user to the domain
	err = r.addUserToDomain(userID, domainID, os)
	if err != nil {
		return ctrl.Result{}, err
	}

	// deploy heat-engine
	heatEngine, op, err := r.engineDeploymentCreateOrUpdate(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatEngineReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("Error creating Heat Engine deployment: %w", err)
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}
	// Mirror HeatEngine status' ReadyCount to this parent CR
	// instance.Status.ServiceIDs = heatEngine.Status.ServiceIDs
	instance.Status.HeatEngineReadyCount = heatEngine.Status.ReadyCount

	// Mirror HeatEngine's condition status
	c := heatEngine.Status.Conditions.Mirror(heatv1beta1.HeatEngineReadyCondition)
	if c != nil {
		instance.Status.Conditions.Set(c)
	}

	// deploy heat-api
	heatAPI, op, err := r.apiDeploymentCreateOrUpdate(instance)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			heatv1beta1.HeatAPIReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			heatv1beta1.HeatAPIReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("Error creating Heat API deployment: %w", err)
	}
	if op != controllerutil.OperationResultNone {
		r.Log.Info(fmt.Sprintf("Deployment %s successfully reconciled - operation: %s", instance.Name, string(op)))
	}

	// Mirror HeatAPI status' APIEndpoints and ReadyCount to this parent CR
	instance.Status.APIEndpoints = heatAPI.Status.APIEndpoints
	instance.Status.ServiceIDs = heatAPI.Status.ServiceIDs
	instance.Status.HeatAPIReadyCount = heatAPI.Status.ReadyCount

	// Mirror HeatAPI's condition status
	c = heatAPI.Status.Conditions.Mirror(heatv1beta1.HeatAPIReadyCondition)
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
		return ctrl.Result{}, fmt.Errorf("Error creating Heat DB: %w", err)
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
		return ctrlResult, fmt.Errorf("Error while initializing database: %w", err)
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
		return ctrl.Result{}, fmt.Errorf("Error during DB sync: %w", err)
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

func (r *HeatReconciler) apiDeploymentCreateOrUpdate(instance *heatv1beta1.Heat) (*heatv1beta1.HeatAPI, controllerutil.OperationResult, error) {
	deployment := &heatv1beta1.HeatAPI{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-api", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = instance.Spec.HeatAPI
		// TODO: Add logic to determine when to set/overwrite, etc
		deployment.Spec.ServiceUser = instance.Spec.ServiceUser
		deployment.Spec.DatabaseHostname = instance.Status.DatabaseHostname
		deployment.Spec.DatabaseUser = instance.Spec.DatabaseUser
		deployment.Spec.Secret = instance.Spec.Secret
		deployment.Spec.TransportURLSecret = instance.Status.TransportURLSecret
		deployment.Spec.PasswordSelectors = instance.Spec.PasswordSelectors

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return fmt.Errorf("Error setting Owner Reference: %w", err)
		}

		return nil
	})

	return deployment, op, fmt.Errorf("Heat API deployment failed: %w", err)
}

func (r *HeatReconciler) engineDeploymentCreateOrUpdate(instance *heatv1beta1.Heat) (*heatv1beta1.HeatEngine, controllerutil.OperationResult, error) {
	deployment := &heatv1beta1.HeatEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-engine", instance.Name),
			Namespace: instance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.Client, deployment, func() error {
		deployment.Spec = instance.Spec.HeatEngine
		// TODO: Add logic to determine when to set/overwrite, etc
		deployment.Spec.ServiceUser = instance.Spec.ServiceUser
		deployment.Spec.DatabaseHostname = instance.Status.DatabaseHostname
		deployment.Spec.DatabaseUser = instance.Spec.DatabaseUser
		deployment.Spec.Secret = instance.Spec.Secret
		deployment.Spec.TransportURLSecret = instance.Status.TransportURLSecret
		deployment.Spec.PasswordSelectors = instance.Spec.PasswordSelectors

		err := controllerutil.SetControllerReference(instance, deployment, r.Scheme)
		if err != nil {
			return fmt.Errorf("Error setting Owner Reference: %w", err)
		}

		return nil
	})

	return deployment, op, fmt.Errorf("Heat Engine deployment failed: %w", err)
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

	keystoneAPI, err := keystonev1.GetKeystoneAPI(ctx, h, instance.Namespace, map[string]string{})
	if err != nil {
		return fmt.Errorf("Error getting Keystone API: %w", err)
	}
	authURL, err := keystoneAPI.GetEndpoint(endpoint.EndpointPublic)
	if err != nil {
		return fmt.Errorf("Error getting Keystone Endpoint: %w", err)
	}

	templateParameters := map[string]interface{}{
		"KeystonePublicURL":        authURL,
		"ServiceUser":              instance.Spec.ServiceUser,
		"StackdomainAdminUsername": heat.StackDomainAdminUsername,
		"StackdomainName":          heat.StackDomainName,
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
		return hash, fmt.Errorf("Error generating Object Hash: %w", err)
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return hash, fmt.Errorf("Error while adding Hash to status: %w", err)
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

		err := controllerutil.SetControllerReference(instance, transportURL, r.Scheme)
		return fmt.Errorf("Error setting Owner Reference for TransportURL: %w", err)
	})

	return transportURL, op, fmt.Errorf("Error while creating TransportURL: %w", err)
}

func (r *HeatReconciler) ensureHeatDomain(domain openstack.Domain, os *openstack.OpenStack) (string, error) {
	domainID, err := os.CreateDomain(r.Log, domain)

	if err != nil {
		return "", err
	}

	return domainID, nil
}

func (r *HeatReconciler) ensureHeatUser(ctx context.Context, helper *helper.Helper, instance *heatv1beta1.Heat, keystoneAPI *keystonev1.KeystoneAPI, os *openstack.OpenStack) (string, error) {

	// get the password of the service user from the secret
	password, _, err := oko_secret.GetDataFromSecret(
		ctx,
		helper,
		instance.Spec.Secret,
		time.Duration(10),
		instance.Spec.PasswordSelectors.Service)
	if err != nil {
		return "", err
	}

	userID, err := os.CreateUser(
		r.Log,
		openstack.User{
			Name:      heat.StackDomainAdminUsername,
			Password:  password,
			ProjectID: "service",
		})
	if err != nil {
		return "", err
	}
	return userID, nil
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
	if err != nil {
		return err
	}

	return nil
}
