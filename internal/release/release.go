// Package release renders the package-manager files (Homebrew formula, Scoop
// manifest) published alongside a GitHub release.
package release

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Repo is the GitHub repository that hosts the release archives.
const Repo = "synodlang/compiler"

var (
	tagRE = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	sumRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ParseSums reads sha256sum output ("<hex>  <name>", optionally "*name") into
// a map from archive name to lowercase hex digest.
func ParseSums(r io.Reader) (map[string]string, error) {
	sums := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		hash, name, ok := strings.Cut(line, " ")
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		hash = strings.ToLower(hash)
		if !ok || name == "" || !sumRE.MatchString(hash) {
			return nil, fmt.Errorf("SHA256SUMS line %d: malformed", n)
		}
		sums[name] = hash
	}
	return sums, sc.Err()
}

// ArchiveName returns the release archive name for a tag, OS and arch.
// Windows archives are zips, everything else is tar.gz.
func ArchiveName(tag, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("synod-%s-%s-%s.%s", tag, goos, goarch, ext)
}

func downloadURL(tag, name string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", Repo, tag, name)
}

// lookup returns the URL and digest of an archive, failing if it has no checksum.
func lookup(tag string, sums map[string]string, goos, goarch string) (url, sum string, err error) {
	name := ArchiveName(tag, goos, goarch)
	sum, ok := sums[name]
	if !ok {
		return "", "", fmt.Errorf("no checksum for %s", name)
	}
	return downloadURL(tag, name), sum, nil
}

func checkTag(tag string) error {
	if !tagRE.MatchString(tag) {
		return fmt.Errorf("bad tag %q: want vX.Y.Z[-pre]", tag)
	}
	return nil
}

const formulaTmpl = `class Synod < Formula
  desc "Compiler for the Synod language"
  homepage "https://github.com/%[1]s"
  version "%[2]s"
  license "Apache-2.0"

  on_macos do
    url "%[3]s"
    sha256 "%[4]s"
  end

  on_linux do
    on_arm do
      url "%[5]s"
      sha256 "%[6]s"
    end
    on_intel do
      url "%[7]s"
      sha256 "%[8]s"
    end
  end

  def install
    bin.install "synod"
  end

  test do
    assert_match "language", shell_output("#{bin}/synod version")
  end
end
`

// Formula renders Formula/synod.rb for the given tag.
func Formula(tag string, sums map[string]string) (string, error) {
	if err := checkTag(tag); err != nil {
		return "", err
	}
	uURL, uSum, err := lookup(tag, sums, "darwin", "universal")
	if err != nil {
		return "", err
	}
	aURL, aSum, err := lookup(tag, sums, "linux", "arm64")
	if err != nil {
		return "", err
	}
	iURL, iSum, err := lookup(tag, sums, "linux", "amd64")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(formulaTmpl, Repo, strings.TrimPrefix(tag, "v"), uURL, uSum, aURL, aSum, iURL, iSum), nil
}

type scoopArch struct {
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

type scoopAutoArch struct {
	URL string `json:"url"`
}

// Manifest renders synod.json for the Scoop bucket.
func Manifest(tag string, sums map[string]string) (string, error) {
	if err := checkTag(tag); err != nil {
		return "", err
	}
	aURL, aSum, err := lookup(tag, sums, "windows", "amd64")
	if err != nil {
		return "", err
	}
	rURL, rSum, err := lookup(tag, sums, "windows", "arm64")
	if err != nil {
		return "", err
	}
	base := fmt.Sprintf("https://github.com/%s/releases/download/v$version", Repo)
	m := struct {
		Version      string               `json:"version"`
		Description  string               `json:"description"`
		Homepage     string               `json:"homepage"`
		License      string               `json:"license"`
		Architecture map[string]scoopArch `json:"architecture"`
		Bin          string               `json:"bin"`
		Checkver     map[string]string    `json:"checkver"`
		Autoupdate   map[string]any       `json:"autoupdate"`
	}{
		Version:     strings.TrimPrefix(tag, "v"),
		Description: "Compiler for the Synod language",
		Homepage:    "https://github.com/" + Repo,
		License:     "Apache-2.0",
		Architecture: map[string]scoopArch{
			"64bit": {aURL, aSum},
			"arm64": {rURL, rSum},
		},
		Bin:      "synod.exe",
		Checkver: map[string]string{"github": "https://github.com/" + Repo},
		Autoupdate: map[string]any{
			"architecture": map[string]scoopAutoArch{
				"64bit": {base + "/synod-v$version-windows-amd64.zip"},
				"arm64": {base + "/synod-v$version-windows-arm64.zip"},
			},
			"hash": map[string]string{"url": base + "/SHA256SUMS"},
		},
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}
