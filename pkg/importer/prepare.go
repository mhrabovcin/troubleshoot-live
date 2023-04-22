package importer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/mhrabovcin/troubleshoot-live/pkg/rewriter"
)

// prepareForImport modifies object loaded from support bundle file in a way
// that can be imported.
func prepareForImport(in any) error {
	// TODO(mh): inject
	rr := rewriter.Default()

	u, ok := in.(*unstructured.Unstructured)
	if !ok {
		panic("non unstructured obj")
	}

	return rr.BeforeImport(u)
}
