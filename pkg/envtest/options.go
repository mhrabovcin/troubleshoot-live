package envtest

import (
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
)

type Option func(*env.Env)

func Arch(arch string) Option {
	return func(e *env.Env) {
		e.Platform.Arch = arch
	}
}
