package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type ProbeData struct {
	Format struct {
		BitRate  string `json:"bit_rate"`
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		BitRate   string `json:"bit_rate"`
	} `json:"streams"`
}

var (
	// Regex om size en de subvelden van time (uur:minuut:seconde) direct te matchen
	progressLineRegex = regexp.MustCompile(`size\s*=\s*([^\s]+).*?time\s*=\s*([0-9]+):([0-9]+):([0-9.]+)`)
	killedByBitrate   bool
)

func main() {
	setupLogging()

	if len(os.Args) < 2 {
		log.Println(`Need argument: input video wildcard pattern (e.g. "*.mp4").`)
		os.Exit(1)
	}

	pattern := os.Args[1]
	inputFiles, err := recursiveGlob(pattern)
	if err != nil {
		log.Printf("Invalid wildcard pattern: %v\n", err)
		os.Exit(1)
	}

	inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
		return strings.Contains(fileName, "_shrink.") || strings.Contains(fileName, "_unshrink.")
	})

	processedFiles := loadShrunkList("shrunk.txt")
	if len(processedFiles) > 0 {
		inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
			_, exists := processedFiles[fileName]
			return exists
		})
		log.Printf("Excluded %d already processed file(s) found in shrunk.txt.\n", len(processedFiles))
	}

	if len(inputFiles) == 0 {
		log.Printf("No files found matching pattern: %s\n", pattern)
		os.Exit(0)
	}

	log.Printf("Found %d file(s) to process.\n", len(inputFiles))

	const minFreeSpaceBytes = 10 * 1024 * 1024 * 1024 // 10 GB

	for i, inputFile := range inputFiles {
		log.Printf("\n--- Processing file [%d/%d]: %s ---\n", i+1, len(inputFiles), inputFile)

		ext := filepath.Ext(inputFile)
		nameWithoutExt := strings.TrimSuffix(filepath.Base(inputFile), ext)
		dir := filepath.Dir(inputFile)
		outputFile := filepath.Join(dir, nameWithoutExt+"_shrink"+ext)
		unshrinkFile := filepath.Join(dir, nameWithoutExt+"_unshrink"+ext)

		cmdProbe := exec.Command("ffprobe",
			"-v", "quiet",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			"-analyzeduration", "100M",
			"-probesize", "100M",
			inputFile,
		)
		output, err := cmdProbe.Output()
		if err != nil {
			log.Printf("Failed to run ffprobe on %s: %v. Skipping.\n", inputFile, err)
			continue
		}

		var probe ProbeData
		if err := json.Unmarshal(output, &probe); err != nil {
			log.Printf("Failed to parse ffprobe JSON for %s: %v. Skipping.\n", inputFile, err)
			continue
		}

		var vWidth, vHeight int
		var videoBitrate int
		var audioBitrate int
		var videoCodec string
		var audioCodec string
		hasVideo := false
		hasAudio := false

		for _, s := range probe.Streams {
			if s.CodecType == "video" && !hasVideo {
				vWidth = s.Width
				vHeight = s.Height
				videoBitrate = parseBitrate(s.BitRate)
				videoCodec = s.CodecName
				hasVideo = true
			} else if s.CodecType == "audio" && !hasAudio {
				audioBitrate = parseBitrate(s.BitRate)
				audioCodec = s.CodecName
				hasAudio = true
			}
		}

		globalBitrate := parseBitrate(probe.Format.BitRate)
		modernCodecs := []string{"hevc", "av1", "vp9"} // efficiënte codecs die we NIET hoeven te converteren
		doConvert := globalBitrate == 0 || globalBitrate > 1500000 || !slices.Contains(modernCodecs, videoCodec)

		ffmpegArgs := []string{
			"-hide_banner",
			"-stats_period", "30", // slechts om N seconden een status message afprinten
		}

		// Als het om een AVI-bestand gaat, zet automatich repareren aan
		if strings.HasSuffix(strings.ToLower(inputFile), ".avi") {
			ffmpegArgs = append(ffmpegArgs, "-fflags", "+genpts")
		}

		ffmpegArgs = append(ffmpegArgs,
			"-hwaccel", "qsv",
			"-hwaccel_output_format", "qsv",
			"-analyzeduration", "100M",
			"-probesize", "100M",
			"-i", inputFile,
			"-map", "0:v:0",
			"-map", "0:a?",
			"-map", "0:s?",
		)

		// what to do with the video resolution
		if vWidth > 1280 && vHeight > 800 {
			scale := min(float64(1280)/float64(vWidth), float64(720)/float64(vHeight))
			newWidth := int(float64(vWidth) * scale)
			newHeight := int(float64(vHeight) * scale)
			newWidth = (newWidth / 2) * 2
			newHeight = (newHeight / 2) * 2
			ffmpegArgs = append(ffmpegArgs, "-vf", fmt.Sprintf("scale_qsv=%d:%d", newWidth, newHeight))
			doConvert = true
		}

		// what to do with the video channels
		if videoBitrate == 0 || videoBitrate > 1200000 || !slices.Contains(modernCodecs, videoCodec) {
			ffmpegArgs = append(ffmpegArgs,
				"-c:v", "hevc_qsv",
				"-rc_mode", "icq",
				"-global_quality", "30",
				"-async_depth", "8",
				"-maxrate", "1100k",
				"-bufsize", "2200k",
				"-preset", "slow",
			)
			doConvert = true
		} else {
			ffmpegArgs = append(ffmpegArgs, "-c:v", "copy")
		}

		// what to do with the audio channels
		if (doConvert && audioBitrate == 0) || audioBitrate > 128000 {
			ffmpegArgs = append(ffmpegArgs,
				"-c:a", "aac",
				"-ac", "2",
				"-b:a", "96k",
			)
			doConvert = true
		} else {
			ffmpegArgs = append(ffmpegArgs, "-c:a", "copy")
		}

		ffmpegArgs = append(ffmpegArgs,
			"-c:s", "copy",
			"-y", outputFile,
		)

		log.Println(
			Duration(probe.Format.Duration), "video", fmt.Sprintf("%dx%d", vWidth, vHeight), videoCodec, KMG(videoBitrate),
			"+",
			"audio", audioCodec, KMG(audioBitrate),
			"=",
			"bitrate", KMG(globalBitrate),
			If(globalBitrate > 1500000, "> 1500k", "< 1500k"),
			If(doConvert, "(not OK)", "(OK)"))

		if !doConvert {
			continue // skip calling ffmpeg to do conversion
		}

		log.Println("Starting: ffmpeg", strings.Join(ffmpegArgs, " "))
		cmdFfmpeg := exec.Command("ffmpeg", ffmpegArgs...)

		stderrPipe, err := cmdFfmpeg.StderrPipe()
		if err != nil {
			log.Printf("Failed to create stderr pipe: %v\n", err)
			continue
		}

		killedByBitrate = false
		if err := cmdFfmpeg.Start(); err != nil {
			log.Printf("Failed to start ffmpeg: %v\n", err)
			continue
		}

		scanner := bufio.NewScanner(stderrPipe)
		scanner.Split(scanCR)
		for scanner.Scan() {
			OnMessage(scanner.Text(), cmdFfmpeg)
		}
		_ = scanner

		err = cmdFfmpeg.Wait()

		if killedByBitrate {
			log.Printf("Encoding aborted for %s due to high average bitrate after 10 minutes.\n", inputFile)
			if _, statErr := os.Stat(outputFile); statErr == nil {
				os.Remove(outputFile)
			}
			appendShrunkList("shrunk.txt", inputFile)
			continue
		}

		if err != nil {
			log.Printf("Shrink = Failed for %s: %v\n", inputFile, err)
			if _, statErr := os.Stat(outputFile); statErr == nil {
				os.Remove(outputFile)
			}
			continue
		}

		origInfo, err := os.Stat(inputFile)
		if err != nil {
			log.Printf("Failed to stat original file %s: %v\n", inputFile, err)
			os.Remove(outputFile)
			continue
		}

		newInfo, err := os.Stat(outputFile)
		if err != nil {
			log.Printf("Failed to stat output file %s: %v\n", outputFile, err)
			os.Remove(outputFile)
			continue
		}

		origSize := int(origInfo.Size())
		newSize := int(newInfo.Size())

		if newSize*5 <= origSize*4 {
			log.Printf("Size %d%% reduced from %s to %s bytes. Swapping files...\n", 100-int((newSize*100)/origSize), KMG(origSize), KMG(newSize))

			if err := os.Rename(inputFile, unshrinkFile); err != nil {
				log.Printf("Failed to rename original file %s: %v\n", inputFile, err)
				continue
			}

			if err := os.Rename(outputFile, inputFile); err != nil {
				log.Printf("Failed to rename output file to %s: %v\n", inputFile, err)
				continue
			}

			log.Printf("Finished processing & swapped: %s\n", inputFile)
		} else {
			log.Printf("Optimization skipped: New size (%s bytes) is not at least 20%% smaller than original (%s bytes). Discarding temp file.\n", KMG(newSize), KMG(origSize))
			if err := os.Remove(outputFile); err != nil {
				log.Printf("Failed to remove temporary output file: %v\n", err)
			}
		}

		appendShrunkList("shrunk.txt", inputFile)

		freeBytesAfter, err := getFreeDiskSpaceBytes(inputFile)
		if err == nil {
			log.Printf("Current free disk space: %.2f GB\n", float64(freeBytesAfter)/(1024*1024*1024))
			if freeBytesAfter < minFreeSpaceBytes {
				log.Println("Free disk space has dropped below 10 GB limit. Stopping batch processing.")
				break
			}
		}
	}

	log.Println("--- Batch processing completed ---")
}

