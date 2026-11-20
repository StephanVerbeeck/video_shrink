package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type File struct {
	Dir, Name, Ext string
}

const pathSeparator = `\`

func Path(path ...string) string {
	return strings.ReplaceAll(strings.Join(path, pathSeparator), pathSeparator+pathSeparator, pathSeparator)
}

func ExeDir() string {
	exePath, err := os.Executable()
	if err != nil {
		exePath = os.Args[0]
	}
	return path.Dir(exePath)
}

// CopyFile kopieert de inhoud van src naar dst
func CopyFile(src, dst string) error {
	MakeDirectory(FileFromPath(dst).Dir)

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Maak het doelbestand aan
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Stream de data van de ene naar de andere file
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Zorg dat alles naar de schijf is weggeschreven
	return destFile.Sync()
}

// MoveFile verplaatst een bestand, zelfs tussen verschillende schijven/devices
func MoveFile(src, dst string) error {
	MakeDirectory(FileFromPath(dst).Dir)

	// Probeer eerst de snelle 'Rename' (werkt alleen op dezelfde schijf)
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Als Rename faalt (bijv. cross-device link), kopieer dan handmatig
	err = CopyFile(src, dst)
	if err != nil {
		return fmt.Errorf("copy failed: %v", err)
	}

	// 4. Verwijder het origineel na een succesvolle kopie
	return os.Remove(src)
}

func DeleteFile(src string) error {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return nil // deleting a file that does not exist should not return an error
	}
	if info.IsDir() {
		return nil // oeps that is not a file budy
	}
	return os.Remove(src)
}

func DeleteDir(src string) error {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return nil // deleting a directory that does not exist should not return an error
	}
	if !info.IsDir() {
		return nil // oeps that is not a directory budy
	}
	return os.RemoveAll(src)
}

// GetFile leest de volledige inhoud van een bestand in een byte slice.
func GetFile(filePath string) []byte {
	// os.ReadFile is de moderne, snelle manier om een heel bestand te happen.
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Println("GetFile:", filePath, err)
	}
	return data
}

// PutFile schrijft een byte slice naar een bestand.
// Als het bestand al bestaat, wordt het overschreven.
func PutFile(filePath string, data []byte) {
	// 0644 zijn de standaard permissies (lezen/schrijven voor eigenaar, lezen voor rest).
	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		log.Println("PutFile:", filePath, err)
	}
}

// SaveData slaat eender welke structuur, slice of map op.
// Omdat we alleen lezen, is een pointer hier niet strikt nodig, maar het mag wel.
func SaveData(filename string, data any) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("SaveData: %w", err)
	}

	tempFile := filename + ".tmp"
	err = os.WriteFile(tempFile, bytes, 0644)
	if err != nil {
		return fmt.Errorf("SaveData: %w", err)
	}

	return os.Rename(tempFile, filename)
}

// LoadData vult de meegegeven pointer 'v' met de data uit het bestand.
// 'v' MOET een pointer zijn (bijv. &mijnStruct).
func LoadData(filename string, v any) error {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return err // geeft de OS error terug (bijv. file not found)
	}

	err = json.Unmarshal(bytes, v)
	if err != nil {
		return fmt.Errorf("LoadData: %w", err)
	}

	return nil
}

func MakeDirectory(path string) {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		log.Printf("Kon map niet aanmaken: %v\n", err)
	}
}

func FileFromPath(path string) (file File) {
	pos := strings.LastIndex(path, "\\")
	if pos != NA {
		file.Dir = path[:pos]
		end := strings.LastIndex(path[pos:], ".")
		if end != NA {
			file.Name = path[pos+1 : pos+end]
			file.Ext = strings.ToLower(path[pos+end+1:])
		} else {
			file.Name = path[pos+1:]
		}
	} else {
		end := strings.LastIndex(path, ".")
		if end != NA {
			file.Name = path[:end]
			file.Ext = strings.ToLower(path[end+1:])
		} else {
			file.Name = path
		}
	}
	return
}

func (file *File) Path() (path string) {
	if file.Ext != "" {
		path = file.Name + "." + file.Ext
	} else {
		path = file.Name
	}
	if file.Dir != "" {
		path = Path(file.Dir, path)
	}
	return
}

func (file *File) Exists() bool {
	return FileExists(file.Path())
}

// FileExists controleert of een bestand bestaat op het opgegeven pad.
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// FileExists controleert of een directory bestaat op het opgegeven pad.
func DirExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func FileSize(filename string) int64 {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) || info.IsDir() {
		return 0
	}
	return info.Size()
}

// PurgeFileName turns any string into a valid file name
func PurgeFileName(title string) string {
	// De lijst met verboden tekens in Windows
	forbidden := `\/:*?"<>|`

	// Gebruik Map om door de runes te lopen
	clean := strings.Map(func(r rune) rune {
		// 1. Controleer of de rune in de verboden lijst staat
		if strings.ContainsRune(forbidden, r) {
			return ' ' // Vervang door underscore
		}

		// 2. Verwijder controle-tekens (zoals tabs of nieuwe regels)
		if unicode.IsControl(r) {
			return -1 // -1 betekent: verwijder de rune
		}

		return r
	}, title)

	// Optioneel: Trim spaties aan het begin en eind (Windows houdt niet van spaties aan het eind)
	return strings.ReplaceAll(strings.TrimSpace(clean), "  ", " ")
}

func EjectDrive(path string) error {
	if path == "" {
		return nil
	}

	// We maken van "F" of "F:/" altijd "F:"
	driveLetter := strings.Split(path, ":")[0]
	drive := strings.ToUpper(driveLetter) + ":"

	// PowerShell commando om het volume uit te werpen
	cmdScript := fmt.Sprintf("(New-Object -ComObject Shell.Application).Namespace(17).ParseName('%s').InvokeVerb('Eject')", drive)

	cmd := exec.Command("powershell", "-Command", cmdScript)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Error ejecting drive %s %v", drive, err)
	}

	return nil
}

// OpenInNotepad opent een bestand en pauzeert de rest van de functie tot Notepad wordt afgesloten.
func OpenInNotepad(filename string) error {
	if !FileExists(filename) {
		nothing := []byte{}
		PutFile(filename, nothing)
	}

	// We gebruiken 'cmd /c' om Notepad te starten.
	// De 'wait' vlag zorgt ervoor dat CMD pas stopt als Notepad echt dicht is.
	cmd := exec.Command("cmd", "/c", "start", "/wait", "notepad.exe", filename)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Notepad: %v", err)
	}
	return nil
}

// FindOldFiles geeft een lijst (slice) van bestandspaden terug voor alle bestanden
// (recursief in dir) met een modificatiedatum ouder dan cutoffDate.
func FindOldFiles(dir string, when time.Time) ([]string, error) {
	var oldFiles []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Sla mappen over; we willen alleen bestanden in de lijst
		if d.IsDir() {
			return nil
		}

		// Haal de bestandsinformatie op om de modificatiedatum te controleren
		info, err := d.Info()
		if err != nil {
			return err
		}

		// Controleer of de modificatiedatum vóór (ouder dan) de gegeven datum ligt
		if info.ModTime().Before(when) {
			oldFiles = append(oldFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("fout bij het doorzoeken van map %s: %w", dir, err)
	}

	return oldFiles, nil
}
