package importer

import "testing"

func TestNormalizeImportOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   ImportOptions
		want int
	}{
		{
			name: "uses default for zero",
			in:   ImportOptions{},
			want: defaultImportConcurrency,
		},
		{
			name: "clamps lower bound",
			in:   ImportOptions{Concurrency: -1},
			want: minImportConcurrency,
		},
		{
			name: "clamps upper bound",
			in:   ImportOptions{Concurrency: maxImportConcurrency + 5},
			want: maxImportConcurrency,
		},
		{
			name: "keeps valid value",
			in:   ImportOptions{Concurrency: 6},
			want: 6,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeImportOptions(tt.in)
			if got.Concurrency != tt.want {
				t.Fatalf("expected concurrency %d, got %d", tt.want, got.Concurrency)
			}
		})
	}
}
