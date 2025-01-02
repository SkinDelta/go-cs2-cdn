// dependencies/dependencies.go
package dependencies

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Tool represents a downloadable tool with its GitHub repository and asset details.
type Tool struct {
	Name            string
	RepoOwner       string
	RepoName        string
	AssetNameSuffix string
	ExecutableName  string
}

// List of tools to manage
var tools = []Tool{
	{
		Name:            "DepotDownloader",
		RepoOwner:       "SteamRE",
		RepoName:        "DepotDownloader",
		AssetNameSuffix: "DepotDownloader-linux-x64.zip",
		ExecutableName:  "DepotDownloader",
	},
	{
		Name:            "ValveResourceFormat",
		RepoOwner:       "ValveResourceFormat",
		RepoName:        "ValveResourceFormat",
		AssetNameSuffix: "cli-linux-x64.zip",
		ExecutableName:  "Source2Viewer-CLI",
	},
}

// EnsureTools ensures that all required tools are present.
// If a tool is missing, it downloads and installs it.
func EnsureTools() error {
	toolsDir := "tools"

	// Create tools directory if it doesn't exist
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		return fmt.Errorf("failed to create tools directory: %w", err)
	}

	for _, tool := range tools {
		execPath := filepath.Join(toolsDir, tool.ExecutableName)
		if !fileExists(execPath) {
			log.Printf("Tool %s not found. Downloading...", tool.Name)
			if err := downloadAndInstallTool(tool, toolsDir); err != nil {
				return fmt.Errorf("failed to download %s: %w", tool.Name, err)
			}
			log.Printf("Tool %s downloaded and installed successfully.", tool.Name)
		} else {
			log.Printf("Tool %s already exists. Skipping download.", tool.Name)
		}
	}

	// Make the tools executable
	for _, tool := range tools {
		execPath := filepath.Join(toolsDir, tool.ExecutableName)
		if err := os.Chmod(execPath, 0755); err != nil {
			return fmt.Errorf("failed to set executable permissions for %s: %w", tool.Name, err)
		}
	}

	return nil
}

func checkOS() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macos"
	default:
		return "linux"
	}
}

func checkGOARCH() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "arm":
		return "arm"
	default:
		return "x64"
	}
}

// downloadAndInstallTool downloads the latest release asset matching the tool's AssetNameSuffix,
// extracts it, and places the executable in the tools directory.
func downloadAndInstallTool(tool Tool, toolsDir string) error {
	// Determine the OS-specific asset name suffix
	tool.AssetNameSuffix = strings.ReplaceAll(tool.AssetNameSuffix, "linux", checkOS())
	tool.AssetNameSuffix = strings.ReplaceAll(tool.AssetNameSuffix, "x64", checkGOARCH())

	// Step 1: Get the latest release
	releaseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", tool.RepoOwner, tool.RepoName)
	client := &http.Client{}
	req, err := http.NewRequest("GET", releaseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch latest release: status code %d", resp.StatusCode)
	}

	// Parse the JSON response to find the asset URL
	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to decode release JSON: %w", err)
	}

	assetURL := ""
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, tool.AssetNameSuffix) {
			assetURL = asset.BrowserDownloadURL
			break
		}
	}

	if assetURL == "" {
		return fmt.Errorf("asset with suffix %s not found in latest release", tool.AssetNameSuffix)
	}

	// Step 2: Download the asset
	log.Printf("Downloading %s from %s", tool.Name, assetURL)
	assetResp, err := http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download asset: %w", err)
	}
	defer assetResp.Body.Close()

	if assetResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download asset: status code %d", assetResp.StatusCode)
	}

	// Step 3: Extract all files from the ZIP archive
	log.Printf("Extracting %s...", tool.Name)
	if err := extractZip(assetResp.Body, toolsDir); err != nil {
		return fmt.Errorf("failed to extract zip: %w", err)
	}
	return nil
}

// extractZip extracts all files from a ZIP archive and saves them to the tools directory.
// It preserves the internal directory structure of the ZIP archive.
func extractZip(zipReader io.Reader, toolsDir string) error {
	// Read all data from zipReader
	data, err := io.ReadAll(zipReader)
	if err != nil {
		return fmt.Errorf("failed to read zip data: %w", err)
	}

	// Use bytes.NewReader to handle binary data correctly
	readerAt := bytes.NewReader(data)
	zr, err := zip.NewReader(readerAt, int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to create zip reader: %w", err)
	}

	// Iterate through the files in the archive
	for _, file := range zr.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Define the full path for the output file
		outPath := filepath.Join(toolsDir, file.Name)

		// Prevent Zip Slip Vulnerability
		if !strings.HasPrefix(outPath, filepath.Clean(toolsDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", outPath)
		}

		// Create necessary directories
		if err := os.MkdirAll(filepath.Dir(outPath), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directories for %s: %w", outPath, err)
		}

		// Open the file inside the ZIP archive
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip file %s: %w", file.Name, err)
		}

		// Create the output file
		outFile, err := os.Create(outPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", outPath, err)
		}

		// Copy the file contents
		if _, err := io.Copy(outFile, rc); err != nil {
			rc.Close()
			outFile.Close()
			return fmt.Errorf("failed to extract file %s: %w", outPath, err)
		}

		// Close the file handles
		rc.Close()
		outFile.Close()

		// If the file is the executable, set executable permissions
		if filepath.Base(outPath) == "DepotDownloader" {
			if err := os.Chmod(outPath, 0755); err != nil {
				return fmt.Errorf("failed to set executable permissions for %s: %w", outPath, err)
			}
		}

		log.Printf("Extracted %s to %s", file.Name, outPath)
	}

	return nil // Successfully extracted all files
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// Release represents the JSON structure of a GitHub release.
type Release struct {
	Assets []Asset `json:"assets"`
}

// Asset represents a single asset in a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}
