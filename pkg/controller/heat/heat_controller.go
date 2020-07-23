package heat

import (
	"context"
	"errors"

	logr "github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	comv1 "github.com/openstack-k8s-operators/heat-operator/pkg/apis/heat/v1"
	heatv1 "github.com/openstack-k8s-operators/heat-operator/pkg/apis/heat/v1"
	heat "github.com/openstack-k8s-operators/heat-operator/pkg/heat"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"math/rand"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log = logf.Log.WithName("controller_heat")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Heat Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHeat{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("heat-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Heat
	err = c.Watch(&source.Kind{Type: &heatv1.Heat{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Heat
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &heatv1.Heat{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary ConfigMaps
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &heatv1.Heat{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary Secrets
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &heatv1.Heat{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileHeat implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileHeat{}

// ReconcileHeat reconciles a Heat object
type ReconcileHeat struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Heat object and makes changes based on the state read
// and what is in the Heat.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileHeat) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Heat")

	// Fetch the Heat instance
	instance := &heatv1.Heat{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Passwords
	checkAndGeneratePassword("heat-service-password", instance.Namespace, r, request)
	checkAndGeneratePassword("heat-stack-domain-admin-password", instance.Namespace, r, request)
	checkAndGeneratePassword("heat-db-password", instance.Namespace, r, request)

	configMap := heat.ConfigMap(instance, instance.Name)
	if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if this ConfigMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "Job.Name", configMap.Name)
		err = r.client.Create(context.TODO(), configMap)
		if err != nil {
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(configMap.Data, foundConfigMap.Data) {
		reqLogger.Info("Updating ConfigMap")
		foundConfigMap.Data = configMap.Data
		err = r.client.Update(context.TODO(), foundConfigMap)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	}

	// Define a new Job object
	reqLogger.Info("HeatEngineContainerImage")
	reqLogger.Info(instance.Spec.HeatEngineContainerImage)
	job := heat.DbSyncJob(instance, instance.Name)
	dbSyncHash, err := util.ObjectHash(job)

	requeue := true
	if instance.Status.DbSyncHash != dbSyncHash {
		// Set Heat instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, job, r.scheme); err != nil {
			return reconcile.Result{}, err
		}
		requeue, err = EnsureJob(job, r, reqLogger)
		reqLogger.Info("Running DB sync")
		if err != nil {
			return reconcile.Result{}, err
		} else if requeue {
			reqLogger.Info("Waiting on DB sync")
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}
	// db sync completed... okay to store the hash to disable it
	r.setDbSyncHash(instance, dbSyncHash)
	// delete the job
	requeue, err = DeleteJob(job, r, reqLogger)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Define a new Deployment object
	configMapHash, err := util.ObjectHash(configMap)
	reqLogger.Info("ConfigMapHash: ", "ConfigMapHash:", configMapHash)
	deployment := heat.HeatAPIDeployment(instance, "heat-api", configMapHash)
	deploymentHash, err := util.ObjectHash(deployment)
	reqLogger.Info("Heat API DeploymentHash: ", "DeploymentHash:", deploymentHash)

	// Set Heat instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, deployment, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Heat API Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.client.Create(context.TODO(), deployment)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Deployment created successfully - don't requeue
		return reconcile.Result{RequeueAfter: time.Second * 5}, err

	} else if err != nil {
		return reconcile.Result{}, err
	} else {

		if instance.Status.DeploymentHash != deploymentHash {
			reqLogger.Info("Heat API Deployment Updated")
			foundDeployment.Spec = deployment.Spec
			err = r.client.Update(context.TODO(), foundDeployment)
			if err != nil {
				return reconcile.Result{}, err
			}
			r.setDeploymentHash(instance, deploymentHash)
			return reconcile.Result{RequeueAfter: time.Second * 10}, err
		}
		if foundDeployment.Status.ReadyReplicas == instance.Spec.HeatAPIReplicas {
			reqLogger.Info("Heat API Deployment Replicas running:", "Replicas", foundDeployment.Status.ReadyReplicas)
		} else {
			reqLogger.Info("Waiting on Heat API Deployment...")
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Define a new Deployment object
	reqLogger.Info("ConfigMapHash: ", "Data Hash:", configMapHash)
	engineDeployment := heat.HeatEngineDeployment(instance, "heat-engine", configMapHash)
	engineDeploymentHash, err := util.ObjectHash(engineDeployment)
	reqLogger.Info("Heat Engine DeploymentHash: ", "DeploymentHash:", engineDeploymentHash)

	// Set Heat instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, engineDeployment, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Deployment already exists
	foundEngineDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: engineDeployment.Name, Namespace: engineDeployment.Namespace}, foundEngineDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Heat Engine Deployment", "Deployment.Namespace", engineDeployment.Namespace, "Deployment.Name", engineDeployment.Name)
		err = r.client.Create(context.TODO(), engineDeployment)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Deployment created successfully - don't requeue
		return reconcile.Result{RequeueAfter: time.Second * 5}, err

	} else if err != nil {
		return reconcile.Result{}, err
	} else {

		if instance.Status.EngineDeploymentHash != engineDeploymentHash {
			reqLogger.Info("Heat Engine Deployment Updated")
			foundEngineDeployment.Spec = engineDeployment.Spec
			err = r.client.Update(context.TODO(), foundEngineDeployment)
			if err != nil {
				return reconcile.Result{}, err
			}
			r.setEngineDeploymentHash(instance, engineDeploymentHash)
			return reconcile.Result{RequeueAfter: time.Second * 10}, err
		}
		if foundEngineDeployment.Status.ReadyReplicas == instance.Spec.HeatEngineReplicas {
			reqLogger.Info("Heat Engine Deployment Replicas running:", "Replicas", foundEngineDeployment.Status.ReadyReplicas)
		} else {
			reqLogger.Info("Waiting on Heat Engine Deployment...")
			return reconcile.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Create the service if none exists
	service := heat.Service(instance, instance.Name)

	// Set Heat instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, service, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Service already exists
	foundService := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.client.Create(context.TODO(), service)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// Create the route if none exists
	route := heat.Route(instance, instance.Name)

	// Set Heat instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, route, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// Check if this Route already exists
	foundRoute := &routev1.Route{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, foundRoute)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		err = r.client.Create(context.TODO(), route)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil

}

func checkAndGeneratePassword(name string, namespace string, r *ReconcileHeat, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	foundSecret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, foundSecret)

	if err != nil && k8s_errors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Type: "Opaque",
			StringData: map[string]string{
				"password": generatePassword(32),
			},
		}
		reqLogger.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Job.Name", secret.Name)
		err = r.client.Create(context.TODO(), secret)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func generatePassword(length int) (password string) {
	const (
		lowerLetters = "abcdefghijkmnopqrstuvwxyz"
		upperLetters = "ABCDEFGHIJKLMNPQRSTUVWXYZ"
		digits       = "0123456789"
		all          = lowerLetters + upperLetters + digits
	)

	rand.Seed(time.Now().UnixNano())
	password = ""
	for i := 0; i < length; i++ {
		n := rand.Intn(len(all))
		password = password + string(all[n])
	}

	return password
}

// EnsureJob func
func EnsureJob(job *batchv1.Job, hr *ReconcileHeat, reqLogger logr.Logger) (bool, error) {
	// Check if this Job already exists
	foundJob := &batchv1.Job{}
	err := hr.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, foundJob)
	if err != nil && k8s_errors.IsNotFound(err) {
		reqLogger.Info("Creating a new Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		err = hr.client.Create(context.TODO(), job)
		if err != nil {
			return false, err
		}
		return true, err
	} else if err != nil {
		reqLogger.Info("EnsureJob err")
		return true, err
	} else if foundJob != nil {
		reqLogger.Info("EnsureJob foundJob")
		if foundJob.Status.Active > 0 {
			reqLogger.Info("Job Status Active... requeuing")
			return true, err
		} else if foundJob.Status.Failed > 0 {
			reqLogger.Info("Job Status Failed")
			return true, k8s_errors.NewInternalError(errors.New("Job Failed. Check job logs"))
		} else if foundJob.Status.Succeeded > 0 {
			reqLogger.Info("Job Status Successful")
		} else {
			reqLogger.Info("Job Status incomplete... requeuing")
			return true, err
		}
	}
	return false, nil

}

// DeleteJob func
func DeleteJob(job *batchv1.Job, hr *ReconcileHeat, reqLogger logr.Logger) (bool, error) {

	// Check if this Job already exists
	foundJob := &batchv1.Job{}
	err := hr.client.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, foundJob)
	if err == nil {
		reqLogger.Info("Deleting Job", "Job.Namespace", job.Namespace, "Job.Name", job.Name)
		err = hr.client.Delete(context.TODO(), foundJob)
		if err != nil {
			return false, err
		}
		return true, err
	}
	return false, nil

}

func (r *ReconcileHeat) setDbSyncHash(instance *comv1.Heat, hashStr string) error {

	if hashStr != instance.Status.DbSyncHash {
		instance.Status.DbSyncHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *ReconcileHeat) setDeploymentHash(instance *comv1.Heat, hashStr string) error {

	if hashStr != instance.Status.DeploymentHash {
		instance.Status.DeploymentHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}

func (r *ReconcileHeat) setEngineDeploymentHash(instance *comv1.Heat, hashStr string) error {

	if hashStr != instance.Status.DeploymentHash {
		instance.Status.EngineDeploymentHash = hashStr
		if err := r.client.Status().Update(context.TODO(), instance); err != nil {
			return err
		}
	}
	return nil

}
