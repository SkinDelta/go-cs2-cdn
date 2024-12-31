// main.go
package main

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/SkinDelta/go-cs2-cdn/internal/cmdrunner"
	"github.com/SkinDelta/go-cs2-cdn/internal/dependencies"
	"github.com/SkinDelta/go-cs2-cdn/internal/parser"
)

const (
	// VPKDir is the path to the VPK file
	VPKDir   string = "data/game/csgo/pak01_dir.vpk"
	ImageDir string = "panorama/images/econ"
	BaseDir  string = "game/csgo"
)

func main() {
	// ---------------------------------------
	// 0) Check dependencies
	// ---------------------------------------
	checkDependencies()

	//---------------------------------------
	// 1) Run DepotDownloader to get the manifest
	//---------------------------------------
	result := cmdrunner.RunCommand("tools/DepotDownloader",
		"-app", "730",
		"-depot", "2347770",
		"-dir", "data",
		"-manifest-only",
	)
	if result.Error != nil {
		log.Fatalf("DepotDownloader encountered an error: %v", result.Error)
	}
	log.Println("Collecting Manifest...")
	log.Println("Finished collecting manifest.")

	//---------------------------------------
	// 2) Find the newly-downloaded manifest
	//---------------------------------------
	matches, err := filepath.Glob("data/manifest_*")
	if err != nil {
		log.Fatalf("Failed to find manifest file: %v", err)
	}
	if len(matches) == 0 {
		log.Fatalf("No manifest file found in ./data/")
	}
	manifestFile := matches[0]

	// Example file: data/manifest_2347770_5002689339188222421.txt
	parts := strings.Split(manifestFile, "_")
	if len(parts) < 3 {
		log.Fatalf("Unexpected manifest file format: %s", manifestFile)
	}
	newManifestID := strings.TrimSuffix(parts[2], ".txt")

	//---------------------------------------
	// 3) Read any previously stored manifest ID (if file is present)
	//---------------------------------------
	trackedID := ""
	if fileExists("manifest_id.txt") {
		data, err := os.ReadFile("manifest_id.txt")
		if err != nil {
			log.Fatalf("Could not read manifest_id.txt: %v", err)
		}
		trackedID = strings.TrimSpace(string(data))
	}

	// If IDs match and it's non-empty, bail out
	if trackedID == newManifestID && trackedID != "" {
		log.Println("Manifest ID matches the current ID. Exiting.")
		return
	}

	//---------------------------------------
	// 4) If we get here, either the file doesn’t exist,
	//    or it’s empty, or the ID didn’t match → download files & run extraction
	//---------------------------------------
	log.Printf("Manifest is new or changed. New ID: %s\n", newManifestID)

	err = os.WriteFile("manifest_id.txt", []byte(newManifestID), 0644)
	if err != nil {
		log.Fatalf("Could not write to manifest_id.txt: %v", err)
	}
	log.Printf("New manifest ID %s has been saved.\n", newManifestID)

	// ---------------------------------------
	// 5) Run DepotDownloader to get the VPK dir
	// ---------------------------------------
	dirFile, err := os.CreateTemp("", "dir-file_*.txt")
	if err != nil {
		log.Fatalf("Error creating temporary file: %v", err)
	}
	defer os.Remove(dirFile.Name())
	defer dirFile.Close()

	writer := bufio.NewWriter(dirFile)
	if _, err := writer.WriteString("game/csgo/pak01_dir.vpk" + "\n"); err != nil {
		log.Fatalf("Error writing to dir-file.txt: %v", err)
	}
	if err := writer.Flush(); err != nil {
		log.Fatalf("Error flushing to dir-file.txt: %v", err)
	}

	log.Println("Collecting vpk_dir file...")

	dirResult := cmdrunner.RunCommand("tools/DepotDownloader",
		"-app", "730",
		"-depot", "2347770",
		"-dir", "data",
		"-filelist", dirFile.Name(),
	)
	if dirResult.Error != nil {
		log.Fatalf("DepotDownloader encountered an error while processing filelist: %v", dirResult.Error)
	}

	// ---------------------------------------
	// 6) Generate the VPK list
	// ---------------------------------------
	fileList, err := os.CreateTemp("", "filelist_*.txt")
	if err != nil {
		log.Fatalf("Error creating temporary file: %v", err)
	}
	defer os.Remove(fileList.Name())
	defer fileList.Close()

	err = parser.GenerateVPKList(VPKDir, ImageDir, BaseDir, fileList.Name())
	if err != nil {
		log.Fatalf("Failed to generate VPK list: %v", err)
	}

	// ---------------------------------------
	// 7) Run DepotDownloader to get the VPK files
	// ---------------------------------------
	downloadResult := cmdrunner.PipeOutput("tools/DepotDownloader",
		"-app", "730",
		"-depot", "2347770",
		"-dir", "data",
		"-filelist", fileList.Name(),
	)
	if downloadResult != nil {
		log.Fatalf("DepotDownloader encountered an error: %v", downloadResult)
	}

	log.Println("Downloading files...")
	log.Println("Finished downloading files.")

	// ---------------------------------------
	// 8) Run Source2Viewer-CLI to extract the files
	// ---------------------------------------
	extractResult := cmdrunner.PipeOutput("tools/Source2Viewer-CLI",
		"-i", VPKDir,
		"-o", "static",
		"-d",
		"--vpk_filepath", ImageDir,
	)
	if extractResult != nil {
		log.Fatalf("Source2Viewer-CLI encountered an error: %v", extractResult)
	}
	log.Println("Extracting files...")

	// Rename files
	log.Println("Renaming files...")
	findAndRenameFiles()

	// Add images to CDN list
	log.Println("Adding images to CDN list...")
	addImagesToCDN()
}

// fileExists is a helper that returns true if a file exists (and is not a directory).
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil && !info.IsDir()
}

func checkDependencies() {
	if err := dependencies.EnsureTools(); err != nil {
		log.Fatalf("Failed to ensure tools: %v", err)
	}

	log.Println("All dependencies are satisfied.")
}

func findAndRenameFiles() {
	err := filepath.WalkDir("static", func(path string, file fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("Error accessing path %q: %v\n", path, err)
			return nil
		}

		if strings.HasSuffix(file.Name(), ".png") {
			// Remove any "_png" substring from the file name
			newName := strings.ReplaceAll(file.Name(), "_png", "")
			err := os.Rename(path, filepath.Join(filepath.Dir(path), newName))
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error walking the path: %v\n", err)
	}
}

func addImagesToCDN() {
	baseURL := "https://cdn.jsdelivr.net/gh/SkinDelta/go-cs2-cdn@main/"
	cdnListPath := "cdn.json"

	// Read existing file entries
	data, err := os.ReadFile(cdnListPath)
	if err != nil {
		log.Fatalf("Error reading cdn.json: %v\n", err)
	}

	entries := make(map[string]string)
	_ = json.Unmarshal(data, &entries)

	// Walk the "static" directory
	err = filepath.WalkDir("static", func(path string, file fs.DirEntry, werr error) error {
		if werr != nil {
			log.Printf("Error accessing path %q: %v\n", path, werr)
			return nil
		}
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".png") {
			entries[path] = baseURL + path
		}
		return nil
	})
	if err != nil {
		log.Printf("Error walking dir: %v\n", err)
	}

	// Write updated entries
	updated, _ := json.MarshalIndent(entries, "", "  ")
	if err := os.WriteFile(cdnListPath, updated, 0644); err != nil {
		log.Fatalf("Error writing cdn.json: %v\n", err)
	}
}
