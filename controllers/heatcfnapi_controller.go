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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	heat "github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	heatcfnapi "github.com/openstack-k8s-operators/heat-operator/pkg/heatcfnapi"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common"
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	"github.com/openstack-k8s-operators/lib-common/modules/common/deployment"
	"github.com/openstack-k8s-operators/lib-common/modules/common/endpoint"
	"github.com/openstack-k8s-operators/lib-common/modules/common/env"
	"github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	"github.com/openstack-k8s-operators/lib-common/modules/common/labels"
	"github.com/openstack-k8s-operators/lib-common/modules/common/secret"
	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
	"github.com/openstack-k8s-operators/lib-common/modules/common/tls"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
)

// HeatCfnAPIReconciler reconciles a Heat object
type HeatCfnAPIReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Log     logr.Logger
	Kclient kubernetes.Interface
}

var keystoneCfnServices = []map[string]string{
	{
		"type": heat.CfnServiceType,
		"name": heat.CfnServiceName,
		"desc": "Heat Cloudformation API service",
	},
}

// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatcfnapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatcfnapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=heat.openstack.org,resources=heatcfnapis/finalizers,verbs=update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;create;update;patch;delete;watch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;
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
func (r *HeatCfnAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, _err error) {
	_ = log.FromContext(ctx)

	// TODO(bshephar): your logic here
	// We should have a Database and a `heat.conf` file by this time. Handled by the heat_controller.
	// We just need to initialise the API in a deployment here, and allow scaling of the ReplicaSet.
	// Updates and Upgrades can be handled by the heat_controller.

	instance := &heatv1beta1.HeatCfnAPI{}
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

	var savedConditions condition.Conditions
	var isNewInstance bool
	isNewInstance, savedConditions = verifyStatusConditions(instance.Status.Conditions)

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
		instance.Status.Hash = map[string]string{}
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)
}

