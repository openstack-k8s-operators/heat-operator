package controller

import (
	"reflect"
	"testing"

	heatv1beta1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"

	"github.com/openstack-k8s-operators/lib-common/modules/common/clusterdns"
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
			"ServerName":    "my-service-internal.test1HeatNamespace.svc",
			"ServerAliases": []string{"my-service-internal.test1HeatNamespace.svc." + clusterdns.GetDNSClusterDomain()},
			"TLS":           false,
		},
		"public": map[string]any{
			"ServerName":            "my-service-public.test1HeatNamespace.svc",
			"ServerAliases":         []string{"my-service-public.test1HeatNamespace.svc." + clusterdns.GetDNSClusterDomain()},
			"TLS":                   true,
			"SSLCertificateFile":    "/etc/pki/tls/certs/public.crt",
			"SSLCertificateKeyFile": "/etc/pki/tls/private/public.key",
		},
	}
	result := map[string]any{}
	for _, tt := range tests {
		if err := renderVhost(result, tt.instance, tt.endpt, tt.serviceName, tt.tlsEnabled, service.RoutedOverrideSpec{}); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestRenderVhostEndpointURL(t *testing.T) {
	instance := &heatv1beta1.Heat{}
	instance.Namespace = "ns"
	result := map[string]any{}
	url := "https://heat.example.com:8004"
	err := renderVhost(result, instance, service.EndpointPublic, "heat-api", false,
		service.RoutedOverrideSpec{EndpointURL: &url})
	if err != nil {
		t.Fatal(err)
	}
	cfg := result["public"].(map[string]any)
	if cfg["ServerName"] != "heat.example.com" {
		t.Errorf("unexpected ServerName %v", cfg["ServerName"])
	}
	if !reflect.DeepEqual(cfg["ServerAliases"], []string{"heat-api-public.ns.svc", "heat-api-public.ns.svc." + clusterdns.GetDNSClusterDomain()}) {
		t.Errorf("unexpected ServerAliases %v", cfg["ServerAliases"])
	}
}
