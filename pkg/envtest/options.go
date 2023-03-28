package envtest

import (
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
)

// Option allows to configure environment.
type Option func(*env.Env)

// Arch can override default go runtime detected system arch. This can be useful
// when running on `arm64` and envtest binaries are not available for the platform.
func Arch(arch string) Option {
	return func(e *env.Env) {
		e.Platform.Arch = arch
	}
}
