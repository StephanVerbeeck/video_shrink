package main

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

type Ints []int
type Floats []float64
type Resource sync.Mutex

const (
	dq = `"` // double quote
	sq = `'` // single quote
)

var (
	// reXmlTagSequence vind XML tags met witruimte/NBSP ertussen op, ook over meerdere regels
	// (?s) : witruimte (\s) include newline.
	// (?i) : Case-insensitive voor <P> vs <p>.
	// <[a-zA-Z!?/]+[^>]*> : een XML tag.
	// (?:[\s\x{00A0}]|&nbsp;)* : Alle vormen van witruimte, incl. enters en NBSP.
	reXmlTagSequence = regexp.MustCompile(`(?si)(?:[\s\x{00A0}]|&nbsp;)*(<[a-zA-Z!?/]+[^>]*>(?:[\s\x{00A0}]|&nbsp;)*)+`)
)

// If (de 'ternary' helper)
func If[T any](conditie bool, waar, onwaar T) T {
	if conditie {
		return waar
	}
	return onwaar
}

// Max geeft de hoogste van twee waarden terug
func Max[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// Min geeft de laagste van twee waarden terug
func Min[T cmp.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// IsBetween zal checken of een waarde binnen het bereik valt (inclusief grenzen)
func IsBetween[T cmp.Ordered](minVal, maxVal, value T) bool {
	return value >= minVal && value <= maxVal
}

// Between geeft value terug, maar nooit kleiner dan min en nooit groter dan max.
func Between[T cmp.Ordered](min, max, value T) T {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// Filter returns a copy of the list which only contains the elements for which the filter function returned true
func Filter[T any](s []T, keep func(T) bool) []T {
	var result []T
	for _, v := range s {
		if keep(v) {
			result = append(result, v)
		}
	}
	return result
}

// DeleteIndex verwijdert de gegeven index.
func DeleteIndex[T any](ptr *[]T, index int) {
	if ptr == nil {
		return
	}

	s := *ptr
	if index < 0 || index >= len(s) {
		return
	}

	// We voegen alles vóór de index samen met alles na de index
	*ptr = append(s[:index], s[index+1:]...)
}

func CharIsAlphaNumeric(char rune) bool {
	return (char >= '0' && char <= '9') || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_'
}

func CharIsAlpha(char rune) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_'
}

func CharIsHex(char rune) bool {
	return (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F') || char == '_'
}

func CharIsNumeric(char rune) bool {
	return (char >= '0' && char <= '9')
}

func StringIsAlphaNumeric(chars string) bool {
	for _, char := range chars {
		if !CharIsAlphaNumeric(char) {
			return false
		}
	}
	return len(chars) > 0
}

func StringIsAlpha(chars string) bool {
	for _, char := range chars {
		if !CharIsAlpha(char) {
			return false
		}
	}
	return len(chars) > 0
}

func StringIsHex(chars string) bool {
	for _, char := range chars {
		if !CharIsHex(char) {
			return false
		}
	}
	return len(chars) > 0
}

func StringIsNumeric(chars string) bool {
	for _, char := range chars {
		if !CharIsNumeric(char) {
			return false
		}
	}
	return len(chars) > 0
}

func StringStart(text string, length int) string {
	textLen := len(text)
	if textLen >= length {
		return text[:length]
	}
	return ""
}

func StringEnd(text string, length int) string {
	textLen := len(text)
	if textLen >= length {
		return text[textLen-length:]
	}
	return ""
}

// PurgeString verwijdert de gegeven woorden en standaardizeerd de spaties
func PurgeString(input string, wordsToPurge ...string) (output string) {
	outputs := Strings{}
outerLoop:
	for _, word := range strings.Fields(input) {
		for _, wordToPurge := range wordsToPurge {
			if strings.EqualFold(word, wordToPurge) {
				continue outerLoop
			}
		}
		outputs.Add(word)
	}
	output = strings.Join(outputs, " ")
	return
}

// BoolAsInt convert boolean to integer
func BoolAsInt(isTrue bool) int {
	if isTrue {
		return 1
	} else {
		return 0
	}
}

// StringAsInt convert text to signed integer
func StringAsInt(text string) (value int) {
	negative := false
	for _, c := range text {
		if c >= '0' && c <= '9' {
			value *= 10
			value += int(c) - '0'
		} else if c == '-' && value == 0 {
			negative = true
		} else if int(c) > ' ' {
			break
		}
	}
	if negative {
		value = -value
	}
	return
}

// HexAsInt convert hexadecimal text to signed integer
func HexAsInt(text string) (value int) {
	negative := false
	for _, c := range text {
		if c >= '0' && c <= '9' {
			value *= 16
			value += int(c) - '0'
		} else if c >= 'a' && c <= 'f' {
			value *= 16
			value += int(c) - ('a' - 10)
		} else if c >= 'A' && c <= 'F' {
			value *= 16
			value += int(c) - ('A' - 10)
		} else if c == '-' && value == 0 {
			negative = true
		} else if int(c) > ' ' {
			break
		}
	}
	if negative {
		value = -value
	}
	return
}

// IntAsHex converts unsigned integer to hexadecimal text string
func IntAsHex(value uint64, length int) (text string) {
	digits := []byte("0000000000000000")
	for l := 15; value != 0 && l >= 0; l-- {
		digit := byte(value & 0xF) // take least significant digit (left most 4 bits)
		value >>= 4                // shift right, 4 bits (equals 1 hex digit)
		if digit < 10 {
			digits[l] = '0' + digit
		} else {
			digits[l] = ('a' - 10) + digit
		}
	}
	return string(digits[16-length:])
}

// IntAsString converts integer to text string
func IntAsString(value int) (text string) {
	text = strconv.Itoa(value)
	return
}

// IntLimit integer values
func IntLimit(value *int, min, max int) {
	if *value < min {
		*value = min
	} else if *value > max {
		*value = max
	}
}

// FloatLimit integer values
func FloatLimit(value *float64, min, max float64) {
	if *value < min {
		*value = min
	} else if *value > max {
		*value = max
	}
}

// IntInLimit check integer value range
func IntInLimit(value, min, max int) bool {
	return value >= min && value <= max
}

// IntAsString converts integer to text string
func FloatAsString(value float64, decimals int) (text string) {
	if decimals == NA && (math.Abs(value) < 1e-8 || math.Abs(value) > 1e+8) {
		text = fmt.Sprintf("%f", value) // use scientific notation
	} else {
		text = strconv.FormatFloat(value, 'f', decimals, 64)
	}
	return
}

func StringAsFloat(text string) (value float64) {
	value, _ = strconv.ParseFloat(text, 64)
	return
}

// FloatInLimit check floating point value range
func FloatInLimit(value, min, max float64) bool {
	return value >= min && value <= max
}

// SplitLines convert text to list of lines
func SplitLines(text string) (lines []string) {
	lines = strings.Split(text, "\n")
	for l := range lines {
		lines[l] = strings.Trim(lines[l], "\r")
	}
	return
}

// JoinLines convert list of lines to text
func JoinLines(lines ...string) (text string) {
	text = strings.Join(lines, crlf)
	return
}

// SplitWords convert sentence into word treating any non alfanumerical as a separator
func SplitWords(text string) (words []string) {
	from := -1
	for pos, char := range text {
		if CharIsAlphaNumeric(char) {
			if from == -1 {
				from = pos
			}
		} else if from >= 0 {
			words = append(words, text[from:pos])
			from = -1
		}
	}
	if from >= 0 { // no seperator behind last word?
		words = append(words, text[from:])
	}
	return
}

// JoinWords returns a string with all the words separated by a single character
func JoinWords(words ...string) string {
	return strings.ReplaceAll(strings.TrimSpace(strings.Join(words, " ")), "  ", " ")
}

// SplitCSV converts string with comma-separated-values in list of string values
func SplitCSV(csv string) (values []string) {
	values = strings.Split(csv, ",")
	for i := range values {
		value := strings.TrimSpace(values[i])
		l := len(value)
		if l >= 2 && value[0] == '"' && value[l-1] == '"' { // if string start and ends with a quote
			value = value[1 : l-2] // keep only the part between the quotes
		}
		values[i] = value
	}
	return
}

// SplitTokens convert formula expression into parts, which are numbers, strings and operators
func SplitTokens(text string) (tokens []string) {
	from, pos := NA, NA
	isNumber := func(value byte) bool {
		return (value >= '0' && value <= '9') || value == '.'
	}
	isLetter := func(value byte) bool {
		return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') || value == '_'
	}
	isOperator := func(value byte) bool {
		return value >= ' ' && !isNumber(value) && !isLetter(value)
	}
	store := func() {
		if from != NA && from < pos {
			tokens = append(tokens, text[from:pos])
			from = NA
		}
	}
	l := len(text)
	for pos = 0; pos < l; pos++ {
		char := text[pos]

		if char <= ' ' {
			store()
		} else if isOperator(char) {
			tokens = append(tokens, text[pos:pos+1])
		} else if char == '"' {
			from = pos + 1
			for pos++; pos < l && text[pos] != '"'; pos++ {
			}
			store()
		} else {
			from = pos
			for pos++; pos < l && text[pos] > ' ' && !isOperator(text[pos]); pos++ {
			}
			store()
			pos--
		}
	}
	return
}

// CommonWords returns the number of words that occur in both lists
func CommonWords(words1, words2 []string, minWordLength int) (commonWords int) {
	for _, word1 := range words1 {
		if len(word1) >= minWordLength { // can be used to ignore drive letters or file-extensions when comparing file paths
			for _, word2 := range words2 {
				if word1 == word2 {
					commonWords++
				}
			}
		}
	}
	return
}

// VarID converts any string to a valid variable name that can be used in scripting langues
func VarID(value string) (id string) {
	gap := false
	bytes := []byte{}
	for _, char := range strings.TrimSpace(value) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			if gap {
				bytes = append(bytes, '_', byte(char))
				gap = false
			} else {
				bytes = append(bytes, byte(char))
			}
			if len(bytes) > 120 {
				break
			}
		} else {
			gap = true // any successive combination of invalid characters is converted to a single space
			if len(bytes) > 100 {
				break
			}
		}
	}
	if len(bytes) == 0 {
		return "_"
	} else if bytes[0] >= '0' && bytes[0] <= '9' {
		return "_" + string(bytes)
	}
	return string(bytes)
}

// IsVarID returns true if the string is a valid variable name
func IsVarID(value string) (valid bool) {
	return value == VarID(value)
}

// IsName returns true if the string only contains characters and starts with an uppercase followed by lowercase
func IsName(name string) bool {
	for i, char := range name {
		if !unicode.IsLetter(char) {
			return false
		}
		if unicode.IsUpper(char) != (i == 0) {
			return false
		}
	}
	return true
}

// IsWord returns true if the string only contains characters
func IsWord(name string) bool {
	for _, char := range name {
		if !unicode.IsLetter(char) {
			return false
		}
	}
	return true
}

// IsCamelCase tests if a given string contains multiple concatinated words where each new word starts with a capital
func IsCamelCase(text string) (camelCase bool, names Strings) {
	from := NA
	text = strings.TrimSpace(text)
	l := len(text)
	for pos, char := range text {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) { // may not contain underscore (only letters and digits)
			camelCase = false
			return
		}
		if unicode.IsUpper(char) {
			if from != NA {
				names.Add(text[from:pos])
			}
			from = pos // start of name found
		} else if from == NA {
			camelCase = false
			return
		}
	}
	if from < l-1 {
		names.Add(text[from:])
	}
	camelCase = len(names) > 1
	return
}

