package cli

import (
	"archive/tar"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesWorkspaceAndRefusesExistingContent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "demo-a7x2m")
	output := execute(t, "init", target, "--arch", "amd64,arm64")
	if !strings.Contains(output, "amd64,arm64") {
		t.Fatalf("init output = %q", output)
	}
	for _, name := range []string{"manifest.yaml", "compose.amd64.yaml", "compose.arm64.yaml", "about.md"} {
		if _, err := os.Stat(filepath.Join(target, name)); err != nil {
			t.Fatalf("init missing %s: %v", name, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(target, "manifest.yaml"))
	if err != nil || !strings.Contains(string(manifest), "lifecycle: {restart: unless-stopped}") {
		t.Fatalf("generated manifest lifecycle = %q, err = %v", manifest, err)
	}
	compose, err := os.ReadFile(filepath.Join(target, "compose.amd64.yaml"))
	if err != nil || strings.Contains(string(compose), "restart:") {
		t.Fatalf("generated source compose restart = %q, err = %v", compose, err)
	}
	command := NewCommand("test")
	command.SetArgs([]string{"init", target})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "必须为空") {
		t.Fatalf("second init error = %v", err)
	}
}

func TestPackValidateAndInspectShareContractRules(t *testing.T) {
	workspace := validWorkspace(t)
	manifestPath := filepath.Join(workspace, "manifest.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest = bytes.Replace(manifest, []byte("    lifecycle: {restart: unless-stopped}\n    endpoints:"), []byte("    lifecycle: {restart: unless-stopped}\n    capabilities:\n      compose-override: {}\n    endpoints:"), 1)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(t.TempDir(), "demo.mpk")
	if output := execute(t, "pack", workspace, "-o", packagePath); !strings.Contains(output, packagePath) {
		t.Fatalf("pack output = %q", output)
	}
	if output := execute(t, "validate", packagePath); !strings.Contains(output, "校验通过") || !strings.Contains(output, "demo-a7x2m/1.0.0") {
		t.Fatalf("validate output = %q", output)
	}
	if output := execute(t, "inspect", packagePath); !strings.Contains(output, `"sha256"`) ||
		!strings.Contains(output, `"displayName": "Demo"`) || !strings.Contains(output, `"compose-override"`) ||
		!strings.Contains(output, `"imageVolumeMatches"`) || !strings.Contains(output, `"matched": true`) ||
		!strings.Contains(output, `"volumes": [`) {
		t.Fatalf("inspect output = %q", output)
	}
}

// TestValidateReportsStableGateRule 验证 CLI 直接展示共享门禁的稳定规则编号。
func TestValidateReportsStableGateRule(t *testing.T) {
	workspace := validWorkspace(t)
	composePath := filepath.Join(workspace, "compose.amd64.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  web:\n    image: demo-a7x2m/web:1.0.0\n    ports: [\"8080:8080\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := NewCommand("test")
	command.SetArgs([]string{"validate", workspace})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "MPK-COMPOSE-PORTS") {
		t.Fatalf("validate error = %v, want MPK-COMPOSE-PORTS", err)
	}
}

func TestValidateAcceptsOverlayDirectoryMount(t *testing.T) {
	workspace := validWorkspace(t)
	manifestPath := filepath.Join(workspace, "manifest.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest = bytes.Replace(manifest, []byte("  web:\n    endpoints:"), []byte("  web:\n    lifecycle: {restart: unless-stopped}\n    endpoints:"), 1)
	manifest = bytes.Replace(manifest,
		[]byte("      - {source: data, subpath: app, target: /var/lib/app}\n"),
		[]byte("      - {source: data, subpath: app, target: /var/lib/app}\n      - {source: overlay, subpath: static, target: /usr/share/app, read_only: true}\n"), 1)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "overlay", "static", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "overlay", "static", "nested", "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := NewCommand("test")
	command.SetArgs([]string{"validate", workspace})
	if err := command.Execute(); err != nil {
		t.Fatalf("validate overlay directory error = %v", err)
	}
}

func TestPackRejectsOutputInsideSource(t *testing.T) {
	workspace := validWorkspace(t)
	command := NewCommand("test")
	command.SetArgs([]string{"pack", workspace, "-o", filepath.Join(workspace, "demo.mpk")})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "不能位于") {
		t.Fatalf("pack error = %v", err)
	}
}

func TestInspectImageArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.tar")
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	writeArchiveEntry(t, writer, "config.json", `{"architecture":"arm64"}`)
	writeArchiveEntry(t, writer, "manifest.json", `[{"Config":"config.json","RepoTags":["demo-a7x2m/web:1.0.0"]}]`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, archive.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	output := execute(t, "inspect", path)
	if !strings.Contains(output, `"repoTag": "demo-a7x2m/web:1.0.0"`) || !strings.Contains(output, `"architecture": "arm64"`) {
		t.Fatalf("inspect output = %q", output)
	}
}

func execute(t *testing.T, arguments ...string) string {
	t.Helper()
	var output bytes.Buffer
	command := NewCommand("test")
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(arguments)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func validWorkspace(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifest.yaml", "schema_version: 1\nid: demo-a7x2m\nversion: 1.0.0\ndisplay_name: Demo\ntype: web\narch: [amd64]\ndependencies: []\nservices:\n  web:\n    lifecycle: {restart: unless-stopped}\n    endpoints:\n      - {name: web, protocol: http, container_port: 8080}\n    mounts:\n      - {source: data, subpath: app, target: /var/lib/app}\n")
	write("compose.amd64.yaml", "services:\n  web:\n    image: demo-a7x2m/web:1.0.0\n")
	var image bytes.Buffer
	writer := tar.NewWriter(&image)
	writeArchiveEntry(t, writer, "config.json", `{"architecture":"amd64","os":"linux","config":{"Volumes":{"/var/lib/app":{}}}}`)
	manifest := []byte(`[{"Config":"config.json","RepoTags":["demo-a7x2m/web:1.0.0"]}]`)
	if err := writer.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(manifest); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "images", "amd64", "web.tar")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, image.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	writeBinary := func(name string, content []byte) {
		t.Helper()
		assetPath := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(assetPath, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeBinary("icons/icon-64.png", pngImage(t, 64, 64))
	writeBinary("icons/icon-256.png", pngImage(t, 256, 256))
	return directory
}

func pngImage(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			value.Set(x, y, color.RGBA{R: 37, G: 99, B: 235, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func writeArchiveEntry(t *testing.T, writer *tar.Writer, name, content string) {
	t.Helper()
	data := []byte(content)
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
}