// SetupWithManager sets up the controller with the Manager.
func (r *HeatCfnAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// index passwordSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.HeatCfnAPI{}, passwordSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.HeatCfnAPI)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.Secret}
	}); err != nil {
		return err
	}

	// index transportURLSecretField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.HeatCfnAPI{}, transportURLSecretField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.HeatCfnAPI)
		if cr.Spec.Secret == "" {
			return nil
		}
		return []string{cr.Spec.TransportURLSecret}
	}); err != nil {
		return err
	}

	// index caBundleSecretNameField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.HeatCfnAPI{}, caBundleSecretNameField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.HeatCfnAPI)
		if cr.Spec.TLS.CaBundleSecretName == "" {
			return nil
		}
		return []string{cr.Spec.TLS.CaBundleSecretName}
	}); err != nil {
		return err
	}

	// index tlsAPIInternalField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.HeatCfnAPI{}, tlsAPIInternalField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.HeatCfnAPI)
		if cr.Spec.TLS.API.Internal.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Internal.SecretName}
	}); err != nil {
		return err
	}

	// index tlsAPIPublicField
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &heatv1beta1.HeatCfnAPI{}, tlsAPIPublicField, func(rawObj client.Object) []string {
		// Extract the secret name from the spec, if one is provided
		cr := rawObj.(*heatv1beta1.HeatCfnAPI)
		if cr.Spec.TLS.API.Public.SecretName == nil {
			return nil
		}
		return []string{*cr.Spec.TLS.API.Public.SecretName}
	}); err != nil {
		return err
	}

	configMapFn := func(_ context.Context, o client.Object) []reconcile.Request {
		result := []reconcile.Request{}

		apis := &heatv1beta1.HeatCfnAPIList{}
		listOpts := []client.ListOption{
			client.InNamespace(o.GetNamespace()),
		}
		if err := r.Client.List(context.Background(), apis, listOpts...); err != nil {
			r.Log.Error(err, "Unable to get CfnAPI CRs %v")
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
		For(&heatv1beta1.HeatCfnAPI{}).
		Owns(&keystonev1.KeystoneService{}).
		Owns(&keystonev1.KeystoneEndpoint{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		// watch the config CMs we don't own
		Watches(&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(configMapFn)).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForSrc),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *HeatCfnAPIReconciler) findObjectsForSrc(ctx context.Context, src client.Object) []reconcile.Request {
	requests := []reconcile.Request{}

	l := log.FromContext(ctx).WithName("Controllers").WithName("HeatCfnAPI")

	for _, field := range heatCfnWatchFields {
		crList := &heatv1beta1.HeatAPIList{}
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

func (r *HeatCfnAPIReconciler) reconcileDelete(ctx context.Context, instance *heatv1beta1.HeatCfnAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling CfnAPI Delete")

	for _, ksSvc := range keystoneCfnServices {
		keystoneEndpoint, err := keystonev1.GetKeystoneEndpointWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil {
			if controllerutil.RemoveFinalizer(keystoneEndpoint, helper.GetFinalizer()) {
				err = r.Update(ctx, keystoneEndpoint)
				if err != nil && !k8s_errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				util.LogForObject(helper, "Removed finalizer from KeystoneEndpoint", instance)
			}
		}

		keystoneService, err := keystonev1.GetKeystoneServiceWithName(ctx, helper, ksSvc["name"], instance.Namespace)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil {
			if controllerutil.RemoveFinalizer(keystoneService, helper.GetFinalizer()) {
				err = r.Update(ctx, keystoneService)
				if err != nil && !k8s_errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				util.LogForObject(helper, "Removed finalizer from our KeystoneService", instance)
			}
		}
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled CfnAPI delete successfully")

	return ctrl.Result{}, nil
}

func (r *HeatCfnAPIReconciler) reconcileInit(
	ctx context.Context,
	instance *heatv1beta1.HeatCfnAPI,
	helper *helper.Helper,
	serviceLabels map[string]string,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling CfnAPI init")

	//
	// expose the service (create service and return the created endpoint URLs)
	//

	// V1
	heatCfnAPIEndpoints := map[service.Endpoint]endpoint.Data{
		service.EndpointPublic: {
			Port: heat.HeatCfnPublicPort,
			Path: "/v1",
		},
		service.EndpointInternal: {
			Port: heat.HeatCfnInternalPort,
			Path: "/v1",
		},
	}

	apiEndpoints := make(map[string]string)

	for endpointType, data := range heatCfnAPIEndpoints {
		endpointTypeStr := string(endpointType)
		endpointName := heatcfnapi.ServiceName + "-" + endpointTypeStr
		svcOverride := instance.Spec.Override.Service[endpointType]
		if svcOverride.EmbeddedLabelsAnnotations == nil {
			svcOverride.EmbeddedLabelsAnnotations = &service.EmbeddedLabelsAnnotations{}
		}

		exportLabels := util.MergeStringMaps(
			serviceLabels,
			map[string]string{
				service.AnnotationEndpointKey: endpointTypeStr,
			},
		)

		// Create the service
		svc, err := service.NewService(
			service.GenericService(&service.GenericServiceDetails{
				Name:      endpointName,
				Namespace: instance.Namespace,
				Labels:    exportLabels,
				Selector:  serviceLabels,
				Port: service.GenericServicePort{
					Name:     endpointName,
					Port:     data.Port,
					Protocol: corev1.ProtocolTCP,
				},
			}),
			5,
			&svcOverride.OverrideSpec,
		)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ExposeServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.ExposeServiceReadyErrorMessage,
				err.Error()))

			return ctrl.Result{}, err
		}

		svc.AddAnnotation(map[string]string{
			service.AnnotationEndpointKey: endpointTypeStr,
		})

		// add Annotation to whether creating an ingress is required or not
		if endpointType == service.EndpointPublic && svc.GetServiceType() == corev1.ServiceTypeClusterIP {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "true",
			})
		} else {
			svc.AddAnnotation(map[string]string{
				service.AnnotationIngressCreateKey: "false",
			})
			if svc.GetServiceType() == corev1.ServiceTypeLoadBalancer {
				svc.AddAnnotation(map[string]string{
					service.AnnotationHostnameKey: svc.GetServiceHostname(), // add annotation to register service name in dnsmasq
				})
			}
		}

		ctrlResult, err := svc.CreateOrPatch(ctx, helper)
		if err != nil {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ExposeServiceReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.ExposeServiceReadyErrorMessage,
				err.Error()))

			return ctrlResult, err
		} else if (ctrlResult != ctrl.Result{}) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.ExposeServiceReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.ExposeServiceReadyRunningMessage))
			return ctrlResult, nil
		}
		// create service - end

		// if TLS is enabled
		if instance.Spec.TLS.API.Enabled(endpointType) {
			// set endpoint protocol to https
			data.Protocol = ptr.To(service.ProtocolHTTPS)
		}

		apiEndpoints[string(endpointType)], err = svc.GetAPIEndpoint(
			svcOverride.EndpointURL, data.Protocol, data.Path)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	instance.Status.Conditions.MarkTrue(condition.ExposeServiceReadyCondition, condition.ExposeServiceReadyMessage)

	// expose service - end

	//
	// create users and endpoints
	//
	for _, ksSvc := range keystoneCfnServices {
		ksSvcSpec := keystonev1.KeystoneServiceSpec{
			ServiceType:        ksSvc["type"],
			ServiceName:        ksSvc["name"],
			ServiceDescription: ksSvc["desc"],
			Enabled:            true,
			ServiceUser:        instance.Spec.ServiceUser,
			Secret:             instance.Spec.Secret,
			PasswordSelector:   instance.Spec.PasswordSelectors.Service,
		}

		ksSvcObj := keystonev1.NewKeystoneService(ksSvcSpec, instance.Namespace, serviceLabels, time.Second*10)
		ctrlResult, err := ksSvcObj.CreateOrPatch(ctx, helper)
		if err != nil || (ctrlResult != ctrl.Result{}) {
			return ctrlResult, err
		}

		// mirror the Status, Reason, Severity and Message of the latest keystoneservice condition
		// into a local condition with the type condition.KeystoneServiceReadyCondition
		c := ksSvcObj.GetConditions().Mirror(condition.KeystoneServiceReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}

		//
		// register endpoints
		//
		ksEndptSpec := keystonev1.KeystoneEndpointSpec{
			ServiceName: ksSvc["name"],
			Endpoints:   apiEndpoints,
		}
		ksEndpt := keystonev1.NewKeystoneEndpoint(
			ksSvc["name"],
			instance.Namespace,
			ksEndptSpec,
			serviceLabels,
			time.Second*10)
		ctrlResult, err = ksEndpt.CreateOrPatch(ctx, helper)
		if err != nil || (ctrlResult != ctrl.Result{}) {
			return ctrlResult, err
		}
		// mirror the Status, Reason, Severity and Message of the latest keystoneendpoint condition
		// into a local condition with the type condition.KeystoneEndpointReadyCondition
		c = ksEndpt.GetConditions().Mirror(condition.KeystoneEndpointReadyCondition)
		if c != nil {
			instance.Status.Conditions.Set(c)
		}
	}

	r.Log.Info("Reconciled CfnAPI init successfully")
	return ctrl.Result{}, nil
}

