// Package migrations embeds the versioned SQL schema so the gateway binary can
// bring any empty or outdated database up to date by itself, regardless of the
// working directory it is started from.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
)

//go:embed *.sql
var FS embed.FS

var fileRe = regexp.MustCompile(`^(\d+)_.+\.(up|down)\.sql$`)

// LatestVersion returns the highest migration version embedded in the binary —
// the schema version this build of the gateway requires.
func LatestVersion() (uint, error) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}
	var latest uint
	for _, e := range entries {
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.ParseUint(m[1], 10, 32)
		if uint(v) > latest {
			latest = uint(v)
		}
	}
	if latest == 0 {
		return 0, fmt.Errorf("no migrations embedded")
	}
	return latest, nil
}
