package migrations

import (
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"testing"
)

// Every version needs both directions, and versions must be contiguous:
// a gap or a missing .down.sql only shows up later as a broken rollback or a
// "schema behind" error in production.
func TestEmbeddedMigrationsAreConsistent(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`^(\d{6})_.+\.(up|down)\.sql$`)
	dirs := map[int]map[string]bool{}
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue // embed.go etc. are not embedded (only *.sql), but be tolerant
		}
		v, _ := strconv.Atoi(m[1])
		if dirs[v] == nil {
			dirs[v] = map[string]bool{}
		}
		dirs[v][m[2]] = true
	}
	if len(dirs) == 0 {
		t.Fatal("no migrations embedded")
	}
	versions := make([]int, 0, len(dirs))
	for v, d := range dirs {
		versions = append(versions, v)
		if !d["up"] || !d["down"] {
			t.Errorf("migration %06d needs both .up.sql and .down.sql (has %v)", v, d)
		}
	}
	sort.Ints(versions)
	for i, v := range versions {
		if v != i+1 {
			t.Fatalf("migration versions must be contiguous from 1: got %v", versions)
		}
	}
	latest, err := LatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	if int(latest) != versions[len(versions)-1] {
		t.Errorf("LatestVersion()=%d, want %d", latest, versions[len(versions)-1])
	}
}