// StringIsEmpty returns true if string is zero length or contains only blanks
func StringIsEmpty(text string) bool {
	return text == "" || strings.TrimSpace(text) == ""
}

// StringPurgeHTML removes all XML tags from any text
func StringPurgeHTML(input string) (output string) {
	output = reXmlTagSequence.ReplaceAllString(input, " ")
	return
}

// RandomInt geeft een willekeurig getal tussen min en max (beide inclusief)
func RandomInt(min, max int) int {
	// rand.IntN(n) geeft een getal van 0 tot n (exclusief n).
	// Door (max - min + 1) te gebruiken en er daarna 'min' bij op te tellen,
	// verschuiven we het bereik naar de juiste plek.
	return rand.IntN(max-min+1) + min
}

func SplitSentences(text string) (sentences Strings) {
	// De regex met de nieuwe regels:
	// 1. Minstens 2 kleine letters voor de punt
	// 2. Optioneel een aanhalingsteken na de punt
	// 3. Gevolgd door witruimte
	re := regexp.MustCompile(`([a-z]{2,}[.!?]['"”’»]?)\s+`)

	// We gebruiken FindAllStringIndex om de tekst te splitsen zonder de
	// delimiters (de punten en spaties) kwijt te raken.
	indices := re.FindAllStringIndex(text, -1)

	lastPos := 0
	for _, loc := range indices {
		// We pakken de tekst vanaf de vorige positie tot het einde van de match (loc[1])
		sentence := strings.TrimSpace(text[lastPos:loc[1]])
		if sentence != "" {
			sentences.Add(sentence)
		}
		lastPos = loc[1]
	}

	// Voeg het resterende deel van de tekst toe (de laatste zin)
	if lastPos < len(text) {
		remainder := strings.TrimSpace(text[lastPos:])
		if remainder != "" {
			sentences = append(sentences, remainder)
		}
	}

	return sentences
}

