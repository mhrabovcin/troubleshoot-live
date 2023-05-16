package rewriter

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Default provides a rewriter that covers most cases of required changes for
// successful import and serving of a diagnostics bundle.
func Default() ResourceRewriter {
	return Multi(
		GeneratedValues(),
		DeletedNamespace(),
		When(
			MatchGVK(schema.FromAPIVersionAndKind("v1", "Pod")),
			Multi(
				RemoveField("spec", "priority"),
				RemoveField("spec", "priorityClassName"),
			),
		),
		When(
			MatchGVK(schema.FromAPIVersionAndKind("v1", "Pod")),
			Multi(
				RemoveField("spec", "runtimeClassName"),
			),
		),
		When(
			MatchGVK(schema.FromAPIVersionAndKind("networking.k8s.io/v1", "Ingress")),
			RemoveField("spec", "ingressClassName"),
		),
	)
}
