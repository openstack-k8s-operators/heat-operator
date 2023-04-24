/*
Copyright 2022.

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

package controllers

import (
	"context"
	"fmt"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	heat "github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	heatapi "github.com/openstack-k8s-operators/heat-operator/pkg/heatapi"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/configmap"
	"github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// GetClient -
func (r *HeatAPIReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *HeatAPIReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *HeatAPIReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *HeatAPIReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// HeatAPIReconciler reconciles a Heat object
type HeatAPIReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Log     logr.Logger
	Kclient kubernetes.Interface
}

var (
	keystoneServices = []map[string]string{
		{
			"type": heat.ServiceType,
			"name": heat.ServiceName,
			"desc": "Heat API service",
		},
	}
)

// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneendpoints,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Heat object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *HeatAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(bshephar): your logic here
	// We should have a Database and a `heat.conf` file by this time. Handled by the heat_controller.
	// We just need to initialise the API in a deployment here, and allow scaling of the ReplicaSet.
	// Updates and Upgrades can be handled by the heat_controller.

	instance := &heatv1beta1.HeatAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if instance.Status.Conditions == nil {
		instance.Status.Conditions = condition.Conditions{}

		cl := condition.CreateList(
			condition.UnknownCondition(condition.ExposeServiceReadyCondition, condition.InitReason, condition.ExposeServiceReadyInitMessage),
			condition.UnknownCondition(condition.InputReadyCondition, condition.InitReason, condition.InputReadyInitMessage),
			condition.UnknownCondition(condition.ServiceConfigReadyCondition, condition.InitReason, condition.ServiceConfigReadyInitMessage),
			condition.UnknownCondition(condition.DeploymentReadyCondition, condition.InitReason, condition.DeploymentReadyInitMessage),
		)

		instance.Status.Conditions.Init(&cl)

		if err := r.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	if instance.Status.Hash == nil {
		instance.Status.Hash = map[string]string{}
	}
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]map[string]string{}
	}
	if instance.Status.ServiceIDs == nil {
		instance.Status.ServiceIDs = map[string]string{}
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

	defer func() {
		if instance.IsReady() {
			instance.Status.Conditions.MarkTrue(condition.ReadyCondition, condition.ReadyMessage)
		}

		if err := helper.SetAfter(instance); err != nil {
			patch := client.MergeFrom(helper.GetBeforeObject())
			if err := r.Status().Patch(ctx, instance, patch); err != nil && !k8s_errors.IsNotFound(err) {
				util.LogErrorForObject(helper, err, "Update Status", instance)
			}
		}
	}()

	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *HeatAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {

	configMapFn := func(o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		apis := &heatv1beta1.HeatAPIList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(context.Background(), apis, listOpts...); err != nil {
			r.Log.Error(err, "Unable to get API CRs %v")
			return nil
		}

		label := o.GetLabels()

		if l, ok := label[labels.GetOwnerNameLabelSelector(labels.GetGroupLabel(heat.ServiceName))]; ok {
			for _, cr := range apis.Items {
				if l == heat.GetOwningHeatName(&cr) {
					name := client.ObjectKey{
						Namespace: o.GetNamespace(),
						Name:      cr.Name,
					}
					r.Log.Info(fmt.Sprintf("ConfigMap object %s and CR %s marked with label: %s", o.GetName(), cr.Name, l))
					result = append(result, reconcile.Request{NamespacedName: name})
				}
			}
		}
		if len(result) > 0 {
			return result
		}
		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&heatv1beta1.HeatAPI{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.Service{}).
		// watch the config CMs we don't own
		Watches(&source.Kind{Type: &corev1.ConfigMap{}},
			handler.EnqueueRequestsFromMapFunc(configMapFn)).
		Complete(r)
}

func (r *HeatAPIReconciler) reconcileDelete(ctx context.Context, instance *heatv1beta1.HeatAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling API Delete")

	for _, ksSvc := range keystoneServices {
		keystoneEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("Failed to get Keystone endpoint: %w", err)
		}
		if err == nil {
			controllerutil.RemoveFinalizer(keystoneEndpoint, helper.GetFinalizer())
			if err = helper.GetClient().Update(ctx, keystoneEndpoint); err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("Failed to remove finalizer: %w", err)
			}
			util.LogForObject(helper, "Removed finalizer from KeystoneEndpoint", instance)
		}

		keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("Failed to get Keystone service: %w", err)
		}
		if err == nil {
			controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer())
			if err = helper.GetClient().Update(ctx, keystoneService); err != nil && !k8s_errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("Failed to remove finalizer from Keystone Service: %w", err)
			}
			util.LogForObject(helper, "Removed finalizer from our KeystoneService", instance)
		}
	}

	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled API delete successfully")
	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("Failed to update %s instance: %w", instance.Name, err)
	}
	return ctrl.Result{}, nil
}

func (r *HeatAPIReconciler) reconcileInit(
	ctx context.Context,
	instance *heatv1beta1.HeatAPI,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling API init")

	//
	// expose the service (create service, route and return the created endpoint URLs)
	//

	// V1
	data := map[endpoint.Endpoint]endpoint.Data{
		endpoint.EndpointPublic: {
			Port: heat.HeatPublicPort,
			Path: "/v1/%(tenant_id)s",
		},
		endpoint.EndpointAdmin: {
			Port: heat.HeatAdminPort,
			Path: "/v1/%(tenant_id)s",
		},
		endpoint.EndpointInternal: {
			Port: heat.HeatInternalPort,
			Path: "/v1/%(tenant_id)s",
		},
	}

	apiEndpoints, ctrlResult, err := endpoint.ExposeEndpoints(
		ctx,
		helper,
		heat.ServiceName,
		serviceLabels,
		data,
		time.Duration(5)*time.Second,
	)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ExposeServiceReadyErrorMessage,
			err.Error()))
		return ctrlResult, fmt.Errorf("Failed tpo expose %s endpoint: %w", instance.Name, err)
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ExposeServiceReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.ExposeServiceReadyRunningMessage))
		return ctrlResult, nil
	}

	//
	// Update instance status with service endpoint url from route host information for v2
	//
	// TODO: need to support https default here
	if instance.Status.APIEndpoints == nil {
		instance.Status.APIEndpoints = map[string]map[string]string{}
	}
	instance.Status.APIEndpoints[heat.ServiceName] = apiEndpoints
	// V1 - end

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	// expose service - end

	//
	// create users and endpoints
	// TODO: rework this
	//
	if instance.Status.ServiceIDs == nil {
		instance.Status.ServiceIDs = map[string]string{}
	}

	for _, ksSvc := range keystoneServices {
		r.Log.Info("Reconciled API init successfully")
		ksSvcSpec := keystonev1.KeystoneServiceSpec{
			ServiceType:        ksSvc["type"],
			ServiceName:        ksSvc["name"],
			ServiceDescription: ksSvc["desc"],
			Enabled:            true,
			ServiceUser:        instance.Spec.ServiceUser,
			Secret:             instance.Spec.Secret,
			PasswordSelector:   instance.Spec.PasswordSelectors.Service,
		}

		ksSvcObj := keystonev1.NewKeystoneService(ksSvcSpec, instance.Namespace, serviceLabels, time.Duration(10)*time.Second)
		ctrlResult, err = ksSvcObj.CreateOrPatch(ctx, helper)
		if err != nil {
			return ctrlResult, fmt.Errorf("Failed to create KeystoneService for %s: %w", instance.Name, err)
		}

		// mirror the Status, Reason, Severity and Message of the latest keystoneservice condition
		// into a local condition with the type condition.KeystoneServiceReadyCondition
		c := ksSvcObj.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}

		if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}

		instance.Status.ServiceIDs[ksSvc["name"]] = ksSvcObj.GetServiceID()

		//
		// register endpoints
		//
		ksEndptSpec := keystonev1.KeystoneEndpointSpec{
			ServiceName: ksSvc["name"],
			Endpoints:   instance.Status.APIEndpoints[ksSvc["name"]],
		}
		ksEndpt := keystonev1.NewKeystoneEndpoint(
			ksSvc["name"],
			instance.Namespace,
			ksEndptSpec,
			serviceLabels,
			time.Duration(10)*time.Second)
		ctrlResult, err = ksEndpt.CreateOrPatch(ctx, helper)
		if err != nil {
			return ctrlResult, fmt.Errorf("Failed to create KeystoneEndpoint: %w", err)
		}
		// mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
		// into a local condition with the type condition.KeystoneEndpointReadyCondition
		c = ksEndpt.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}

		if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}
	}

	r.Log.Info("Reconciled API init successfully")
	return ctrl.Result{}, nil
}

func (r *HeatAPIReconciler) reconcileNormal(ctx context.Context, instance *heatv1beta1.HeatAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, helper.GetFinalizer())
	// Register the finalizer immediately to avoid orphaning resources on delete
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, fmt.Errorf("Failed to update instance with finalizer: %w", err)
	}
	// ConfigMap
	configMapVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
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
		return ctrl.Result{}, fmt.Errorf("Failed to get OpenStack Secret: %w", err)
	}
	configMapVars[ospSecret.Name] = env.SetValue(hash)
	// run check OpenStack secret - end

	//
	// check for required Heat config maps that should have been created by parent Heat CR
	//

	parentHeatName := heat.GetOwningHeatName(instance)

	configMaps := []string{
		fmt.Sprintf("%s-scripts", parentHeatName),     //ScriptsConfigMap
		fmt.Sprintf("%s-config-data", parentHeatName), //ConfigMap
	}

	_, err = configmap.GetConfigMaps(ctx, helper, instance, configMaps, instance.Namespace, &configMapVars)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("could not find all config maps for parent Heat CR %s", parentHeatName)
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("Failed to get config map: %w", err)
	}
	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check parent Heat CR config maps - end

	//
	// Create ConfigMaps required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create custom Configmap for this heat volume service
	//
	err = r.generateServiceConfigMaps(ctx, helper, instance, &configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("Failed to generate config map for %s: %w", instance.Name, err)
	}
	// Create ConfigMaps - end
	// Create ConfigMaps and Secrets - end
	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, fmt.Errorf("Failed to create hash of config inputs: %w", err)
	}
	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)
	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector:     heat.ServiceName,
		heat.ComponentSelector: heat.APIComponent,
	}

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper, serviceLabels)
	if err != nil {
		return ctrlResult, fmt.Errorf("Failed to initialize %s service: %w", instance.Name, err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, fmt.Errorf("Failed to update %s: %w", instance.Name, err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, fmt.Errorf("Failed to upgrade %s: %w", instance.Name, err)
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//

	// Define a new Deployment object
	depl := deployment.NewDeployment(
		heatapi.Deployment(instance, inputHash, serviceLabels),
		time.Duration(5)*time.Second,
	)

	ctrlResult, err = depl.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, fmt.Errorf("Failed to create %s deployment: %w", instance.Name, err)
	} else if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}
	instance.Status.ReadyCount = depl.GetDeployment().Status.ReadyReplicas
	if instance.Status.ReadyCount > 0 {
		instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
	}
	// create Deployment - end

	r.Log.Info("Reconciled API successfully")
	return ctrl.Result{}, nil
}

func (r *HeatAPIReconciler) reconcileUpdate(ctx context.Context, instance *heatv1beta1.HeatAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling API update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled API update successfully")
	return ctrl.Result{}, nil
}

func (r *HeatAPIReconciler) reconcileUpgrade(ctx context.Context, instance *heatv1beta1.HeatAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling API upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled API upgrade successfully")
	return ctrl.Result{}, nil
}

// generateServiceConfigMaps - create custom configmap to hold service-specific config
// TODO add DefaultConfigOverwrite
func (r *HeatAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	h *helper.Helper,
	instance *heatv1beta1.HeatAPI,
	envVars *map[string]env.Setter,
) error {
	//
	// create custom Configmap for heat-api-specific config input
	// - %-config-data configmap holding custom config for the service's heat.conf
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(heat.ServiceName), map[string]string{})

	// customData hold any customization for the service.
	// custom.conf is going to /etc/heat/heat.conf.d
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{common.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig}

	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	customData[common.CustomServiceConfigFileName] = instance.Spec.CustomServiceConfig

	cms := []util.Template{
		// Custom ConfigMap
		{
			Name:         fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeConfig,
			InstanceType: instance.Kind,
			CustomData:   customData,
			Labels:       cmLabels,
		},
	}

	return configmap.EnsureConfigMaps(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
func (r *HeatAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *heatv1beta1.HeatAPI,
	envVars map[string]env.Setter,
) (string, error) {
	mergedMapVars := env.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := util.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, fmt.Errorf("Failed to create Hash of inputs: %w", err)
	}
	if hashMap, changed := util.SetHash(instance.Status.Hash, common.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return hash, fmt.Errorf("Failed to update %s status with config hash: %w", instance.Name, err)
		}
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", common.InputHashName, hash))
	}
	return hash, nil
}
