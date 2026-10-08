// Command migrate manages the database schema. By default it uses the
// migrations embedded in the binary (so it works from any directory); pass
// -dir to run SQL files from a directory instead.
//
//	go run ./cmd/migrate -config configs/config.yaml up
//	go run ./cmd/migrate -config configs/config.yaml down 1
//	go run ./cmd/migrate -config configs/config.yaml version
//	go run ./cmd/migrate -config configs/config.yaml force <version>   # clear a dirty state
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"

	"github.com/stevenchen/llm-gateway/internal/adapter/repository/postgres"
	"github.com/stevenchen/llm-gateway/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to config.yaml")
	dir := flag.String("dir", "", "run migrations from this directory instead of the embedded set")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		return errors.New("usage: migrate [-config path] [-dir path] <up|down [n]|version|force <v>>")
	}

	cfg, err := config.LoadDefault(*configPath, flagSet("config"))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	m, err := postgres.NewMigrator(cfg.Postgres, *dir)
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	defer m.Close()

	switch args[0] {
	case "up":
		err = m.Up()
	case "down":
		steps := 1
		if len(args) > 1 {
			if steps, err = strconv.Atoi(args[1]); err != nil {
				return fmt.Errorf("invalid steps: %w", err)
			}
		}
		err = m.Steps(-steps)
	case "force":
		if len(args) < 2 {
			return errors.New("force needs a version")
		}
		v, perr := strconv.Atoi(args[1])
		if perr != nil {
			return fmt.Errorf("invalid version: %w", perr)
		}
		err = m.Force(v)
	case "version":
		version, dirty, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			fmt.Println("version=none (database has never been migrated)")
			return nil
		}
		if verr != nil {
			return fmt.Errorf("get version: %w", verr)
		}
		fmt.Printf("version=%d dirty=%v\n", version, dirty)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate %s: %w", args[0], err)
	}
	fmt.Println("ok")
	return nil
}

// flagSet reports whether the named flag was passed explicitly on the command line.
func flagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
