/*
 * SPDX-License-Identifier: GPL-3.0
 * Vencord Installer, a cross platform gui/cli app for installing Vencord
 * Copyright (c) 2023 Vendicated and Vencord contributors
 */

package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	path "path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// vars instead of the constants so tests can point them at a stub server
var (
	downloadRetryDelay = 2 * time.Second

	releaseUrl         = ReleaseUrl
	releaseUrlFallback = ReleaseUrlFallback
	latestDownloadUrl  = "https://github.com/Slipcords/Slipped/releases/latest/download/desktop.asar"
)

type GithubRelease struct {
	Name    string `json:"name"`
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var ReleaseData GithubRelease
var GithubError error
var GithubDoneChan chan bool

var InstalledHash = "None"
var LatestHash = "Unknown"
var IsDevInstall bool

func GetGithubRelease(url, fallbackUrl string) (*GithubRelease, error) {
	Log.Debug("Fetching", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		Log.Error("Failed to create Request", err)
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		Log.Error("Failed to send Request", err)
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode >= 300 {
		isRateLimitedOrBlocked := res.StatusCode == 401 || res.StatusCode == 403 || res.StatusCode == 429
		triedFallback := url == fallbackUrl

		if isRateLimitedOrBlocked && !triedFallback {
			Log.Error(fmt.Sprintf("Failed to fetch %s (status code %d). Trying fallback url %s", url, res.StatusCode, fallbackUrl))
			return GetGithubRelease(fallbackUrl, fallbackUrl)
		}

		err = errors.New(res.Status)
		Log.Error(url, "returned Non-OK status", GithubError)
		return nil, err
	}

	var data GithubRelease

	if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
		Log.Error("Failed to decode GitHub JSON Response", err)
		return nil, err
	}

	return &data, nil
}

func InitGithubDownloader() {
	GithubDoneChan = make(chan bool, 1)

	IsDevInstall = os.Getenv("SLIPCORD_DEV_INSTALL") == "1"
	Log.Debug("Is Dev Install: ", IsDevInstall)
	if IsDevInstall {
		GithubDoneChan <- true
		return
	}

	go func() {
		// Make sure UI updates once the request either finished or failed
		defer func() {
			GithubDoneChan <- GithubError == nil
		}()

		data, err := GetGithubRelease(releaseUrl, releaseUrlFallback)
		if err != nil {
			GithubError = err
			return
		}

		ReleaseData = *data

		i := strings.LastIndex(data.Name, " ") + 1
		LatestHash = data.Name[i:]
		Log.Debug("Finished fetching GitHub Data")
		Log.Debug("Latest hash is", LatestHash, "Local Install is", Ternary(LatestHash == InstalledHash, "up to date!", "outdated!"))
	}()

	// either .asar file or directory with main.js file (in DEV)
	SlipcordFile := SlipcordDirectory

	stat, err := os.Stat(SlipcordFile)
	if err != nil {
		return
	}

	// dev
	if stat.IsDir() {
		SlipcordFile = path.Join(SlipcordFile, "main.js")
	}

	// Check hash of installed version if exists
	b, err := os.ReadFile(SlipcordFile)
	if err != nil {
		return
	}

	Log.Debug("Found existing Slipcord Install. Checking for hash...")

	re := regexp.MustCompile(`// Slipcord (\w+)`)
	match := re.FindSubmatch(b)
	if match != nil {
		InstalledHash = string(match[1])
		Log.Debug("Existing hash is", InstalledHash)

	} else {
		Log.Debug("Didn't find hash")

	}
}

// the release is republished on every build, so the asset we resolved can be replaced
// (and its url start returning 404) while we are installing. Retry with a fresh lookup.
const downloadAttempts = 3

// asar files start with a 16 byte pickle header followed by a JSON file table
func isAsarFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.Size() < 1_000_000 {
		return false
	}

	header := make([]byte, 24)
	if _, err := f.Read(header); err != nil {
		return false
	}

	pickleSize := binary.LittleEndian.Uint32(header[0:4])
	return pickleSize == 4 && string(header[16:24]) == `{"files"`
}

func findAsarAsset() string {
	for _, ass := range ReleaseData.Assets {
		if ass.Name == "desktop.asar" {
			return ass.DownloadURL
		}
	}

	// name may change one day, any real asar asset will do
	for _, ass := range ReleaseData.Assets {
		if strings.HasSuffix(ass.Name, ".asar") && !strings.Contains(ass.Name, "LEGAL") {
			return ass.DownloadURL
		}
	}

	return ""
}

// re-read the release so a replaced asset gets resolved again
func refreshReleaseData() {
	data, err := GetGithubRelease(releaseUrl, releaseUrlFallback)
	if err != nil {
		Log.Debug("Failed to refresh release data:", err)
		return
	}

	ReleaseData = *data
	LatestHash = data.Name[strings.LastIndex(data.Name, " ")+1:]
}

// download to a temp file first so a failed or partial download can never clobber the
// current working install
func downloadAsar(url string) (err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", url, res.Status)
	}

	tmp, err := os.CreateTemp(path.Dir(SlipcordDirectory), ".slipcord-*.asar")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	defer func() {
		tmp.Close()
		if err != nil {
			os.Remove(tmpName)
		}
	}()

	written, err := io.Copy(tmp, res.Body)
	if err != nil {
		return err
	}

	if expected := res.Header.Get("Content-Length"); expected != "" && expected != strconv.FormatInt(written, 10) {
		return fmt.Errorf("unexpected end of input: Content-Length was %s, but only read %d", expected, written)
	}

	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}

	if !isAsarFile(tmpName) {
		return fmt.Errorf("%s did not return a valid asar archive", url)
	}

	return os.Rename(tmpName, SlipcordDirectory)
}

func installLatestBuilds() (retErr error) {
	Log.Debug("Installing latest builds...")

	if IsDevInstall {
		Log.Debug("Skipping due to dev install")
		return
	}

	resolved := findAsarAsset()
	if resolved == "" {
		refreshReleaseData()
		resolved = findAsarAsset()
	}
	if resolved == "" {
		retErr = errors.New("Didn't find desktop.asar download link")
		Log.Error(retErr)
		return
	}

	// the canonical latest-download url resolves to whatever asset is current, so it keeps
	// working after the release replaces the asset we resolved above
	urls := []string{resolved, latestDownloadUrl}

	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		for _, url := range urls {
			Log.Debug("Downloading", url)

			if err := downloadAsar(url); err != nil {
				lastErr = err
				Log.Debug("Download failed:", err)
				continue
			}

			_ = FixOwnership(SlipcordDirectory)
			InstalledHash = LatestHash
			return
		}

		if attempt < downloadAttempts {
			Log.Debug("Retrying download in", downloadRetryDelay*time.Duration(attempt))
			time.Sleep(downloadRetryDelay * time.Duration(attempt))
			refreshReleaseData()
			urls = []string{findAsarAsset(), urls[1]}
		}
	}

	retErr = fmt.Errorf("Failed to download desktop.asar after %d attempts: %v. The release is probably mid-upload, try again in a minute.", downloadAttempts, lastErr)
	Log.Error(retErr)
	return
}
