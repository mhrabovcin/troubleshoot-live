package importer

const (
	defaultImportConcurrency = 8
	minImportConcurrency     = 1
	maxImportConcurrency     = 12
)

// ImportOptions controls importer behavior.
type ImportOptions struct {
	// Concurrency controls how many insert workers are used in concurrent stages.
	Concurrency int
}

// DefaultImportOptions returns the default importer options.
func DefaultImportOptions() ImportOptions {
	return ImportOptions{
		Concurrency: defaultImportConcurrency,
	}
}

func normalizeImportOptions(opts ImportOptions) ImportOptions {
	if opts.Concurrency == 0 {
		opts.Concurrency = defaultImportConcurrency
	}

	if opts.Concurrency < minImportConcurrency {
		opts.Concurrency = minImportConcurrency
	}
	if opts.Concurrency > maxImportConcurrency {
		opts.Concurrency = maxImportConcurrency
	}

	return opts
}
