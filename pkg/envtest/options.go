package envtest

import "sigs.k8s.io/controller-runtime/tools/setup-envtest/env"

// Option allows to configure environment.
type Option func(*env.Env)

// StartOption allows to configure runtime API server startup.
type StartOption func(*APIServerStartConfig)

// Arch can override default go runtime detected system arch. This can be useful
// when running on `arm64` and envtest binaries are not available for the platform.
func Arch(arch string) Option {
	return func(e *env.Env) {
		e.Platform.Arch = arch
	}
}

// WithStorageBackend configures the storage backend used by the API server.
func WithStorageBackend(storageBackend StorageBackend) StartOption {
	return func(cfg *APIServerStartConfig) {
		cfg.StorageBackend = storageBackend
	}
}

// WithStorageID configures the storage allocation identity for one API server.
func WithStorageID(id string) StartOption {
	return func(cfg *APIServerStartConfig) {
		cfg.StorageID = id
	}
}

// WithAPIServerFeatureGate configures a kube-apiserver --feature-gates entry.
func WithAPIServerFeatureGate(name string, enabled bool) StartOption {
	return func(cfg *APIServerStartConfig) {
		if cfg.FeatureGates == nil {
			cfg.FeatureGates = map[string]bool{}
		}
		cfg.FeatureGates[name] = enabled
	}
}

// WithAPIServerFeatureGates configures kube-apiserver --feature-gates entries.
func WithAPIServerFeatureGates(featureGates map[string]bool) StartOption {
	return func(cfg *APIServerStartConfig) {
		if cfg.FeatureGates == nil {
			cfg.FeatureGates = map[string]bool{}
		}
		for name, enabled := range featureGates {
			cfg.FeatureGates[name] = enabled
		}
	}
}
