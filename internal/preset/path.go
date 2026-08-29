package preset

import (
	"path"
	"strings"
)

// KindFromPath infers a preset's kind from its location in the presets
// repository, following the path-based layout in docs/presets-repo-layout.md:
//
//	vendors/<slug>/printers/<id>.yaml   -> printer
//	vendors/<slug>/filaments/<id>.yaml  -> filament
//	processes/<id>.yaml                 -> process
//
// It reports ok=false when the path is in neither tree, which is itself a
// rejectable state (a file outside both trees is invalid).
func KindFromPath(p string) (Kind, bool) {
	segments := strings.Split(path.Clean(strings.ReplaceAll(p, "\\", "/")), "/")
	for _, seg := range segments {
		switch seg {
		case "printers":
			return KindPrinter, true
		case "filaments":
			return KindFilament, true
		case "processes":
			return KindProcess, true
		}
	}
	return "", false
}
