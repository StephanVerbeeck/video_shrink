package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func setupLogging() (*os.File, error) {
	// 1. Verkrijg het executable pad en bepaal de 'log' submap
	ex, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("kon executable pad niet achterhalen: %w", err)
	}

	exDir := filepath.Dir(ex)
	logDir := filepath.Join(exDir, "log")

	// Zorg ervoor dat de 'log' map bestaat
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("kon log-map niet aanmaken: %w", err)
	}

	// Zoek .log bestanden en zip ze enkel als ze OUDER zijn dan 24 uur
	files, err := os.ReadDir(logDir)
	if err == nil {
		now := time.Now()
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".log") {
				info, err := file.Info()
				if err != nil {
					continue
				}

				// Check of het bestand ouder is dan 24 uur
				if now.Sub(info.ModTime()) > 24*time.Hour {
					logFilePath := filepath.Join(logDir, file.Name())
					zipFilePath := logFilePath + ".zip"

					if err := zipAndRemoveLog(logFilePath, zipFilePath); err != nil {
						log.Printf("Waarschuwing: kon oud logbestand niet zippen (%s): %v\n", file.Name(), err)
					}
				}
			}
		}
	}

	// Genereer de bestandsnaam voor de nieuwe log (in de 'log' map)
	exName := filepath.Base(ex)
	ext := filepath.Ext(exName)
	baseName := strings.TrimSuffix(exName, ext)
	timestamp := time.Now().Format("20060102_150405")

	logFileName := fmt.Sprintf("%s_%s.log", baseName, timestamp)
	logPath := filepath.Join(logDir, logFileName)

	// Open het nieuwe logbestand
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("kon logbestand niet aanmaken: %w", err)
	}

	// Stel multi-writer in
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)

	fmt.Println("logfile:", logPath)

	return logFile, nil
}

// Hulpfunctie om een logbestand te zippen en daarna het origineel te verwijderen
func zipAndRemoveLog(srcPath, dstZipPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstZipPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	zipWriter := zip.NewWriter(dstFile)
	defer zipWriter.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.Base(srcPath)
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	if _, err := io.Copy(writer, srcFile); err != nil {
		return err
	}

	zipWriter.Close()
	dstFile.Close()
	srcFile.Close()

	return os.Remove(srcPath)
}