// OnMessage onderschept elke lijn output van FFmpeg
func OnMessage(msg string, cmd *exec.Cmd) {
	secs, avgBitrateK, isProgressMsg := parseProgressLine(msg)
	if isProgressMsg {
		// 10 minuten = 600 seconden geëncodeerde videotijd
		if secs >= 600 && avgBitrateK > 1500 {
			log.Printf("=> WAARSCHUWING: Gemiddelde bitrate (%.1f kbits/s) na 10 minuten is hoger dan 1500! FFmpeg wordt gestopt...\n", avgBitrateK)
			if cmd.Process != nil {
				killedByBitrate = true
				cmd.Process.Kill()
			}
		}
	}
	println(msg)
}

// Parse helper met regex voor size en time-subvelden
func parseProgressLine(line string) (seconds float64, bitrate float64, ok bool) {
	var sizeInKiB float64
	var err error

	matches := progressLineRegex.FindStringSubmatch(line)
	if len(matches) < 5 {
		return
	}

	hours, _ := strconv.ParseFloat(matches[2], 64)
	minutes, _ := strconv.ParseFloat(matches[3], 64)
	seconds, _ = strconv.ParseFloat(matches[4], 64)

	minutes += hours * 60
	seconds += minutes * 60

	sizePart := strings.TrimSpace(matches[1])
	if value, ok := strings.CutSuffix(sizePart, "KiB"); ok {
		sizeInKiB, err = strconv.ParseFloat(value, 64)
	} else if value, ok := strings.CutSuffix(sizePart, "kB"); ok {
		val, parseErr := strconv.ParseFloat(value, 64)
		if parseErr == nil {
			sizeInKiB = val * (1000.0 / 1024.0)
		}
		err = parseErr
	} else {
		valStr := strings.TrimRight(sizePart, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
		sizeInKiB, err = strconv.ParseFloat(valStr, 64)
	}

	if err != nil {
		return
	}

	// Bereken gemiddelde bitrate sinds start: (KiB * 8.192) / seconden = kbits/s
	if seconds > 0 { // avoid div/zero
		bitrate = (sizeInKiB * 8.192) / seconds
	}
	ok = true
	return
}

func parseDurationToSeconds(s string) (seconds float64) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return
	}
	hours, _ := strconv.ParseFloat(parts[0], 64)
	minutes, _ := strconv.ParseFloat(parts[1], 64)
	seconds, _ = strconv.ParseFloat(parts[2], 64)

	minutes += hours * 60
	seconds += minutes * 60
	return
}

