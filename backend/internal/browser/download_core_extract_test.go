package browser

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCoreArchivePreservesAppBundle(t *testing.T) {
	for _, format := range []string{".zip", ".tar.gz"} {
		for _, root := range []string{"", "cloakbrowser-darwin-arm64/"} {
			t.Run(format+root, func(t *testing.T) {
				archive := filepath.Join(t.TempDir(), "cloakbrowser"+format)
				files := map[string]string{
					root + "Chromium.app/Contents/MacOS/Chromium": "synthetic executable",
					root + "Chromium.app/Contents/Info.plist":     "synthetic plist",
				}
				writeCoreTestArchive(t, archive, format, files)
				dest := t.TempDir()
				if err := extractCoreArchiveAndStripRoot(archive, dest, func(int, string) {}); err != nil {
					t.Fatal(err)
				}
				for _, rel := range []string{"Chromium.app/Contents/MacOS/Chromium", "Chromium.app/Contents/Info.plist"} {
					if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
						t.Fatalf("bundle file lost: %s: %v", rel, err)
					}
				}
			})
		}
	}
}

func TestExtractCoreArchiveStillStripsDistributionDirectory(t *testing.T) {
	for _, format := range []string{".zip", ".tar.gz"} {
		t.Run(format, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "core"+format)
			writeCoreTestArchive(t, archive, format, map[string]string{"distribution/chrome": "synthetic executable", "distribution/resources.pak": "resources"})
			dest := t.TempDir()
			if err := extractCoreArchiveAndStripRoot(archive, dest, func(int, string) {}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dest, "chrome")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func writeCoreTestArchive(t *testing.T, name, format string, files map[string]string) {
	t.Helper()
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if format == ".zip" {
		writer := zip.NewWriter(file)
		for name, content := range files {
			entry, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	compressed := gzip.NewWriter(file)
	writer := tar.NewWriter(compressed)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStripAppRootStillValidatesPath(t *testing.T) {
	for _, root := range []string{"../Chromium.app", "/Chromium.app", "foo/../../Chromium.app"} {
		if err := stripSingleExtractedRoot(t.TempDir(), map[string]struct{}{root: {}}); err == nil {
			t.Fatalf("accepted escaping app root %s", root)
		}
	}
}