func Without(text string, parts ...string) string {
	for _, part := range parts {
		text = strings.ReplaceAll(text, part, "")
	}
	return text
}

func Percent(at, total float64) int {
	return int((at / total) * 100.)
}

// DrainChannel verwijdert alle opgestapelde berichten uit een channel.
func DrainChannel[T any](channel <-chan T) (skipped int) {
	for {
		select {
		case <-channel: // Bericht ontvangen en direct weggegooid
			skipped++
		default: // Het channel is leeg, we kunnen stoppen
			return
		}
	}
}

// TrimLines removes blanks from begin and end of each line and removes blank lines from begin and end of the text
func TrimLines(text string) string {
	lines := SplitLines(text)
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	from := 0
	to := len(lines) - 1
	for from < to && lines[to] == "" {
		to--
	}
	for from < to && lines[from] == "" {
		from++
	}
	return JoinLines(lines[from : to+1]...)
}

func IsQuoted(value string) bool {
	l := len(value)
	return l >= 2 && value[0] == value[l-1] && (value[0] == dq[0] || value[0] == sq[0])
}

func Quoted(value string) string {
	if IsQuoted(value) {
		return value
	}
	return dq + value + dq
}

func Unquoted(value string) string {
	l := len(value)
	if l >= 2 && value[0] == value[l-1] && (value[0] == dq[0] || value[0] == sq[0]) {
		return value[1 : l-1]
	}
	return value
}

