/*
Copyright 2025.

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

// Package v1beta1 implements webhook handlers for Heat v1beta1 API resources.
package v1beta1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
)

var (
	// ErrInvalidObjectType is returned when an unexpected object type is provided
	ErrInvalidObjectType = errors.New("invalid object type")
)

// nolint:unused
// log is for logging in this package.
var heatlog = logf.Log.WithName("heat-resource")

// SetupHeatWebhookWithManager registers the webhook for Heat in the manager.
func SetupHeatWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&heatv1beta1.Heat{}).
		WithValidator(&HeatCustomValidator{}).
		WithDefaulter(&HeatCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-heat-openstack-org-v1beta1-heat,mutating=true,failurePolicy=fail,sideEffects=None,groups=heat.openstack.org,resources=heats,verbs=create;update,versions=v1beta1,name=mheat-v1beta1.kb.io,admissionReviewVersions=v1

// HeatCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Heat when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type HeatCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &HeatCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Heat.
func (d *HeatCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	heat, ok := obj.(*heatv1beta1.Heat)

	if !ok {
		return fmt.Errorf("expected an Heat object but got %T: %w", obj, ErrInvalidObjectType)
	}
	heatlog.Info("Defaulting for Heat", "name", heat.GetName())

	// Call the Default method on the Heat type
	heat.Default()

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-heat-openstack-org-v1beta1-heat,mutating=false,failurePolicy=fail,sideEffects=None,groups=heat.openstack.org,resources=heats,verbs=create;update,versions=v1beta1,name=vheat-v1beta1.kb.io,admissionReviewVersions=v1

// HeatCustomValidator struct is responsible for validating the Heat resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type HeatCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &HeatCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Heat.
func (v *HeatCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	heat, ok := obj.(*heatv1beta1.Heat)
	if !ok {
		return nil, fmt.Errorf("expected a Heat object but got %T: %w", obj, ErrInvalidObjectType)
	}
	heatlog.Info("Validation for Heat upon creation", "name", heat.GetName())

	// Call the ValidateCreate method on the Heat type
	return heat.ValidateCreate()
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Heat.
func (v *HeatCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	heat, ok := newObj.(*heatv1beta1.Heat)
	if !ok {
		return nil, fmt.Errorf("expected a Heat object for the newObj but got %T: %w", newObj, ErrInvalidObjectType)
	}
	heatlog.Info("Validation for Heat upon update", "name", heat.GetName())

	// Call the ValidateUpdate method on the Heat type
	return heat.ValidateUpdate(oldObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Heat.
func (v *HeatCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	heat, ok := obj.(*heatv1beta1.Heat)
	if !ok {
		return nil, fmt.Errorf("expected a Heat object but got %T: %w", obj, ErrInvalidObjectType)
	}
	heatlog.Info("Validation for Heat upon deletion", "name", heat.GetName())

	// Call the ValidateDelete method on the Heat type
	return heat.ValidateDelete()
}