func (r *HeatCfnAPIReconciler) reconcileNormal(ctx context.Context, instance *heatv1beta1.HeatCfnAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// Secret
	secretVars := make(map[string]env.Setter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := secret.GetSecret(ctx, helper, instance.Spec.Secret, instance.Namespace)
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
	// run check OpenStack secret - end

	//
	// check for required TransportURL secret holding transport URL string
	//
	_, hash, err = secret.GetSecret(ctx, helper, instance.Spec.TransportURLSecret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			r.Log.Info(fmt.Sprintf("Transport secret %s not found", instance.Spec.TransportURLSecret))
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
	secretVars[instance.Spec.TransportURLSecret] = env.SetValue(hash)
	// run check TransportURL secret - end

	//
	// check for required Heat config maps that should have been created by parent Heat CR
	//
	parentHeatName := heat.GetOwningHeatName(instance)

	ctrlResult, err := r.getSecret(ctx, helper, instance, fmt.Sprintf("%s-scripts", parentHeatName), &secretVars)
	if err != nil {
		return ctrlResult, err
	}
	ctrlResult, err = r.getSecret(ctx, helper, instance, fmt.Sprintf("%s-config-data", parentHeatName), &secretVars)
	// note r.getSecret adds Conditions with condition.InputReadyWaitingMessage
	// when secret is not found
	if err != nil {
		return ctrlResult, err
	}

	instance.Status.Conditions.MarkTrue(condition.InputReadyCondition, condition.InputReadyMessage)
	// run check parent Heat CR config maps - end

	//
	// TLS input validation
	//
	// Validate the CA cert secret if provided
	if instance.Spec.TLS.CaBundleSecretName != "" {
		hash, err := tls.ValidateCACertSecret(
			ctx,
			helper.GetClient(),
			types.NamespacedName{
				Name:      instance.Spec.TLS.CaBundleSecretName,
				Namespace: instance.Namespace,
			},
		)
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				instance.Status.Conditions.Set(condition.FalseCondition(
					condition.TLSInputReadyCondition,
					condition.RequestedReason,
					condition.SeverityInfo,
					fmt.Sprintf(condition.TLSInputReadyWaitingMessage, instance.Spec.TLS.CaBundleSecretName)))
				return ctrl.Result{}, nil
			}
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.ErrorReason,
				condition.SeverityWarning,
				condition.TLSInputErrorMessage,
				err.Error()))
			return ctrl.Result{}, err
		}

		if hash != "" {
			secretVars[tls.CABundleKey] = env.SetValue(hash)
		}
	}

	// Validate API service certs secrets
	certsHash, err := instance.Spec.TLS.API.ValidateCertSecrets(ctx, helper, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.TLSInputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				fmt.Sprintf(condition.TLSInputReadyWaitingMessage, err.Error())))
			return ctrl.Result{}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.TLSInputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.TLSInputErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	secretVars[tls.TLSHashName] = env.SetValue(certsHash)

	// all cert input checks out so report InputReady
	instance.Status.Conditions.MarkTrue(condition.TLSInputReadyCondition, condition.InputReadyMessage)

	//
	// Create Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create custom Secret for this heat-api-cfn service
	//
	err = r.generateServiceSecrets(ctx, helper, instance, &secretVars)
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
	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, err := r.createHashOfInputHashes(instance, secretVars)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.ServiceConfigReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.ServiceConfigReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}
	instance.Status.Conditions.MarkTrue(condition.ServiceConfigReadyCondition, condition.ServiceConfigReadyMessage)
	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	serviceLabels := map[string]string{
		common.AppSelector:       heat.ServiceName,
		common.ComponentSelector: heat.CfnAPIComponent,
	}

	// Handle service init
	ctrlResult, err = r.reconcileInit(ctx, instance, helper, serviceLabels)
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

	//
	// normal reconcile tasks
	//

	// Define a new Deployment object
	deplSpec, err := heatcfnapi.Deployment(instance, inputHash, serviceLabels)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}
	depl := deployment.NewDeployment(
		deplSpec,
		time.Second*10,
	)

	ctrlResult, err = depl.CreateOrPatch(ctx, helper)
	if err != nil {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.DeploymentReadyErrorMessage,
			err.Error()))
		return ctrlResult, err
	}
	if (ctrlResult != ctrl.Result{}) {
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.DeploymentReadyCondition,
			condition.RequestedReason,
			condition.SeverityInfo,
			condition.DeploymentReadyRunningMessage))
		return ctrlResult, nil
	}
	if depl.GetDeployment().Generation == depl.GetDeployment().Status.ObservedGeneration {
		instance.Status.ReadyCount = depl.GetDeployment().Status.ReadyReplicas
		if instance.Status.ReadyCount > 0 {
			instance.Status.Conditions.MarkTrue(condition.DeploymentReadyCondition, condition.DeploymentReadyMessage)
		}
	}
	// create Deployment - end

	// We reached the end of the Reconcile, update the Ready condition based on
	// the sub conditions
	if instance.Status.Conditions.AllSubConditionIsTrue() {
		instance.Status.Conditions.MarkTrue(
			condition.ReadyCondition, condition.ReadyMessage)
	}
	r.Log.Info("Reconciled CfnAPI successfully")
	return ctrl.Result{}, nil
}

