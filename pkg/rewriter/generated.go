package rewriter

// GeneratedValues removes generated values.
// See: https://kubernetes.io/docs/reference/using-api/api-concepts/#generated-values
func GeneratedValues() ResourceRewriter {
	return Multi(
		RemoveField("metadata", "generateName"),
		RemoveField("metadata", "creationTimestamp"),
		RemoveField("metadata", "deletionTimestamp"),
		RemoveField("metadata", "deletionGracePeriodSeconds"),
		RemoveField("metadata", "uid"),
		RemoveField("metadata", "resourceVersion"),
	)
}
