package browser

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
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

func TestExtractCoreArchiveDetectsGzipTarWithoutFilenameExtension(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "download_no_extension")
	writeCoreTestArchive(t, archive, ".tar.gz", map[string]string{
		"distribution/Chromium.app/Contents/MacOS/Chromium": "synthetic executable",
	})
	dest := t.TempDir()
	if err := extractCoreArchiveAndStripRoot(archive, dest, func(int, string) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "Chromium.app", "Contents", "MacOS", "Chromium")); err != nil {
		t.Fatalf("gzip tar without extension was not extracted: %v", err)
	}
}

func TestCoreArchiveTempPatternPreservesDMGExtension(t *testing.T) {
	if got := coreArchiveTempPattern("https://example.com/ungoogled-chromium_142.0.7444.175-1.1_macos.dmg"); got != "download_*.dmg" {
		t.Fatalf("temp pattern = %q, want download_*.dmg", got)
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

func TestDMGFormatTakesPriorityOverCompressedPayloadHeader(t *testing.T) {
	// UDIF disk images may begin with compressed disk blocks. This release's
	// first bytes are BZh9, but its final 512-byte block identifies it as DMG.
	for _, name := range []string{"ungoogled-chromium_macos.dmg", "download_no_extension"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			data := make([]byte, 1024)
			copy(data, "BZh91AY&SY")
			copy(data[len(data)-512:], "koly")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := detectCoreArchiveFormat(path)
			if err != nil || got != coreArchiveDMG {
				t.Fatalf("format = %q, err = %v; want dmg", got, err)
			}
		})
	}
}

// Opt-in regression for real publisher DMGs; never fetches or launches a browser.
func TestExtractRealCoreDMG(t *testing.T) {
	archive := os.Getenv("ANT_BROWSER_TEST_DMG")
	if archive == "" || runtime.GOOS != "darwin" {
		t.Skip("set ANT_BROWSER_TEST_DMG on macOS to verify a real DMG")
	}
	format, err := detectCoreArchiveFormat(archive)
	if err != nil || format != coreArchiveDMG {
		t.Fatalf("format = %q, err = %v; want dmg", format, err)
	}
	dest := t.TempDir()
	if err := extractCoreArchiveAndStripRoot(archive, dest, func(int, string) {}); err != nil {
		t.Fatal(err)
	}
	exe, _, ok := FindCoreExecutable(dest)
	if !ok {
		t.Fatal("no browser executable in extracted DMG")
	}
	info, err := os.Stat(exe)
	if err != nil || info.Mode().Perm()&0111 == 0 {
		t.Fatal("extracted browser is not executable")
	}
	// Framework symlinks must resolve inside the copied bundle rather than depend
	// on the image remaining mounted.
	links := 0
	err = filepath.WalkDir(filepath.Dir(filepath.Dir(exe)), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			links++
			if _, err := os.Stat(path); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("broken app bundle link after unmount: %v", err)
	}
	t.Logf("extracted browser executable %s; verified %d bundle symlinks", filepath.Base(exe), links)
}
