package controllers

import (
	"reflect"
	"testing"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/service"
)

func TestRenderVhost(t *testing.T) {
	instanceTest1 := &heatv1beta1.Heat{}
	instanceTest1.Namespace = "test1HeatNamespace"

	tests := []struct {
		name        string
		instance    *heatv1beta1.Heat
		endpt       service.Endpoint
		serviceName string
		tlsEnabled  bool
		expected    map[string]interface{}
	}{
		{
			name:        "Basic case with TLS disabled",
			instance:    instanceTest1,
			endpt:       "internal",
			serviceName: "my-service",
			tlsEnabled:  false,
			expected: map[string]interface{}{
				"internal": map[string]interface{}{
					"ServerName": "my-service-internal.test1HeatNamespace.svc",
					"TLS":        false,
				},
			},
		},
		{
			name:        "Basic case with TLS enabled",
			instance:    instanceTest1,
			endpt:       "public",
			serviceName: "my-service",
			tlsEnabled:  true,
			expected: map[string]interface{}{
				"public": map[string]interface{}{
					"ServerName":            "my-service-public.test1HeatNamespace.svc",
					"TLS":                   true,
					"SSLCertificateFile":    "/etc/pki/tls/certs/public.crt",
					"SSLCertificateKeyFile": "/etc/pki/tls/private/public.key",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderVhost(tt.instance, tt.endpt, tt.serviceName, tt.tlsEnabled)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
