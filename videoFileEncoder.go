package main

import (
	"os/exec"
)

// VideoEncodingHardware test daadwerkelijk of de hardware-acceleratie werkt op deze machine.
func VideoEncodingHardware() string {
	// 1. Test Intel QSV
	if testFfmpegHardware("hevc_qsv") {
		return "QSV"
	}

	// 2. Test NVIDIA CUDA / NVENC
	if testFfmpegHardware("hevc_nvenc") {
		return "CUDA"
	}

	// 3. Test AMD AMF
	if testFfmpegHardware("hevc_amf") {
		return "AMF"
	}

	// Fallback als niets werkt
	return "CPU"
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
