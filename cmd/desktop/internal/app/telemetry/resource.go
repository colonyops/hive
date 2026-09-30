package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

const ServiceName = "hive-desktop"

// These resource attributes describe the service and the host that produced
// its telemetry. service.version is promoted by Prometheus but not by Loki,
// which is why deployment.environment.name separates builds.
const (
	attrServiceName       = "service.name"
	attrServiceVersion    = "service.version"
	attrServiceInstanceID = "service.instance.id"
	attrDeploymentEnvName = "deployment.environment.name"
	attrHostID            = "host.id"
)

// newResource omits an empty value rather than sending it blank: an empty
// `instance` label is worse than no label.
func newResource(opts Options) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{attribute.String(attrServiceName, ServiceName)}
	for key, value := range map[string]string{
		attrServiceVersion:    opts.Version,
		attrServiceInstanceID: opts.serviceInstanceID,
		attrDeploymentEnvName: opts.Environment,
		attrHostID:            opts.HostID,
	} {
		if value != "" {
			attrs = append(attrs, attribute.String(key, value))
		}
	}
	return resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
}
