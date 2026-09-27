package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	path "path/filepath"
	"testing"
	"time"
)

// a minimal asar: 4 byte pickle length, 4 byte json size, 4 bytes of padding,
// then the `{"files": ...}` header followed by enough bytes to clear isAsarFile
func fakeAsar() []byte {
	header := `{"files":{}}`
	body := make([]byte, 1_000_000)
	copy(body, header)

	prefix := make([]byte, 16)
	prefix[0] = 4

	return append(prefix, body...)
}

func withDownloadDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	target := path.Join(dir, "slipcord.asar")

	original := SlipcordDirectory
	SlipcordDirectory = target
	t.Cleanup(func() { SlipcordDirectory = original })

	return target
}

func TestFindAsarAsset(t *testing.T) {
	original := ReleaseData
	t.Cleanup(func() { ReleaseData = original })

	ReleaseData = GithubRelease{}
	ReleaseData.Assets = append(ReleaseData.Assets, struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	}{Name: "Slipcord-Setup.exe", DownloadURL: "http://example.com/setup"})

	if got := findAsarAsset(); got != "" {
		t.Fatalf("expected no asset, got %q", got)
	}

	ReleaseData.Assets = append(ReleaseData.Assets, struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	}{Name: "desktop.asar", DownloadURL: "http://example.com/desktop.asar"})

	if got := findAsarAsset(); got != "http://example.com/desktop.asar" {
		t.Fatalf("expected exact match to win, got %q", got)
	}

	// renamed asset should still be found, but never a legal notice
	ReleaseData.Assets = ReleaseData.Assets[:1]
	ReleaseData.Assets = append(ReleaseData.Assets, struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	}{Name: "desktop.asar.LEGAL.txt", DownloadURL: "http://example.com/legal"})

	if got := findAsarAsset(); got != "" {
		t.Fatalf("legal notice should not be used, got %q", got)
	}
}

func TestIsAsarFile(t *testing.T) {
	dir := t.TempDir()

	good := path.Join(dir, "good.asar")
	if err := os.WriteFile(good, fakeAsar(), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isAsarFile(good) {
		t.Error("expected valid asar to be detected")
	}

	bad := path.Join(dir, "bad.asar")
	if err := os.WriteFile(bad, []byte("<html>404: Not Found</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isAsarFile(bad) {
		t.Error("expected html error page to be rejected")
	}

	if isAsarFile(path.Join(dir, "missing.asar")) {
		t.Error("expected missing file to be rejected")
	}
}

func TestDownloadAsarRejectsErrorPage(t *testing.T) {
	target := withDownloadDir(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("nope"))
	}))
	defer server.Close()

	if err := downloadAsar(server.URL); err == nil {
		t.Fatal("expected error page download to fail")
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("failed download should not create the destination file")
	}

	entries, err := os.ReadDir(path.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("temp files should be cleaned up, found %d entries", len(entries))
	}
}

func TestDownloadAsarRejectsTruncatedBody(t *testing.T) {
	target := withDownloadDir(t)
	asar := fakeAsar()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(asar)))
		w.Write(asar[:len(asar)/2])
	}))
	defer server.Close()

	if err := downloadAsar(server.URL); err == nil {
		t.Fatal("expected truncated download to fail")
	}

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("truncated download should not replace the destination file")
	}
}

// rolling releases can 404 the asset we resolved; the installer must recover
// via the latest/download url and then via a re-resolved release
func TestInstallLatestBuildsFallsBackWhenAssetIsReplaced(t *testing.T) {
	target := withDownloadDir(t)
	asar := fakeAsar()

	var assetHits, latestHits int
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/release", func(w http.ResponseWriter, r *http.Request) {
		// the asset url from the release always 404s, like a replaced asset
		fmt.Fprintf(w, `{"name":"Slipcord abc123","assets":[{"name":"desktop.asar","browser_download_url":%q}]}`,
			server.URL+"/gone")
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		assetHits++
		http.NotFound(w, r)
	})
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		latestHits++
		w.Write(asar)
	})

	originalRelease, originalLatest, originalData, originalHash := releaseUrl, latestDownloadUrl, ReleaseData, LatestHash
	releaseUrl = server.URL + "/release"
	latestDownloadUrl = server.URL + "/latest"
	ReleaseData = GithubRelease{}
	LatestHash = "abc123"
	t.Cleanup(func() {
		releaseUrl, latestDownloadUrl, ReleaseData, LatestHash = originalRelease, originalLatest, originalData, originalHash
	})

	if err := installLatestBuilds(); err != nil {
		t.Fatalf("install should recover via the latest download url: %v", err)
	}

	if assetHits == 0 {
		t.Error("expected the replaced asset url to be tried")
	}
	if latestHits == 0 {
		t.Error("expected fallback to the latest download url")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(asar) {
		t.Errorf("installed %d bytes, expected %d", len(got), len(asar))
	}
	if InstalledHash != "abc123" {
		t.Errorf("installed hash is %q, expected abc123", InstalledHash)
	}
}

func TestInstallLatestBuildsKeepsExistingInstallOnFailure(t *testing.T) {
	target := withDownloadDir(t)

	existing := []byte("previous working install")
	if err := os.WriteFile(target, existing, 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	originalRelease, originalLatest, originalData, originalHash := releaseUrl, latestDownloadUrl, ReleaseData, LatestHash
	releaseUrl = server.URL
	latestDownloadUrl = server.URL
	ReleaseData = GithubRelease{
		Assets: []struct {
			Name        string `json:"name"`
			DownloadURL string `json:"browser_download_url"`
		}{{Name: "desktop.asar", DownloadURL: server.URL + "/gone"}},
	}
	t.Cleanup(func() {
		releaseUrl, latestDownloadUrl, ReleaseData, LatestHash = originalRelease, originalLatest, originalData, originalHash
	})

	// shorten the backoff so the test doesn't sit through the real delays
	originalDelay := downloadRetryDelay
	downloadRetryDelay = time.Millisecond
	t.Cleanup(func() { downloadRetryDelay = originalDelay })

	if err := installLatestBuilds(); err == nil {
		t.Fatal("expected install to fail when every url 404s")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(existing) {
		t.Error("failed install must not clobber the working install")
	}
}
