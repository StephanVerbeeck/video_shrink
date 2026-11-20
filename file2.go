package main

import (
	"cmp"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// FileSortBy bepaalt waarop gesorteerd wordt.
type FileSortBy int

const (
	SortByName FileSortBy = iota
	SortBySize
	SortByDate
)

// SortOrder bepaalt de richting van de sortering.
type SortOrder int

const (
	Ascending SortOrder = iota
	Descending
)

type fileEntry struct {
	path    string
	name    string
	size    int64
	modTime time.Time
}

func FilesSince(pattern string, since time.Time, sortBy FileSortBy, order SortOrder) (matches []string, err error) {
	dir := filepath.Dir(pattern)
	filePattern := filepath.Base(pattern)
	lowerFilePattern := strings.ToLower(filePattern)

	var entries []fileEntry

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if !d.IsDir() {
			matched, matchErr := filepath.Match(lowerFilePattern, strings.ToLower(d.Name()))
			if matchErr != nil {
				return matchErr
			}

			if matched {
				var info os.FileInfo
				var infoErr error

				// Haal FileInfo op als we moeten filteren op datum OF sorteren op grootte/datum
				if !since.IsZero() || sortBy == SortBySize || sortBy == SortByDate {
					info, infoErr = d.Info()
					if infoErr != nil {
						return nil // Sla bestand over als info niet gelezen kan worden
					}
				}

				if !since.IsZero() && info.ModTime().Before(since) {
					return nil // Sla bestand over omdat het te oud is
				}

				entry := fileEntry{
					path: path,
					name: strings.ToLower(d.Name()), // Case-insensitive sortering op naam
				}
				if info != nil {
					entry.size = info.Size()
					entry.modTime = info.ModTime()
				}

				entries = append(entries, entry)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sorteer de verzamelde bestanden
	slices.SortFunc(entries, func(a, b fileEntry) int {
		var res int
		switch sortBy {
		case SortBySize:
			res = cmp.Compare(a.size, b.size)
		case SortByDate:
			res = a.modTime.Compare(b.modTime)
		case SortByName:
			fallthrough
		default:
			res = cmp.Compare(a.name, b.name)
		}

		// Bij gelijke waarden (bijv. exact dezelfde grootte), gebruik pad als tie-breaker
		if res == 0 {
			res = cmp.Compare(a.path, b.path)
		}

		if order == Descending {
			return -res
		}
		return res
	})

	// Zet de gesorteerde entries om naar de uiteindelijke slice met paden
	matches = make([]string, len(entries))
	for i, e := range entries {
		matches[i] = e.path
	}

	return matches, nil
}

// FileDirMake creates the directory path for the given file (including all parent levels)
// if it didn't exist yet. Returns true if it was newly created, or false if it already existed.
func FileDirMake(fileName string) bool {
	dir := filepath.Dir(fileName)

	// Check if the directory already exists
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return false
	}

	// Create the directory and any missing parent levels
	err := os.MkdirAll(dir, 0755)
	return err == nil
}

// FileDirNameExt splits a file path into its directory, filename (without extension), and extension.
func FileDirNameExt(filename string) (dir, name, ext string) {
	dir = filepath.Dir(filename)
	base := filepath.Base(filename)
	ext = filepath.Ext(base)
	name = strings.TrimSuffix(base, ext)
	return
}

// FileDelete deletes the specified file.
// Returns true if the file existed and was successfully erased.
func FileDelete(fileName string) bool {
	info, err := os.Stat(fileName)
	if err != nil || info.IsDir() {
		return false // Doesn't exist or is a directory, not a file
	}

	err = os.Remove(fileName)
	return err == nil
}

// DirDelete deletes the specified directory along with all its subdirectories and files.
// Returns true if the directory existed and was successfully erased.
func DirDelete(dirName string) bool {
	info, err := os.Stat(dirName)
	if err != nil || !info.IsDir() {
		return false // Doesn't exist or is a file, not a directory
	}

	// os.RemoveAll recursively deletes a path and any children it contains
	err = os.RemoveAll(dirName)
	return err == nil
}

// FileCopy copies a file from src to dst. Returns true if successful.
func FileCopy(src, dst string) bool {
	sourceFile, err := os.Open(src)
	if err != nil {
		return false
	}
	defer sourceFile.Close()

	info, err := sourceFile.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return false
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return false
	}

	// Ensure data is fully written to disk
	return destFile.Sync() == nil
}

// FileRename moves a file. If the destination is in a different directory,
// it performs a copy-and-delete and verifies that the destination file length
// matches the source file length before deleting the original.
func FileRename(src, dst string) bool {
	srcInfo, err := os.Stat(src)
	if err != nil || srcInfo.IsDir() {
		return false
	}

	srcDir := filepath.Dir(src)
	dstDir := filepath.Dir(dst)

	// If destination is in the same directory, use the fast and atomic OS rename
	if srcDir == dstDir {
		return os.Rename(src, dst) == nil
	}

	// Different directory: perform a copy
	if !FileCopy(src, dst) {
		return false
	}

	// Verify destination file length matches source file length
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return false
	}

	if srcInfo.Size() != dstInfo.Size() {
		return false // Length mismatch; keep source safe and return false
	}

	// Safe to delete source file after length validation
	return os.Remove(src) == nil
}

// DirCopy recursively copies a directory from src to dst. Returns true if successful.
func DirCopy(src, dst string) bool {
	srcInfo, err := os.Stat(src)
	if err != nil || !srcInfo.IsDir() {
		return false
	}

	// Create destination directory with the source's permissions
	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return false
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if !DirCopy(srcPath, dstPath) {
				return false
			}
		} else {
			if !FileCopy(srcPath, dstPath) {
				return false
			}
		}
	}

	return true
}

// DirRename moves a directory. If the destination is in a different parent directory,
// it performs a recursive copy-and-delete after verifying the destination tree exists.
func DirRename(src, dst string) bool {
	srcInfo, err := os.Stat(src)
	if err != nil || !srcInfo.IsDir() {
		return false
	}

	srcDir := filepath.Dir(src)
	dstDir := filepath.Dir(dst)

	// If destination is in the same parent directory, use the fast and atomic OS rename
	if srcDir == dstDir {
		if os.Rename(src, dst) == nil {
			return true
		}
	}

	// Different parent directory: perform a recursive copy
	if !DirCopy(src, dst) {
		return false
	}

	// Verify the destination directory exists and is valid before deleting the source
	dstInfo, err := os.Stat(dst)
	if err != nil || !dstInfo.IsDir() {
		return false
	}

	// Safe to delete the original directory tree
	return os.RemoveAll(src) == nil
}
