package main

import (
	"cmp"
	"slices"
	"strings"
)

const (
	NA   = -1     // not available or not applicable
	crlf = "\r\n" // cariage return line feed
)

type Strings List[string]

// Empty this list of strings
func (ss *Strings) Empty() {
	*ss = nil
}

// IsEmpty returns true if the list of strings is empty
func (ss Strings) IsEmpty() bool {
	return len(ss) == 0
}

// Contains returns true if any of the given strings equals the given value
func (ss Strings) Contains(value string) bool {
	return StringsContains(ss, value)
}

// Add string to start of list of strings
func (ss *Strings) Add(value string) {
	*ss = append(*ss, value)
}

// AddUnique add extra string to start of list of unique strings
func (ss *Strings) AddUnique(value string) bool {
	if !ss.Contains(value) {
		*ss = append(*ss, value)
		return true // new item was added
	}
	return false // already contained that item
}

// PopLast string from list (undoes Add)
func (ss *Strings) PopLast() (value string) {
	l := len(*ss)
	if l > 0 {
		l--
		value = (*ss)[l]
		(*ss) = (*ss)[:l]
	}
	return
}

// PopFirst string from list (undoes Prepend)
func (ss *Strings) PopFirst() (value string) {
	l := len(*ss)
	if l > 0 {
		l--
		value = (*ss)[0]
		(*ss) = (*ss)[1:]
	}
	return
}

// Prepend add extra string to start of list of strings
func (ss *Strings) Prepend(value string) {
	*ss = append([]string{value}, *ss...)
}

// PrependUnique add extra string to end of list of unique strings
func (ss *Strings) PrependUnique(value string) {
	if !ss.Contains(value) {
		*ss = append([]string{value}, *ss...)
	}
}

// Sort list of strings alphabetical
func (ss *Strings) Sort() {
	slices.SortFunc(*ss, func(a, b string) int {
		return cmp.Compare(strings.ToLower(a), strings.ToLower(b))
	})
}

// returns a mirror of the list
func (ss Strings) Reversed() (rev Strings) {
	l := len(ss)
	rev = make(Strings, l)
	for i := range l {
		rev[l-i-1] = ss[i]
	}
	return
}

// Remove string from list of strings
func (ss *Strings) Remove(value string) bool {
	l := len(*ss)
	for i := 0; i < l; i++ {
		if (*ss)[i] == value {
			for ; i < l-1; i++ {
				(*ss)[i] = (*ss)[i+1]
			}
			(*ss) = (*ss)[:l-1]
			return true // removed from list
		}
	}
	return false
}

// Join all strings with separator
func (ss Strings) Join(separator string) string {
	return strings.Join(ss, separator)
}

// JoinFML all strings with different separator for first and last value
func (ss Strings) JoinFML(first, middle, last string) string {
	l := len(ss)
	switch l {
	case 0: // no values
		return ""
	case 1: // one value
		return ss[0]
	case 2: // two values
		return strings.Join(ss, last)
	case 3: // three values
		return strings.Join(Strings{ss[0], first, ss[1], last, ss[2]}, "")
	case 4: // four values
		return strings.Join(Strings{ss[0], first, ss[1], middle, ss[2], last, ss[3]}, "")
	default: // more than 4 values
		return strings.Join(Strings{ss[0], first, strings.Join(ss[1:l-1], middle), last, ss[l-1]}, "")
	}
}

// Eq test for equal
func (ss Strings) Eq(zz Strings) bool {
	if len(ss) != len(zz) {
		return false
	}
	for i := range ss {
		if ss[i] != zz[i] {
			return false
		}
	}
	return true
}

func (ss *Strings) DeleteIndex(index int) {
	(*List[string])(ss).DeleteIndex(index)
}

func (ss *Strings) Delete(value string) {
	(*List[string])(ss).Delete(value)
}

func (ss Strings) Index(value string) int {
	return (List[string])(ss).Index(value)
}

func (ss *Strings) Save(file string) {
	PutFile(file, []byte(JoinLines((*ss)...)))
}

func (ss *Strings) Load(file string) {
	if FileExists(file) {
		*ss = SplitLines(string(GetFile(file)))
	} else {
		ss.Empty()
	}
}

// StringsIndex returns index of first equal string or -1 if none are equal
func StringsIndex(strings []string, value string) int {
	for index, v := range strings {
		if v == value {
			return index
		}
	}
	return NA
}

// StringsContains returns true if any of the given strings equals the given value
func StringsContains(strings []string, value string) bool {
	return StringsIndex(strings, value) != NA
}
