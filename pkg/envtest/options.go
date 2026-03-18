package envtest

import (
	"net/url"

	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
)

// Option allows to configure environment.
type Option func(*env.Env)

// StartOption allows to configure runtime API server startup.
type StartOption func(*APIServerStorageConfig)

// Arch can override default go runtime detected system arch. This can be useful
// when running on `arm64` and envtest binaries are not available for the platform.
func Arch(arch string) Option {
	return func(e *env.Env) {
		e.Platform.Arch = arch
	}
}

// WithDatastoreEndpoint configures the datastore endpoint used by the API server.
func WithDatastoreEndpoint(endpoint *url.URL) StartOption {
	return func(cfg *APIServerStorageConfig) {
		cfg.Endpoint = endpoint
	}
}

// WithDatastorePrefix configures kube-apiserver --etcd-prefix value.
func WithDatastorePrefix(prefix string) StartOption {
	return func(cfg *APIServerStorageConfig) {
		cfg.Prefix = prefix
	}
}
