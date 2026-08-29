// Command preset-validator validates authored preset files against the vendored
// slicer schemas plus the cloud's additional strictness.
//
// It is the single-file half of the validation the cloud runs: schema
// conformance, unknown-field and forbidden-field rejection, per-type parameter
// allowlists, semantic range checks, the id pattern and the file-name-equals-id
// rule. Cross-file invariants (catalog-wide id uniqueness, vendor ownership) are
// applied by ingest, not here.
//
// Usage:
//
//	preset-validator [--kind printer|filament|process] <path>...
//
// Each path may be a file or a directory (walked for *.yaml / *.yml). When
// --kind is omitted, the kind is inferred from the path per the presets
// repository layout (vendors/<slug>/printers, .../filaments, processes/).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ColdCrabby/cloud-presets/internal/preset"
)

func main() {
	kindFlag := flag.String("kind", "", "preset kind: printer|filament|process (inferred from path if empty)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [--kind printer|filament|process] <path>...\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	v, err := preset.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not build validator: %v\n", err)
		os.Exit(2)
	}

	files, err := collectFiles(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	var checked, failed int
	for _, file := range files {
		kind, err := resolveKind(*kindFlag, file)
		if err != nil {
			fmt.Printf("FAIL %s\n  - %v\n", file, err)
			failed++
			checked++
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("FAIL %s\n  - %v\n", file, err)
			failed++
			checked++
			continue
		}

		res := v.ValidateFile(kind, file, data)
		checked++
		if res.Valid() {
			fmt.Printf("ok   %s (%s)\n", file, kind)
			continue
		}
		failed++
		fmt.Printf("FAIL %s (%s)\n", file, kind)
		for _, e := range res.Errors {
			fmt.Printf("  - %s\n", e.String())
		}
	}

	fmt.Printf("\n%d file(s) checked, %d failed\n", checked, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func resolveKind(flagValue, file string) (preset.Kind, error) {
	if flagValue != "" {
		k := preset.Kind(flagValue)
		if !k.Valid() {
			return "", fmt.Errorf("invalid --kind %q", flagValue)
		}
		return k, nil
	}
	k, ok := preset.KindFromPath(file)
	if !ok {
		return "", fmt.Errorf("cannot infer kind from path (expected it under printers/, filaments/ or processes/); pass --kind")
	}
	return k, nil
}

func collectFiles(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if isYAML(path) {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no YAML files found")
	}
	return out, nil
}

func isYAML(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}