func loadShrunkList(filename string) (m map[string]bool) {
	m = make(map[string]bool)
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(scanCR)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			m[line] = true
		}
	}
	_ = scanner
	return
}

func appendShrunkList(filename, filePath string) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Warning: failed to update %s: %v\n", filename, err)
		return
	}
	defer file.Close()

	if _, err := file.WriteString(filePath + "\n"); err != nil {
		log.Printf("Warning: failed to write to %s: %v\n", filename, err)
	}
}

func KMG(value int) (res string) {
	if value > 1e10 {
		return fmt.Sprintf("%dG", value/1e9)
	}
	if value > 1e7 {
		return fmt.Sprintf("%dM", value/1e6)
	}
	if value > 1e4 {
		return fmt.Sprintf("%dK", value/1e3)
	}
	if value <= 0 {
		return "_"
	}
	return fmt.Sprintf("%d", value)
}

func Duration(secondsStr string) (res string) {
	seconds, err := strconv.ParseFloat(secondsStr, 64)
	if err != nil {
		return "__:__"
	}
	duration := time.Duration(seconds * float64(time.Second))
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	return fmt.Sprintf("%02d:%02d", hours, minutes)
}

func recursiveGlob(pattern string) (matches []string, err error) {
	dir := filepath.Dir(pattern)
	filePattern := filepath.Base(pattern)

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			matched, matchErr := filepath.Match(filePattern, d.Name())
			if matchErr != nil {
				return matchErr
			}
			if matched {
				matches = append(matches, path)
			}
		}
		return nil
	})
	return
}

func parseBitrate(s string) (val int) {
	val = 0
	if s == "" {
		return
	}
	parsed, err := strconv.Atoi(s)
	if err != nil {
		return
	}
	val = parsed
	return
}

func getFreeDiskSpaceBytes(path string) (freeBytes uint64, err error) {
	freeBytes = 0
	dir := filepath.Dir(path)
	vol := filepath.VolumeName(dir)
	if vol == "" {
		vol = "."
	} else {
		vol = vol + "\\"
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	ptr, err := syscall.UTF16PtrFromString(vol)
	if err != nil {
		return
	}

	var totalBytes, totalFreeBytes uint64
	ret, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		err = callErr
		return
	}
	return
}

func If(test bool, s1 string, s2 string) string {
	if test {
		return s1
	} else {
		return s2
	}
}

// Custom split functie voor bufio.Scanner om op zowel \r als \n te splitsen (vanwege FFmpeg output)
func scanCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := range data {
		if data[i] == '\n' {
			return i + 1, data[0:i], nil
		}
		if data[i] == '\r' {
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[0:i], nil
			}
			return i + 1, data[0:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
