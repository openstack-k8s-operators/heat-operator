package heat

import (
	comv1 "github.com/openstack-k8s-operators/heat-operator/pkg/apis/heat/v1"
	util "github.com/openstack-k8s-operators/heat-operator/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type heatConfigOptions struct {
	HeatDatabasePassword         string
	HeatDatabaseUsername         string
	HeatMessagingPassword        string
	HeatMessagingUsername        string
	HeatAPIContainerImage        string
	HeatEngineContainerImage     string
	HeatAPIReplicas              int32
	HeatEngineReplicas           int32
	HeatServicePassword          string
	HeatStackDomainAdminPassword string
}

// ConfigMap custom heat config map
func ConfigMap(cr *comv1.Heat, cmName string) *corev1.ConfigMap {
	opts := heatConfigOptions{
		cr.Spec.HeatDatabasePassword,
		cr.Spec.HeatDatabaseUsername,
		cr.Spec.HeatMessagingPassword,
		cr.Spec.HeatMessagingUsername,
		cr.Spec.HeatAPIContainerImage,
		cr.Spec.HeatEngineContainerImage,
		cr.Spec.HeatAPIReplicas,
		cr.Spec.HeatEngineReplicas,
		cr.Spec.HeatServicePassword,
		cr.Spec.HeatStackDomainAdminPassword,
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cr.Namespace,
		},
		Data: map[string]string{
			"heat.conf":                     util.ExecuteTemplateFile("heat.conf", &opts),
			"10-heat_api_wsgi.conf":         util.ExecuteTemplateFile("10-heat_api_wsgi.conf", nil),
			"kolla_config_heat_api.json":    util.ExecuteTemplateFile("kolla_config_heat_api.json", nil),
			"kolla_config_heat_engine.json": util.ExecuteTemplateFile("kolla_config_heat_engine.json", nil),
		},
	}

	return cm
}
