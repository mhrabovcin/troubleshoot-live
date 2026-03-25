package importer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
)

// ObjectPreparer modifies object loaded from support bundle file in a way that
// can be imported.
type ObjectPreparer interface {
	Prepare(u *unstructured.Unstructured) error
}

type rewriterObjectPreparer struct {
	rewriter rewriter.ResourceRewriter
}

func (p rewriterObjectPreparer) Prepare(u *unstructured.Unstructured) error {
	return p.rewriter.BeforeImport(u)
}

func defaultObjectPreparer() ObjectPreparer {
	return rewriterObjectPreparer{
		rewriter: rewriter.Default(),
	}
}
