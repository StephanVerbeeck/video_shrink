package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Struct om de minimale benodigde ffprobe JSON-output te vangen
type FFprobeOutput struct {
	Streams []struct {
		CodecName string `json:"codec_name"`
		CodecType string `json:"codec_type"`
	} `json:"streams"`
}

// isValidVideoFile controleert via ffprobe of een bestand een geldige, bruikbare videostream heeft.
func isValidVideoFile(filePath string) (bool, error) {
	// Voer ffprobe uit in JSON-formaat voor de eerste videostream (v:0)
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,codec_type",
		"-of", "json",
		filePath,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Als ffprobe crasht of de headers niet kan lezen, is het bestand corrupt
		return false, fmt.Errorf("ffprobe kon bestand niet lezen: %v (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	var probe FFprobeOutput
	if err := json.Unmarshal(out.Bytes(), &probe); err != nil {
		return false, fmt.Errorf("fout bij parsen ffprobe JSON: %v", err)
	}

	// Geen enkele videostream gevonden? Dan is het geen geldige video
	if len(probe.Streams) == 0 {
		return false, nil
	}

	// Check de codec naam
	codecName := strings.ToLower(probe.Streams[0].CodecName)

	// Als de codec leeg is, of expliciet 'rawvideo' (zoals bij jouw Doctor Who beschadigingen), keur hem af
	if codecName == "" || codecName == "rawvideo" {
		return false, nil
	}

	return true, nil
}
