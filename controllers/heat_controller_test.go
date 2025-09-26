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
	}{
		{
			name:        "Basic case with TLS disabled",
			instance:    instanceTest1,
			endpt:       "internal",
			serviceName: "my-service",
			tlsEnabled:  false,
		},
		{
			name:        "Basic case with TLS enabled",
			instance:    instanceTest1,
			endpt:       "public",
			serviceName: "my-service",
			tlsEnabled:  true,
		},
	}

	expected := map[string]any{
		"internal": map[string]any{
			"ServerName": "my-service-internal.test1HeatNamespace.svc",
			"TLS":        false,
		},
		"public": map[string]any{
			"ServerName":            "my-service-public.test1HeatNamespace.svc",
			"TLS":                   true,
			"SSLCertificateFile":    "/etc/pki/tls/certs/public.crt",
			"SSLCertificateKeyFile": "/etc/pki/tls/private/public.key",
		},
	}
	result := map[string]any{}
	for _, tt := range tests {
		renderVhost(result, tt.instance, tt.endpt, tt.serviceName, tt.tlsEnabled)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}
