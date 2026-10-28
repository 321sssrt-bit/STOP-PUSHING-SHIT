// Developer-only Codex packaging; no installer or downloads.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string) []byte { b, e := os.ReadFile(path); check(e); return b }

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run ./tools/package <binary-directory> (from repository root)")
	}
	pluginRoot := filepath.Join("plugins", "codex")
	outputRoot := filepath.Join("dist", "codex")
	manifest := read(filepath.Join(pluginRoot, ".codex-plugin/plugin.json"))
	var plugin struct{ Version string }
	check(json.Unmarshal(manifest, &plugin))
	check(os.MkdirAll(outputRoot, 0755))
	var results []map[string]any
	platforms := []string{"darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64", "windows-amd64"}
	for _, target := range append(platforms, "universal") {
		suffix := ""
		if strings.HasPrefix(target, "windows-") {
			suffix = ".exe"
		}
		var binary []byte
		if target != "universal" {
			binary = read(filepath.Join(os.Args[1], "guard-"+target+suffix))
		}
		name := "plugin/bin/issue-pr-guard" + suffix
		contents := map[string][]byte{
			".agents/plugins/marketplace.json": read(filepath.Join(pluginRoot, "marketplace.json")),
			"plugin/.codex-plugin/plugin.json": manifest,
			"INSTALL.md":                       read(filepath.Join(pluginRoot, "INSTALL.md")),
			"LICENSE":                          read("LICENSE"),
			"THIRD_PARTY_NOTICES.txt":          read("THIRD_PARTY_NOTICES.txt"),
			name:                               binary,
		}
		if target == "universal" {
			delete(contents, name)
			for _, platform := range platforms {
				ext := ""
				if strings.HasPrefix(platform, "windows-") {
					ext = ".exe"
				}
				contents["plugin/bin/"+platform+"/issue-pr-guard"+ext] = read(filepath.Join(os.Args[1], "guard-"+platform+ext))
			}
		}
		for _, f := range []string{"hooks.json", "run.sh", "run.ps1"} {
			contents["plugin/hooks/"+f] = read(filepath.Join(pluginRoot, "hooks", f))
		}
		archive := "STOP-PUSHING-SHIT-" + plugin.Version + "-" + target + ".zip"
		file, err := os.Create(filepath.Join(outputRoot, archive))
		check(err)
		out := zip.NewWriter(file)
		for path, data := range contents {
			h := &zip.FileHeader{Name: path, Method: zip.Deflate}
			h.SetMode(0644)
			if strings.HasPrefix(path, "plugin/bin/") || strings.HasSuffix(path, ".sh") {
				h.SetMode(0755)
			}
			entry, err := out.CreateHeader(h)
			check(err)
			_, err = entry.Write(data)
			check(err)
		}
		check(out.Close())
		check(file.Close())
		checkArchive, err := zip.OpenReader(filepath.Join(outputRoot, archive))
		check(err)
		if len(checkArchive.File) != len(contents) {
			panic("unexpected package contents")
		}
		var entries []string
		for _, entry := range checkArchive.File {
			input, err := entry.Open()
			check(err)
			data, err := io.ReadAll(input)
			check(err)
			check(input.Close())
			if !bytes.Equal(data, contents[entry.Name]) || strings.HasSuffix(entry.Name, ".go") || strings.HasSuffix(entry.Name, ".py") {
				panic("package verification failed: " + entry.Name)
			}
			if strings.HasPrefix(entry.Name, "plugin/bin/") && entry.Mode().Perm()&0111 == 0 {
				panic("missing executable permission")
			}
			entries = append(entries, entry.Name)
		}
		check(checkArchive.Close())
		info, err := os.Stat(filepath.Join(outputRoot, archive))
		check(err)
		results = append(results, map[string]any{"target": target, "archive": archive, "bytes": info.Size(), "executable_bytes": len(binary), "entries": entries})
	}
	data, err := json.MarshalIndent(results, "", "  ")
	check(err)
	fmt.Println(string(data))
}