func (r *HeatCfnAPIReconciler) reconcileUpdate() (ctrl.Result, error) {
	r.Log.Info("Reconciling CfnAPI update")

	// TODO: should have minor update tasks if required
	// - delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled CfnAPI update successfully")
	return ctrl.Result{}, nil
}

func (r *HeatCfnAPIReconciler) reconcileUpgrade() (ctrl.Result, error) {
	r.Log.Info("Reconciling CfnAPI upgrade")

	// TODO: should have major version upgrade tasks
	// -delete dbsync hash from status to rerun it?

	r.Log.Info("Reconciled CfnAPI upgrade successfully")
	return ctrl.Result{}, nil
}

// getSecret - get the specified secret, and add its hash to envVars
func (r *HeatCfnAPIReconciler) getSecret(
	ctx context.Context,
	h *helper.Helper,
	instance *heatv1beta1.HeatCfnAPI,
	secretName string,
	envVars *map[string]env.Setter,
) (ctrl.Result, error) {
	secret, hash, err := secret.GetSecret(ctx, h, secretName, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			r.Log.Info(fmt.Sprintf("Secret %s not found", secretName))
			instance.Status.Conditions.Set(condition.FalseCondition(
				condition.InputReadyCondition,
				condition.RequestedReason,
				condition.SeverityInfo,
				condition.InputReadyWaitingMessage))
			return ctrl.Result{RequeueAfter: time.Duration(10) * time.Second}, nil
		}
		instance.Status.Conditions.Set(condition.FalseCondition(
			condition.InputReadyCondition,
			condition.ErrorReason,
			condition.SeverityWarning,
			condition.InputReadyErrorMessage,
			err.Error()))
		return ctrl.Result{}, err
	}

	(*envVars)[secret.Name] = env.SetValue(hash)

	return ctrl.Result{}, nil
}

