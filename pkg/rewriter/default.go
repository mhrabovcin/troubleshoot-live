package rewriter

// Default provides a rewriter that covers most cases of required changes for
// successful import and serving of a diagnostics bundle.
func Default() ResourceRewriter {
	return Multi(
		GeneratedValues(),
		DeletedNamespace(),
	)
}
