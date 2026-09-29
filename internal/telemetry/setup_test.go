package telemetry

import (
	"testing"
)

func TestSamplerFromEnv(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")
	if samplerFromEnv() == nil {
		t.Fatal("sampler")
	}
	t.Setenv("OTEL_TRACES_SAMPLER", "traceidratio")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.1")
	if samplerFromEnv() == nil {
		t.Fatal("nil sampler")
	}
}

func TestResourceAttributesIncludeDownwardAPI(t *testing.T) {
	t.Setenv("K8S_POD_NAME", "payment-0")
	t.Setenv("K8S_NAMESPACE_NAME", "shop")
	t.Setenv("K8S_NODE_NAME", "node-a")
	t.Setenv("K8S_DEPLOYMENT_NAME", "payment")
	attrs := resourceAttributes("payment-service")
	found := map[string]string{}
	for _, a := range attrs {
		found[string(a.Key)] = a.Value.AsString()
	}
	if found["service.name"] != "payment-service" || found["k8s.pod.name"] != "payment-0" || found["k8s.namespace.name"] != "shop" {
		t.Fatalf("%v", found)
	}
	if found["k8s.node.name"] != "node-a" || found["k8s.deployment.name"] != "payment" {
		t.Fatalf("%v", found)
	}
}
