package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Gecompileerde regex om te verifiëren of de logregel begint met een datum in de vorm van YYYY/MM/DD
var logDateRegex = regexp.MustCompile(`^([0-9]{4})/([0-9]{2})/([0-9]{2})`)

// logWriter vervangt de slashes in een gevalideerde Go logdatum door streepjes
type logWriter struct {
	writer io.Writer
}

func (w logWriter) Write(p []byte) (n int, err error) {
	s := string(p)

	// Controleer met de regex of het formaat klopt en vervang de slashes door hyphens
	if logDateRegex.MatchString(s) {
		s = logDateRegex.ReplaceAllString(s, `${1}-${2}-${3}`)
	}

	return w.writer.Write([]byte(s))
}

func setupLogging() (*os.File, error) {
	// 1. Verkrijg het executable pad en bepaal de 'log' submap
	logDir := filepath.Join(ExeDir(), "log")

	// Zorg ervoor dat de 'log' map bestaat
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("kon log-map niet aanmaken: %w", err)
	}

	// wis oude log files (retention = 100 days)
	oud := time.Now().AddDate(0, 0, -100)
	oudeLogFiles, _ := FindOldFiles(logDir, oud)
	for _, logFile := range oudeLogFiles {
		if isLogFile(logFile) { // avoid unfortunate accidents
			DeleteFile(logFile)
		}
	}

	// Zoek .log bestanden en zip ze enkel als ze OUDER zijn dan 24 uur
	grens24Uur := time.Now().Add(-24 * time.Hour)
	teZippenFiles, _ := FindOldFiles(logDir, grens24Uur)
	for _, logFile := range teZippenFiles {
		// Enkel ongezipte .log bestanden verwerken (geen .log.zip)
		if strings.HasSuffix(strings.ToLower(logFile), ".log") {
			zipFile := logFile + ".zip"
			if err := zipAndRemoveLog(logFile, zipFile); err != nil {
				log.Printf("Waarschuwing: kon oud logbestand niet zippen (%s): %v\n", filepath.Base(logFile), err)
			}
		}
	}

	// Genereer de bestandsnaam voor de nieuwe log (in de 'log' map)
	timestamp := time.Now().Format("20060102_150405")
	logFileName := fmt.Sprintf("video_shrink_%s.log", timestamp)
	logPath := filepath.Join(logDir, logFileName)

	// Open het nieuwe logbestand
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("kon logbestand niet aanmaken: %w", err)
	}

	// Stel multi-writer in en wrap deze in logWriter voor het gewenste datumformaat
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(logWriter{writer: multiWriter})

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

// scanLogFiles opent alle .log en .log.zip bestanden in de logmap en voert
// de meegeleverde handler-functie uit voor elke individuele regel.
func scanLogFiles(logDir string, lineHandler func(line string) error) error {
	files, err := os.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("kon log-map niet uitlezen: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		lowerName := strings.ToLower(fileName)
		filePath := filepath.Join(logDir, fileName)

		if strings.HasSuffix(lowerName, ".log") {
			if err := scanTextFile(filePath, lineHandler); err != nil {
				return fmt.Errorf("fout bij het lezen van %s: %w", fileName, err)
			}
		} else if strings.HasSuffix(lowerName, ".log.zip") {
			if err := scanZipLogFile(filePath, lineHandler); err != nil {
				return fmt.Errorf("fout bij het lezen van gezipt logbestand %s: %w", fileName, err)
			}
		}
	}

	return nil
}

// scanLineByLine leest elke io.Reader (zowel een gewoon bestand als een zip-stream) regel voor regel
func scanLineByLine(r io.Reader, lineHandler func(line string) error) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if err := lineHandler(scanner.Text()); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// scanTextFile opent een gewoon tekstbestand en geeft de stream door aan scanReader
func scanTextFile(filePath string, lineHandler func(line string) error) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return scanLineByLine(f, lineHandler)
}

// scanZipLogFile leest een .log.zip bestand rechtstreeks uit in het geheugen zonder tijdelijk bestand
func scanZipLogFile(zipPath string, lineHandler func(line string) error) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Sla eventuele mappen in het zip-archief over
		if f.FileInfo().IsDir() {
			continue
		}

		// Gebruik een anonieme functie zodat rc.Close() direct na elke file in de zip wordt aangeroepen
		err := func() error {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			return scanLineByLine(rc, lineHandler)
		}()

		if err != nil {
			return err
		}
	}

	return nil
}

func scanExeLogFiles(scanLine func(line string) error) {
	logDir := Path(ExeDir(), "log")

	err := scanLogFiles(logDir, scanLine)

	if err != nil {
		log.Printf("Fout tijdens scannen van logs: %v\n", err)
	}
}
