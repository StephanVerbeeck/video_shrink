package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type FFprobeOutput struct {
	Streams []struct {
		CodecName  string `json:"codec_name"`
		CodecType  string `json:"codec_type"`
		NbFrames   string `json:"nb_frames"`
		Duration   string `json:"duration"`
		RFrameRate string `json:"r_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

var (
	// A map of common video container extensions (must include the leading dot and be lowercase)
	videoExtensions = map[string]bool{
		".mp4":  true,
		".m4v":  true,
		".mov":  true,
		".avi":  true,
		".divx": true,
		".mkv":  true,
		".webm": true,
		".flv":  true,
		".wmv":  true,
		".mpeg": true,
		".mpg":  true,
		".3gp":  true,
		".ogv":  true,
		".ts":   true,
		".mts":  true,
		".m2ts": true,
	}
)

// isVideoFile checks if the given file path has a known video extension.
func isVideoFile(file string) bool {
	// Get the extension and convert to lowercase for case-insensitive matching
	ext := strings.ToLower(filepath.Ext(file))

	// Return true if the extension exists in the map
	return videoExtensions[ext]
}

// isLogFile checks if the given file path has a logfile extension.
func isLogFile(file string) bool {
	return strings.HasSuffix(file, ".log") || strings.HasSuffix(file, ".log.zip")
}

// isValidVideoFile controleert de codec, frames en valideert of de verhouding (FPS) zinvol is.
func isValidVideoFile(filePath string) (bool, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,codec_type,nb_frames,duration,r_frame_rate",
		"-show_entries", "format=duration",
		"-of", "json",
		filePath,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("ffprobe kon bestand niet lezen: %v", err)
	}

	var probe FFprobeOutput
	if err := json.Unmarshal(out.Bytes(), &probe); err != nil {
		return false, err
	}

	// Geen videostream gevonden? Ongeldig.
	if len(probe.Streams) == 0 {
		return false, nil
	}

	stream := probe.Streams[0]
	codecName := strings.ToLower(stream.CodecName)

	// Check op lege codec of corrupte rawvideo
	if codecName == "" || codecName == "rawvideo" {
		return false, nil
	}

	// Bepaal de duur (probeer eerst stream duration, anders format duration)
	durationSec := 0.0
	if stream.Duration != "" {
		durationSec, _ = strconv.ParseFloat(stream.Duration, 64)
	}
	if durationSec <= 0 && probe.Format.Duration != "" {
		durationSec, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	}

	// Als er een geldige duur is én nb_frames is aanwezig en te parsen:
	if durationSec > 0 && stream.NbFrames != "" && stream.NbFrames != "N/A" {
		if frames, err := strconv.ParseInt(stream.NbFrames, 10, 64); err == nil && frames > 0 {
			calculatedFps := float64(frames) / durationSec

			// Een normale videostream heeft een FPS tussen pakweg 5 en 120.
			// Als het totaal aantal frames en de duur een onmogelijke FPS opleveren (bijv. < 1 of > 150), is het corrupt.
			if calculatedFps < 5.0 || calculatedFps > 120.0 {
				return false, fmt.Errorf("ongeldige FPS berekend (%.2f): frames=%d, duur=%.2fs", calculatedFps, frames, durationSec)
			}
		} else if err == nil && frames == 0 {
			// 0 frames betekent een lege huls
			return false, nil
		}
	}

	return true, nil
}

// VideoFileDuration returns the duration of the video file in seconds, or 0 if it cannot be read.
func VideoFileDuration(filename string) (duration int) {
	cmdProbe := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filename,
	)
	output, err := cmdProbe.Output()
	if err != nil {
		return 0
	}

	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &probe); err != nil {
		return 0
	}

	secFloat, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil {
		return 0
	}

	return int(secFloat)
}

// VideoFilesEqualDuration checks if two video files have durations that differ by no more than toleranceSeconds.
func VideoFilesEqualDuration(file1, file2 string, toleranceSeconds int) bool {
	d1 := VideoFileDuration(file1)
	if d1 == 0 {
		return false
	}

	d2 := VideoFileDuration(file2)
	if d2 == 0 {
		return false
	}

	diff := d1 - d2
	if diff < 0 {
		diff = -diff
	}

	return diff <= toleranceSeconds
}

// hhmmssFromSeconds converts a total number of seconds into an "HH:MM:SS" formatted string.
func hhmmssFromSeconds(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

// FileModified returns the modification time of the file, or a zero time.Time if it doesn't exist or can't be read.
func FileModified(filename string) (at time.Time) {
	info, err := os.Stat(filename)
	if err != nil {
		return time.Time{} // If the file doesn't exist or unauthorized (check using t.IsZero())
	}
	return info.ModTime()
}

// TimeStamp formats a time.Time object into "yyyy-mm-dd hh:mm:ss".
func TimeStamp(at time.Time) string {
	return at.Format("2006-01-02 15:04:05")
}