// GCD berekent de grootste gemene deler van a en b
func GCD(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// CompareVersion compares two strings naturally.
// Numeric sequences are compared by integer value; all other characters
// are compared directly by their rune (UTF-8 byte) value.
// Returns -1 if a < b, 1 if a > b, and 0 if a == b.
func CompareVersion(a, b string) int {
	ia, ib := 0, 0
	la, lb := len(a), len(b)
	eqZeroBias := 0 // Tie-breaker if numeric values are equal but leading zeroes differ

	for ia < la && ib < lb {
		ca, cb := a[ia], b[ib]
		isNumA := ca >= '0' && ca <= '9'
		isNumB := cb >= '0' && cb <= '9'

		// If BOTH positions start a sequence of digits, compare them numerically
		if isNumA && isNumB {
			startA, startB := ia, ib

			// Scan the full digit run for both strings
			for ia < la && a[ia] >= '0' && a[ia] <= '9' {
				ia++
			}
			for ib < lb && b[ib] >= '0' && b[ib] <= '9' {
				ib++
			}

			rawLenA, rawLenB := ia-startA, ib-startB

			// Strip leading zeroes to compare true numeric value
			for startA < ia && a[startA] == '0' {
				startA++
			}
			for startB < ib && b[startB] == '0' {
				startB++
			}

			numLenA, numLenB := ia-startA, ib-startB

			// 1. More significant digits = larger number (e.g., 100 > 99)
			if numLenA < numLenB {
				return -1
			}
			if numLenA > numLenB {
				return 1
			}

			// 2. Same number of digits = compare digit by digit
			for k := 0; k < numLenA; k++ {
				if a[startA+k] < b[startB+k] {
					return -1
				}
				if a[startA+k] > b[startB+k] {
					return 1
				}
			}

			// 3. Numeric values are equal; store tie-breaker for leading zeroes if not already set
			if eqZeroBias == 0 && rawLenA != rawLenB {
				if rawLenA < rawLenB {
					eqZeroBias = -1 // "1" comes before "01"
				} else {
					eqZeroBias = 1
				}
			}
		} else {
			// Normal character/rune comparison
			if ca < cb {
				return -1
			}
			if ca > cb {
				return 1
			}
			ia++
			ib++
		}
	}

	// Shorter string comes first if all compared parts are identical
	if ia < la {
		return 1
	}
	if ib < lb {
		return -1
	}

	return eqZeroBias
}
