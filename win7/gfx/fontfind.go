package gfx

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ==========================
// ===  Font discovery    ===
// ==========================

// fontDirs lists system font folders in search order; deep marks
// folders walked recursively (Linux keeps fonts in nested folders).
func fontDirs() []struct {
	dir  string
	deep bool
} {
	win := os.Getenv("WINDIR")
	if win == "" {
		win = `C:\Windows`
	}
	home, _ := os.UserHomeDir()
	type d = struct {
		dir  string
		deep bool
	}
	out := []d{
		{filepath.Join(win, "Fonts"), false},
		{"/System/Library/Fonts", false},
		{"/System/Library/Fonts/Supplemental", false},
		{"/Library/Fonts", false},
	}
	if home != "" {
		out = append(out, d{filepath.Join(home, "Library", "Fonts"), false})
	}
	out = append(out, d{"/usr/share/fonts", true}, d{"/usr/local/share/fonts", true})
	if home != "" {
		out = append(out, d{filepath.Join(home, ".local", "share", "fonts"), true})
	}
	return out
}

// FindFont returns the path of the first of names (file names, matched
// case-insensitively) found in the system font folders, or "".
func FindFont(names ...string) string {
	if len(names) == 0 {
		return ""
	}
	want := map[string]bool{}
	for _, n := range names {
		want[strings.ToLower(n)] = true
	}
	found := map[string]string{}
	for _, d := range fontDirs() {
		if d.deep {
			_ = filepath.WalkDir(d.dir, func(p string, e fs.DirEntry, err error) error {
				if err != nil {
					return nil // unreadable folders are skipped, not fatal
				}
				if !e.IsDir() {
					k := strings.ToLower(e.Name())
					if want[k] && found[k] == "" {
						found[k] = p
					}
				}
				return nil
			})
			continue
		}
		ents, err := os.ReadDir(d.dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			k := strings.ToLower(e.Name())
			if want[k] && found[k] == "" && !e.IsDir() {
				found[k] = filepath.Join(d.dir, e.Name())
			}
		}
	}
	for _, n := range names {
		if p := found[strings.ToLower(n)]; p != "" {
			return p
		}
	}
	return ""
}
