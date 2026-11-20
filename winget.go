package main

import (
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// IsExecutableOnPath controleert of een executable (bijv. "ffmpeg" of "ffmpeg.exe")
// beschikbaar is in het PATH van het systeem.
func IsExecutableOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Winget will verify that the given package is installed (by installing it when it was not)
func Winget(name, exeName, packageName string) {
	if IsExecutableOnPath(exeName) { // e.g. exeName="ffmpeg.exe"
		return
	}
	if _, found := FindExecutable(exeName); found { // find it even when it is not on the path (might not be a console command, e.g. "chrome.exe")
		return
	}

	log.Println("⚠️", exeName, "not found, starting installation of", name)

	if !IsExecutableOnPath("winget") {
		log.Println("⚠️ winget.exe not found, unable to install", name)
		return
	}

	cmd := exec.Command("winget", "install", packageName, "--accept-source-agreements", "--accept-package-agreements") // e.g. packageName="Gyan.FFmpeg"

	// send output to the console so that user can see the installing taking place
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		log.Printf("error during installation via winget: %v", err)
	}

	log.Println("🎉", name, "is succesvol installed!")
	log.Println("⚠️  WARNING: You still must restart the program (restart shell or restart computer first when needed for this package).")
}

// FindExecutable zoekt generiek naar een executable (bijv. "chrome.exe" of "git") op Windows.
func FindExecutable(exeName string) (string, bool) {
	// 1. Controleer de systeem-PATH
	if path, err := exec.LookPath(exeName); err == nil {
		return path, true
	}

	// 2. Controleer het Windows-register (App Paths)
	if path, found := searchRegistryAppPaths(exeName); found {
		return path, true
	}

	// 3. Doorzoek de standaard installatiemappen (met een maximale diepte voor snelheid)
	baseDirs := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		filepath.Join(os.Getenv("LocalAppData"), "Programs"), // Vaak gebruikt door o.a. VS Code, Python, Node
		os.Getenv("LocalAppData"),
	}

	for _, baseDir := range baseDirs {
		if baseDir == "" {
			continue
		}
		// Zoek tot maximaal 3 submappen diep om vertraging door diepe mappenstructuren te voorkomen
		if path, found := searchExeDirectory(baseDir, exeName, 3); found {
			return path, true
		}
	}

	return "", false
}

// searchRegistryAppPaths kijkt in HKLM en HKCU onder App Paths naar de geregistreerde executable.
func searchRegistryAppPaths(exeName string) (string, bool) {
	regPaths := []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\` + exeName,
		`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths\` + exeName,
	}
	roots := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}

	for _, root := range roots {
		for _, regPath := range regPaths {
			k, err := registry.OpenKey(root, regPath, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			val, _, err := k.GetStringValue("")
			k.Close()

			if err == nil && val != "" {
				// Verwijder eventuele aanhalingstekens rondom het pad in het register
				cleanPath := strings.Trim(val, `"`)
				if _, err := os.Stat(cleanPath); err == nil {
					return cleanPath, true
				}
			}
		}
	}
	return "", false
}

// searchExeDirectory doorzoekt een map tot een opgegeven maximale diepte naar een programma.
func searchExeDirectory(rootDir, targetExe string, maxDepth int) (string, bool) {
	targetLower := strings.ToLower(targetExe)
	var foundPath string

	rootDepth := strings.Count(filepath.Clean(rootDir), string(os.PathSeparator))

	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Negeer mappen waar we geen toegangsrechten voor hebben (zoals WindowsApps)
			return fs.SkipDir
		}

		currentDepth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - rootDepth
		if d.IsDir() && currentDepth > maxDepth {
			return fs.SkipDir
		}

		if !d.IsDir() && strings.ToLower(d.Name()) == targetLower {
			foundPath = path
			return filepath.SkipAll // Stop direct zodra het bestand is gevonden
		}
		return nil
	})

	return foundPath, foundPath != ""
}
