package controllers

import (
	"reflect"
	"testing"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
)

func TestRenderVhost(t *testing.T) {
	instanceTest1 := &heatv1beta1.Heat{}
	instanceTest1.Namespace = "test1HeatNamespace"

	tests := []struct {
		name                string
		instance            *heatv1beta1.Heat
		endpt               service.Endpoint
		serviceName         string
		tlsEnabled          bool
		httpdOverrideSecret *corev1.Secret
		want                map[string]interface{}
		wantCustomTemplates map[string]string
	}{
		{
			name:                "Basic case with TLS disabled",
			instance:            instanceTest1,
			endpt:               "internal",
			serviceName:         "my-service",
			tlsEnabled:          false,
			wantCustomTemplates: map[string]string{},
			want: map[string]interface{}{
				"internal": map[string]interface{}{
					"ServerName": "my-service-internal.test1HeatNamespace.svc",
					"TLS":        false,
					"Override":   false,
				},
			},
		},
		{
			name:                "Basic case with TLS enabled",
			instance:            instanceTest1,
			endpt:               "public",
			serviceName:         "my-service",
			tlsEnabled:          true,
			wantCustomTemplates: map[string]string{},
			want: map[string]interface{}{
				"public": map[string]interface{}{
					"ServerName":            "my-service-public.test1HeatNamespace.svc",
					"TLS":                   true,
					"SSLCertificateFile":    "/etc/pki/tls/certs/public.crt",
					"SSLCertificateKeyFile": "/etc/pki/tls/private/public.key",
					"Override":              false,
				},
			},
		},
		{
			name:        "Basic case with TLS disabled and httpdOverrideSecret",
			instance:    instanceTest1,
			endpt:       "internal",
			serviceName: "my-service",
			tlsEnabled:  false,
			httpdOverrideSecret: &corev1.Secret{
				Data: map[string][]byte{
					"foo": []byte("bar"),
				},
			},
			wantCustomTemplates: map[string]string{
				"httpd_custom_my-service_internal_foo": "bar",
			},
			want: map[string]interface{}{
				"internal": map[string]interface{}{
					"ServerName": "my-service-internal.test1HeatNamespace.svc",
					"TLS":        false,
					"Override":   true,
				},
			},
		},
		{
			name:        "Basic case with TLS enabled and httpdOverrideSecret",
			instance:    instanceTest1,
			endpt:       "public",
			serviceName: "my-service",
			tlsEnabled:  true,
			httpdOverrideSecret: &corev1.Secret{
				Data: map[string][]byte{
					"foo": []byte("bar"),
				},
			},
			wantCustomTemplates: map[string]string{
				"httpd_custom_my-service_public_foo": "bar",
			},
			want: map[string]interface{}{
				"public": map[string]interface{}{
					"ServerName":            "my-service-public.test1HeatNamespace.svc",
					"TLS":                   true,
					"SSLCertificateFile":    "/etc/pki/tls/certs/public.crt",
					"SSLCertificateKeyFile": "/etc/pki/tls/private/public.key",
					"Override":              true,
				},
			},
		},
	}

	result := map[string]interface{}{}
	for _, tt := range tests {
		tt := &tt
		t.Run(tt.name, func(t *testing.T) {
			customTemplates := renderVhost(result, tt.instance, tt.endpt, tt.serviceName, tt.tlsEnabled, tt.httpdOverrideSecret)
			if !reflect.DeepEqual(result[string(tt.endpt)], tt.want[string(tt.endpt)]) {
				t.Errorf("Expected %v\ngot %v", tt.want, result)
			}
			if !reflect.DeepEqual(customTemplates, tt.wantCustomTemplates) {
				t.Errorf("CustomTemplate = %v, want %v", customTemplates, tt.wantCustomTemplates)
			}

		})

	}
}