// generateServiceSecrets - create custom secret to hold service-specific config
// TODO add DefaultConfigOverwrite
func (r *HeatCfnAPIReconciler) generateServiceSecrets(
	ctx context.Context,
	h *helper.Helper,
	instance *heatv1beta1.HeatCfnAPI,
	envVars *map[string]env.Setter,
) error {
	//
	// create custom Secret for heat-api-specific config input
	// - %-config-data secret holding custom config for the service's heat.conf
	//

	cmLabels := labels.GetLabels(instance, labels.GetGroupLabel(heat.CfnServiceName), map[string]string{})

	db, err := mariadbv1.GetDatabaseByNameAndAccount(ctx, h, heat.DatabaseCRName, instance.Spec.DatabaseAccount, instance.Namespace)
	if err != nil {
		return err
	}
	var tlsCfg *tls.Service
	if instance.Spec.TLS.Ca.CaBundleSecretName != "" {
		tlsCfg = &tls.Service{}
	}

	// customData hold any customization for the service.
	// custom.conf is going to /etc/heat/heat.conf.d
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{
		heat.CustomServiceConfigFileName: instance.Spec.CustomServiceConfig,
		"my.cnf":                         db.GetDatabaseClientConfig(tlsCfg), //(mschuppert) for now just get the default my.cnf
	}

	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	customData[heat.CustomServiceConfigFileName] = instance.Spec.CustomServiceConfig

	// Fetch the two service config snippets (DefaultsConfigFileName and
	// CustomConfigFileName) from the Secret generated by the top level
	// heat controller, and add them to this service specific Secret.
	heatSecretName := heat.GetOwningHeatName(instance) + "-config-data"
	heatSecret, _, err := secret.GetSecret(ctx, h, heatSecretName, instance.Namespace)
	if err != nil {
		return err
	}
	customData[heat.DefaultsConfigFileName] = string(heatSecret.Data[heat.DefaultsConfigFileName])
	customData[heat.CustomConfigFileName] = string(heatSecret.Data[heat.CustomConfigFileName])

	customSecrets := ""
	for _, secretName := range instance.Spec.CustomServiceConfigSecrets {
		secret, _, err := secret.GetSecret(ctx, h, secretName, instance.Namespace)
		if err != nil {
			return err
		}
		for _, data := range secret.Data {
			customSecrets += string(data) + "\n"
		}
	}
	customData[heat.CustomServiceConfigSecretsFileName] = customSecrets

	cms := []util.Template{
		// Custom Secret
		{
			Name:         fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:    instance.Namespace,
			Type:         util.TemplateTypeConfig,
			InstanceType: instance.Kind,
			CustomData:   customData,
			Labels:       cmLabels,
		},
	}

	return secret.EnsureSecrets(ctx, h, instance, cms, envVars)
}

// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
func (r *HeatCfnAPIReconciler) createHashOfInputHashes(
	instance *heatv1beta1.HeatCfnAPI,
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
