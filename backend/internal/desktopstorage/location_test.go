package desktopstorage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultRootPreservesExistingData(t *testing.T) {
	for _, tc := range []struct {
		name string
		dirs []string
		want string
	}{
		{"new only", []string{"hfp-allinone"}, "hfp-allinone"},
		{"legacy only", []string{"BeefTV"}, "BeefTV"},
		{"both", []string{"hfp-allinone", "BeefTV"}, "hfp-allinone"},
		{"neither", nil, "hfp-allinone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			for _, name := range tc.dirs {
				if err := os.Mkdir(filepath.Join(configDir, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			got, err := defaultRoot(configDir)
			if err != nil || got != filepath.Join(configDir, tc.want) {
				t.Fatalf("defaultRoot() = %q, %v; want %q", got, err, tc.want)
			}
			entries, err := os.ReadDir(configDir)
			if err != nil || len(entries) != len(tc.dirs) {
				t.Fatalf("directory selection changed existing data: %v, %v", entries, err)
			}
		})
	}
}
