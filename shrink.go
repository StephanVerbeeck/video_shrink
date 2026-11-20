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

const (
	shrunkDoneFile    = "shrunk done.txt"
	shrunkFailedFile  = "shrunk failed.txt"
	shrunkSkippedFile = "shrunk skipped.txt"
)

var (
	// Regex om size en de subvelden van time (uur:minuut:seconde) direct te matchen
	progressLineRegex = regexp.MustCompile(`size\s*=\s*([^\s]+).*?time\s*=\s*([0-9]+):([0-9]+):([0-9.]+)`)
	// Matcht regels die beginnen met een component in vierkante haakjes en een foutwoord bevatten
	errorPattern    = regexp.MustCompile(`^\[.+\] .+`)
	killedByBitrate bool
	killedByErrors  bool
	errorCount      int
	command         = "shrink" // default command (if not specified on the command line before the file-wildcards)
	inputFiles      []string
	totalSize       int64
	fileSince       = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
)

func main() {
	setupLogging()
	Winget("FFMPEG", "ffmpeg.exe", "Gyan.FFmpeg")
	Winget("Chrome internet browser", "chrome.exe", "Google.Chrome.EXE")
	BuildShrunkFailed()

	if len(os.Args) < 2 {
		log.Println(`Need arguments: action(check/shrink/commit/new) and input video wildcard patterns (e.g. "d:\video\*.mp4").`)
		os.Exit(1)
	}

	patterns := []string{}
	for _, pattern := range os.Args[1:] {

		switch strings.ToLower(pattern) {
		case "check", "shrink", "commit":
			command = pattern
			continue
		case "new": // only shrink files that have been added since the previous shrink
			fileSince = FileModified(Path(ExeDir(), shrunkDoneFile))
			command = "shrink"
			continue
		}

		if strings.HasPrefix(pattern, `*:\`) {
			for drive := 'a'; drive <= 'z'; drive++ {
				if DirExists(string(drive) + `:\`) {
					patterns = append(patterns, string(drive)+pattern[1:])
				}
			}
		} else {
			patterns = append(patterns, pattern)
		}
	}
	for _, pattern := range patterns {
		files, err := FilesSince(pattern, fileSince, SortBySize, Descending)
		if err != nil {
			log.Printf("No video files here: %v\n", err)
		}
		inputFiles = append(inputFiles, files...)
	}

	// if the user did not specify the container file type then we allow them all (but filter out files that are not video files)
	inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
		return !isVideoFile(fileName)
	})

	if command == "shrink" {
		inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
			return strings.Contains(fileName, "_shrink.") || strings.Contains(fileName, "_unshrink.") || strings.Contains(fileName, `\XXX\`)
		})

		log.Printf("Found %d files (since %s).\n", len(inputFiles), TimeStamp(fileSince))

		processedFiles := loadShrunkList(Path(ExeDir(), shrunkDoneFile))
		if len(processedFiles) > 0 {
			inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
				_, exists := processedFiles[fileName]
				return exists
			})
			log.Printf(`Excluded %d already processed file(s) found in "shrunk done.txt".\n`, len(processedFiles))
		}

		failedFiles := loadShrunkList(Path(ExeDir(), shrunkFailedFile))
		if len(failedFiles) > 0 {
			inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
				_, exists := failedFiles[fileName]
				return exists
			})
			log.Printf(`Excluded %d already processed file(s) found in "shrunk failed.txt".\n`, len(failedFiles))
		}

		skippedFiles := loadShrunkList(Path(ExeDir(), shrunkSkippedFile))
		if len(skippedFiles) > 0 {
			inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
				_, exists := skippedFiles[fileName]
				return exists
			})
			log.Printf(`Excluded %d skipped processing file(s) found in "shrunk skipped.txt".\n`, len(skippedFiles))
		}
	}
	if command == "commit" {
		inputFiles = slices.DeleteFunc(inputFiles, func(fileName string) bool {
			return strings.Contains(fileName, "_shrink.") || strings.Contains(fileName, "_unshrink.") || strings.Contains(fileName, `\XXX\`)
		})
	}

	log.Printf("Found %d files to process (since %s).\n", len(inputFiles), TimeStamp(fileSince))

	hw := VideoEncodingHardware()
	log.Println("Gedetecteerde hardware-acceleratie:", hw)

	const minFreeSpaceBytes = 10 * 1024 * 1024 * 1024 // 10 GB

	for i, inputFile := range inputFiles {
		// construct names for involved video files
		ext := filepath.Ext(inputFile)
		nameWithoutExt := strings.TrimSuffix(filepath.Base(inputFile), ext)
		dir := filepath.Dir(inputFile)
		outputFile := filepath.Join(dir, nameWithoutExt+If(ext != ".mkv", ".mp4", ext))
		shrinkFile := filepath.Join(dir, nameWithoutExt+"_shrink"+If(ext != ".mkv", ".mp4", ext))
		unshrinkFile := filepath.Join(dir, nameWithoutExt+"_unshrink"+ext)

		if command == "commit" {
			if !FileExists(unshrinkFile) && !FileExists(shrinkFile) {
				continue // the original file never existed or was already deleted
			}
		}

		log.Printf("____ Processing file [%d/%d]: %s ____\n", i+1, len(inputFiles), inputFile)

		ok, err := isValidVideoFile(inputFile)
		if !ok {
			log.Println("invalid:,", inputFile, err)
			if FileExists(unshrinkFile) { // if the file was previously shrunken and the original thereof is still present
				if ok, _ := isValidVideoFile(unshrinkFile); ok { // if the original file was better than the copy
					FileDelete(inputFile)               // delete the copy
					FileRename(unshrinkFile, inputFile) // restore the original to its previous file name
					log.Println("restored:,", inputFile)
				}
			} else {
				FileRename(inputFile, inputFile+".invalid") // playback of that file is not possible so mark them for later actions
			}
			continue // skip this input file (it is not a valid video)
		}
		if command == "commit" {
			size := FileSize(unshrinkFile)
			if FileDelete(unshrinkFile) { // delete the original file
				log.Println("delete:", unshrinkFile, KMG(size)+"b")
				totalSize += size
			}
			size = FileSize(shrinkFile)
			if FileDelete(shrinkFile) { // delete the conversion-output file
				log.Println("delete:", shrinkFile, KMG(size)+"b")
				totalSize += size
			}
			continue
		}
		if command == "check" {
			continue // we only check if we could use the file but do not proceed with converting/shrinking it
		}

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
		modernCodecs := []string{"h264", "hevc", "av1", "vp9"} // efficiënte codecs die we NIET hoeven te converteren
		doConvert := globalBitrate > 1500000 || !slices.Contains(modernCodecs, videoCodec)

		ffmpegArgs := []string{
			"-hide_banner",
			"-stats_period", "30", // slechts om N seconden een status message afprinten
			"-max_error_rate", "0.01", // Breek ffmpeg af als de foutenratio hoger is dan 1%
		}

		// Als het om een AVI-bestand gaat, zet automatich repareren aan
		if strings.HasSuffix(strings.ToLower(inputFile), ".avi") {
			ffmpegArgs = append(ffmpegArgs,
				"-fflags", "+genpts",
			)
		}

		// Hardware-specifieke decodering vlaggen toevoegen
		switch hw {
		case "QSV":
			ffmpegArgs = append(ffmpegArgs,
				"-hwaccel", "qsv",
				"-hwaccel_output_format", "qsv",
			)
		case "CUDA":
			ffmpegArgs = append(ffmpegArgs,
				"-hwaccel", "cuda",
				"-hwaccel_output_format", "cuda",
			)
		case "AMF":
			ffmpegArgs = append(ffmpegArgs,
				"-hwaccel", "d3d11va",
				"-hwaccel_output_format", "d3d11va",
			)
		}

		ffmpegArgs = append(ffmpegArgs,
			"-analyzeduration", "100M",
			"-probesize", "100M",
			"-i", inputFile,
			"-map", "0:v:0",
			"-map", "0:a?",
			"-map", "0:s?",
		)

		requiredShrink := 0.80

		// what to do with the video resolution
		if vWidth > 1280 && vHeight > 800 && vWidth > vHeight { // if larger than 720p and not portrait mode
			scale := min(float64(1280)/float64(vWidth), float64(720)/float64(vHeight))
			newWidth := int(float64(vWidth) * scale)
			newHeight := int(float64(vHeight) * scale)
			newWidth = (newWidth / 2) * 2
			newHeight = (newHeight / 2) * 2
			switch hw {
			case "QSV":
				ffmpegArgs = append(ffmpegArgs, "-vf", fmt.Sprintf("scale_qsv=%d:%d", newWidth, newHeight))
			case "CUDA":
				ffmpegArgs = append(ffmpegArgs, "-vf", fmt.Sprintf("scale_cuda=%d:%d", newWidth, newHeight))
			case "AMF", "":
				ffmpegArgs = append(ffmpegArgs, "-vf", fmt.Sprintf("scale=%d:%d", newWidth, newHeight))
			}
			doConvert = true
		}

		// what to do with the video channels
		if doConvert || globalBitrate > 1500000 || videoBitrate > 1200000 || !slices.Contains(modernCodecs, videoCodec) {
			switch hw {
			case "QSV": // INTEL GPU
				ffmpegArgs = append(ffmpegArgs,
					"-c:v", "hevc_qsv",
					"-rc_mode", "la_icq",
					"-global_quality", "28",
					"-look_ahead", "1", // Enables Intel's lookahead buffer for better complexity tracking
					"-async_depth", "8",
					"-maxrate", "1200k",
					"-bufsize", "2400k",
					"-preset", "slow",
				)
			case "CUDA": // NVIDIA GPU
				ffmpegArgs = append(ffmpegArgs,
					"-c:v", "hevc_nvenc", // convert video streams to hevc
					"-cq", "30",
					"-preset", "p7",
					"-maxrate", "1200k",
					"-bufsize", "2400k",
				)
			case "AMF": // AMD GPU
				ffmpegArgs = append(ffmpegArgs,
					"-c:v", "hevc_amf",
					"-quality", "quality",
					"-rc", "cqp", // Gebruik Constant Quantization Parameter (zoals QSV ICQ)
					"-qp_p", "30", // Kwaliteitswaarde voor P-frames (vergelijkbaar met CRF 28 / ICQ 30)
					"-qp_i", "28", // I-frames iets betere kwaliteit geven voor stabiliteit
					"-maxrate", "1200k", // Hard plafond behouden voor hoge resoluties/complexe beelden
					"-bufsize", "2400k",
					"-gops_per_idr", "1",
				)
			default: // No GPU
				// Software CPU Fallback
				ffmpegArgs = append(ffmpegArgs,
					"-c:v", "libx265", // convert video streams to hevc
					"-crf", "28",
					"-preset", "medium",
					"-maxrate", "1200k",
					"-bufsize", "2400k",
				)
			}
			doConvert = true
		} else {
			ffmpegArgs = append(ffmpegArgs, "-c:v", "copy")
		}

		// what to do with the audio channels
		if (doConvert && audioBitrate == 0) || audioBitrate > 150000 {
			ffmpegArgs = append(ffmpegArgs,
				"-c:a", "aac", // convert audio streams to aac
				"-ac", "2",
				"-b:a", "96k",
			)
			if !doConvert {
				requiredShrink = 0.95 // if not converting any video stream (only audio stream) than compression may be less
			}
			doConvert = true
		} else {
			ffmpegArgs = append(ffmpegArgs, "-c:a", "copy")
		}

		ffmpegArgs = append(ffmpegArgs,
			"-c:s", "copy", // copy any subtitle streams
			"-y", shrinkFile,
		)

		log.Println(
			Duration(probe.Format.Duration), "video", fmt.Sprintf("%dx%d", vWidth, vHeight), videoCodec, KMG(int64(videoBitrate)),
			"+",
			"audio", audioCodec, KMG(int64(audioBitrate)),
			"=",
			"bitrate", KMG(int64(globalBitrate)),
			If(globalBitrate > 1500000, "> 1500k", "< 1500k"),
			If(doConvert, "(not OK)", "(OK)"))

		if !doConvert {
			continue // skip calling ffmpeg to do conversion
		}

		log.Println("Starting: ffmpeg", strings.Join(ffmpegArgs, " "))
		cmdFfmpeg := exec.Command("ffmpeg", ffmpegArgs...)

		// Redirect stdout to stderr so both outputs can be read through a single stderr pipe scanner
		cmdFfmpeg.Stdout = cmdFfmpeg.Stderr

		stderrPipe, err := cmdFfmpeg.StderrPipe()
		if err != nil {
			log.Printf("Failed to create stderr pipe: %v\n", err)
			continue
		}

		killedByBitrate = false
		killedByErrors = false
		errorCount = 0

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
			if _, statErr := os.Stat(shrinkFile); statErr == nil {
				os.Remove(shrinkFile)
			}
			continue
		}

		if err != nil {
			log.Printf("Shrink = Failed for %s: %v\n", inputFile, err)
			FileDelete(shrinkFile)
			continue
		}

		origInfo, err := os.Stat(inputFile)
		if err != nil {
			log.Printf("Failed to stat original file %s: %v\n", inputFile, err)
			FileDelete(shrinkFile)
			continue
		}

		newInfo, err := os.Stat(shrinkFile)
		if err != nil {
			log.Printf("Failed to stat output file %s: %v\n", outputFile, err)
			FileDelete(shrinkFile)
			continue
		}

		ok, err = isValidVideoFile(shrinkFile)
		if !ok {
			log.Println("invalid,", shrinkFile, err)
			continue // skip this output file (it is not a valid video)
		}

		if !VideoFilesEqualDuration(shrinkFile, inputFile, 10) {
			log.Printf("Failed: Duration of outputfile %s differs from duration inputfile %s\n",
				hhmmssFromSeconds(VideoFileDuration(shrinkFile)),
				hhmmssFromSeconds(VideoFileDuration(inputFile)))
			continue // skip this output file (it is not the same video)
		}

		origSize := origInfo.Size()
		newSize := newInfo.Size()
		shrinkSize := origSize - newSize

		if float64(newSize)/float64(origSize) <= requiredShrink {
			log.Printf("Size %d%% (%s) reduced from %s to %s bytes. Swapping files...\n",
				100-int((newSize*100)/origSize),
				KMG(shrinkSize)+"b",
				KMG(origSize),
				KMG(newSize))

			totalSize += shrinkSize

			if err := os.Rename(inputFile, unshrinkFile); err != nil {
				log.Printf("Failed to rename original file %s: %v\n", inputFile, err)
				continue
			}

			if err := os.Rename(shrinkFile, outputFile); err != nil {
				log.Printf("Failed to rename output file to %s: %v\n", inputFile, err)
				continue
			}

			log.Printf("Finished processing & swapped: %s\n", inputFile)
		} else {
			log.Printf("Optimization skipped: New size (%s bytes) is more than %d%% of original (%s bytes). Discarding temp file.\n",
				KMG(newSize),
				int(requiredShrink*100),
				KMG(origSize))
			if err := os.Remove(shrinkFile); err != nil {
				log.Printf("Failed to remove temporary output file: %v\n", err)
			}
		}

		appendShrunkList(Path(ExeDir(), shrunkDoneFile), inputFile)

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

	if command == "commit" {
		log.Println("total deleted:", KMG(totalSize)+"b")
	}
	if command == "shrink" {
		log.Println("total shrink:", KMG(totalSize)+"b")
	}

}

// OnMessage onderschept elke lijn output van FFmpeg
func OnMessage(msg string, cmd *exec.Cmd) {
	// Check for error lines matching the pattern
	if errorPattern.MatchString(msg) {
		errorCount++
		if errorCount > 50 && !killedByErrors {
			log.Println("=> WAARSCHUWING: Meer dan 50 fouten gedetecteerd in FFmpeg output! Conversie wordt afgebroken...")
			if cmd.Process != nil {
				killedByErrors = true
				cmd.Process.Kill()
			}
		}
	}

	secs, avgBitrateK, isProgressMsg := parseProgressLine(msg)
	if isProgressMsg {
		// 10 minuten = 600 seconden geëncodeerde videotijd
		if secs >= 600 && avgBitrateK > 1200 {
			log.Printf("=> WAARSCHUWING: Gemiddelde bitrate (%.1f kbits/s) na 10 minuten is hoger dan 1200! FFmpeg wordt gestopt...\n", avgBitrateK)
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

func KMG(value int64) (res string) {
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

// BuildShrunkFailed examines all log files to determine which video files are unshrinkable
func BuildShrunkFailed() {
	var (
		videoFile    string
		failedFiles  Strings
		skippedFiles Strings
	)
	ok := true
	log.Println("Scanning log files for shrink failures")

	failedFilesListFile := Path(ExeDir(), shrunkFailedFile)
	skippedFilesListFile := Path(ExeDir(), shrunkSkippedFile)
	failedFiles = SplitLines(string(GetFile(failedFilesListFile)))
	skippedFiles = SplitLines(string(GetFile(skippedFilesListFile)))

	scanExeLogFiles(func(line string) error {
		if match, fields := IsWildcard(line, "*____ Processing file [*/*]: * ____"); match {
			if !ok {
				failedFiles.AddUnique(videoFile)
			}
			videoFile = fields[3]
			ok = false
		} else if match, _ := IsWildcard(line, "* (OK)"); match { // files that didn't need compression
			skippedFiles.AddUnique(videoFile)
			ok = true
		} else if match, _ := IsWildcard(line, "* Finished processing *"); match { // files that didn't compress for whatever reason
			ok = true
		}
		return nil
	})
	failedFiles.Sort()
	skippedFiles.Sort()
	PutFile(failedFilesListFile, []byte(JoinLines(failedFiles...)))
	PutFile(skippedFilesListFile, []byte(JoinLines(skippedFiles...)))
}
