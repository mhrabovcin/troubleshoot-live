package rewriter

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

type multiRewriter struct {
	rewriters []ResourceRewriter
}

var _ ResourceRewriter = (*multiRewriter)(nil)

// Multi executes serially multiple rewriters.
func Multi(rewriters ...ResourceRewriter) ResourceRewriter {
	return &multiRewriter{
		rewriters: rewriters,
	}
}

func (r *multiRewriter) BeforeImport(u *unstructured.Unstructured) error {
	for _, rewriter := range r.rewriters {
		if err := rewriter.BeforeImport(u); err != nil {
			return err
		}
	}
	return nil
}

func (r *multiRewriter) BeforeServing(u *unstructured.Unstructured) error {
	for _, rewriter := range r.rewriters {
		if err := rewriter.BeforeServing(u); err != nil {
			return err
		}
	}
	return nil
}
