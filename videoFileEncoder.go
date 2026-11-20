package main

import (
	"os/exec"
)

// VideoEncodingHardware test daadwerkelijk of de hardware-acceleratie werkt op deze machine.
func VideoEncodingHardware() string {
	if testFfmpegHardware("hevc_nvenc") { // Test NVIDIA CUDA / NVENC
		return "CUDA" // Nvidia GPU hardware detected
	}
	if testFfmpegHardware("hevc_qsv") { // Test Intel QSV
		return "QSV" // Intel GPU hardware detected
	}
	if testFfmpegHardware("hevc_amf") { // Test AMD AMF
		return "AMF" // AMD GPU detected
	}

	// no video encoding hardware present (or drivers not installed)
	return ""
}

// testFfmpegHardware voert een bliksemnette test uit van 1 frame om te zien of de encoder opstart
func testFfmpegHardware(encoder string) bool {
	// We genereren 1 leeg frame met lavfi en proberen dat te coderen met de opgegeven hardware-encoder
	cmd := exec.Command("ffmpeg",
		"-v", "error",
		"-f", "lavfi",
		"-i", "color=c=black:s=128x128:r=1",
		"-frames:v", "1",
		"-c:v", encoder,
		"-f", "null",
		"-",
	)

	// Als het commando succesvol afrondt (exit code 0), is de hardware beschikbaar en werkend
	err := cmd.Run()
	return err == nil
}
