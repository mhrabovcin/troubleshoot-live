package rewriter

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type conditionalRewriter struct {
	rewriter  ResourceRewriter
	condition Condition
}

var _ ResourceRewriter = (*conditionalRewriter)(nil)

// When executes provided rewriter only if condition matches.
func When(condition Condition, rewriter ResourceRewriter) ResourceRewriter {
	return &conditionalRewriter{
		rewriter:  rewriter,
		condition: condition,
	}
}

func (r *conditionalRewriter) BeforeImport(u *unstructured.Unstructured) error {
	if r.condition(u) {
		return r.rewriter.BeforeImport(u)
	}
	return nil
}

func (r *conditionalRewriter) BeforeServing(u *unstructured.Unstructured) error {
	if r.condition(u) {
		return r.rewriter.BeforeServing(u)
	}
	return nil
}

// Condition is function that returns bool with condition match result.
type Condition func(*unstructured.Unstructured) bool

// MatchGVK checks if resource matches given GroupVersionKind.
func MatchGVK(gvk schema.GroupVersionKind) Condition {
	return func(u *unstructured.Unstructured) bool {
		apiVersion, kind := gvk.ToAPIVersionAndKind()

		if u.GetAPIVersion() != apiVersion {
			return false
		}

		if u.GetKind() != kind {
			return false
		}

		return true
	}
}
