package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/humanetools/orbit/internal/version"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update orbit to the latest version",
	RunE:  runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func runUpdate(cmd *cobra.Command, args []string) error {
	fmt.Println("Checking for updates...")

	resp, err := http.Get("https://api.github.com/repos/humanetools/orbit/releases/latest")
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}
	defer resp.Body.Close()

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("parse release: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(version.Version, "v")

	if latest == current {
		fmt.Printf("Already up to date (v%s)\n", current)
		return nil
	}

	fmt.Printf("Updating v%s → v%s\n", current, latest)

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	filename := fmt.Sprintf("orbit_%s_%s_%s.tar.gz", latest, goos, goarch)
	url := fmt.Sprintf("https://github.com/humanetools/orbit/releases/download/v%s/%s", latest, filename)

	dlResp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", dlResp.Status)
	}

	gz, err := gzip.NewReader(dlResp.Body)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("orbit binary not found in archive")
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		if hdr.Name == "orbit" || hdr.Name == "orbit.exe" {
			break
		}
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current binary: %w", err)
	}

	tmpFile := execPath + ".tmp"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(f, tr); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return fmt.Errorf("write binary: %w", err)
	}
	f.Close()

	if err := os.Rename(tmpFile, execPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Printf("Updated to v%s\n", latest)
	return nil
}
