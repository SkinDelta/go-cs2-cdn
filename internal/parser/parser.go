// parser/parser.go
package parser

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// GenerateVPKList orchestrates the steps to generate the VPK list
func GenerateVPKList(vpkDir string, requiredPrefix string, vpkBaseDir string, outFile string) error {
	// Step 1: Generate the manifest
	manifestPath, err := generateVPKDirectory(vpkDir)
	if err != nil {
		return fmt.Errorf("error generating manifest: %w", err)
	}
	defer os.Remove(manifestPath) // Clean up the temporary manifest file

	// Step 2: Parse the manifest to get fnumbers for required images
	fnumbers, err := ParseVPKDir(manifestPath, requiredPrefix)
	if err != nil {
		return fmt.Errorf("error parsing manifest: %w", err)
	}

	if len(fnumbers) == 0 {
		log.Println("No matching image files found in the manifest.")
		return nil
	}

	// Step 3: Map fnumbers to VPK filenames
	vpks := MapFNumberToVPK(fnumbers, vpkBaseDir)

	if len(vpks) == 0 {
		log.Println("No VPKs mapped from the extracted fnumbers.")
		return nil
	}

	// Step 4: Write the VPK list to the output file
	if err := WriteVPKList(vpks, outFile); err != nil {
		return fmt.Errorf("error writing VPK list: %w", err)
	}

	return nil
}

// generateVPKDirectory runs the Source2Viewer-CLI command to generate the VPK Dir output file
func generateVPKDirectory(vpkDir string) (string, error) {

	// Create a temporary file to store the manifest
	tempFile, err := os.CreateTemp("", "vpkdir_*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary manifest file: %w", err)
	}
	defer tempFile.Close()

	// Prepare and execute the command with the correct output path
	log.Printf("Running Source2Viewer-CLI to generate vpk dir at %s", tempFile.Name())

	cmd := exec.Command("tools/Source2Viewer-CLI",
		"-i", vpkDir,
		"--vpk_dir",
	)

	cmd.Stdout = tempFile
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error running Source2Viewer-CLI: %w", err)
	}

	log.Printf("vpk dir generated successfully at %s", tempFile.Name())
	return tempFile.Name(), nil
}

// ParseVPKDir parses the VPK dir file and returns a slice of unique fnumber values for required images
func ParseVPKDir(vpkDirPath string, requiredPrefix string) ([]int, error) {
	file, err := os.Open(vpkDirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open VPK Dir file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	fnumberSet := make(map[int]struct{})
	matchedLines := 0

	for scanner.Scan() {
		line := scanner.Text()

		// Skip header and summary lines
		if strings.HasPrefix(line, "---") ||
			strings.TrimSpace(line) == "" {
			continue
		}

		// Split the line into fields
		fields := strings.Fields(line)
		if len(fields) < 5 {
			// Not enough fields to parse
			continue
		}

		// Extract FilePath (first field)
		filePath := fields[0]

		// Normalize the file path for consistent comparison
		cleanPath := filepath.ToSlash(filepath.Clean(filePath))
		normalizedPrefix := filepath.ToSlash(filepath.Clean(requiredPrefix))

		// Check if the FilePath starts with the required prefix
		if !strings.HasPrefix(cleanPath, normalizedPrefix) {
			continue
		}
		matchedLines++

		// Extract fnumber from the fields
		fnumber := 0
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "fnumber=") {
				fnumStr := strings.TrimPrefix(field, "fnumber=")
				fnum, err := strconv.Atoi(fnumStr)
				if err != nil {
					log.Printf("Warning: Invalid fnumber '%s' in line: %s", fnumStr, line)
					break
				}
				fnumber = fnum
				break
			}
		}

		if fnumber != 0 {
			fnumberSet[fnumber] = struct{}{}
		} else {
			log.Printf("No valid fnumber found in line: %s", line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading VPK Dir file: %w", err)
	}

	log.Printf("Total matched lines: %d", matchedLines)

	// Convert the set to a slice
	var fnumbers []int
	for fnum := range fnumberSet {
		fnumbers = append(fnumbers, fnum)
	}

	return fnumbers, nil
}

// MapFNumberToVPK maps fnumber to VPK filenames
func MapFNumberToVPK(fnumbers []int, baseDir string) []string {
	vpkSet := make(map[string]struct{})
	for _, fnum := range fnumbers {
		// Adjust the formatting if VPK filenames have leading zeros
		vpkName := fmt.Sprintf("pak01_%03d.vpk", fnum) // e.g., pak01_044.vpk or pak01_187.vpk
		vpkPath := filepath.Join(baseDir, vpkName)
		vpkSet[vpkPath] = struct{}{}
	}

	// Convert the set to a slice
	var vpks []string
	for vpk := range vpkSet {
		vpks = append(vpks, vpk)
	}

	return vpks
}

// WriteVPKList writes the list of VPK paths to the output file
func WriteVPKList(vpks []string, outFile string) error {
	// Open the output file for writing
	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	// Write each VPK filename to the file
	for _, vpk := range vpks {
		if _, err := f.WriteString(vpk + "\n"); err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}
	}

	return nil
}
