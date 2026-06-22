package collect

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func execCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

func splitFields(s string) []string {
	return strings.Fields(s)
}

var (
	homePathRe     = regexp.MustCompile(`/home/[^/\s]+`)
	rootUserPathRe = regexp.MustCompile(`/root`)
)

// redactPath replaces user-specific path segments with generic placeholders.
// /home/alice/project/app.js → /home/<user>/project/app.js
// The filename itself is preserved; only the home directory owner is masked.
func redactPath(path string) string {
	path = homePathRe.ReplaceAllStringFunc(path, func(m string) string {
		return "/home/<user>"
	})
	path = rootUserPathRe.ReplaceAllString(path, "/root")
	return path
}

// redactPathToBasename reduces a full path to just the filename.
// Used in --safe mode to avoid leaking directory structure.
func redactPathToBasename(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
