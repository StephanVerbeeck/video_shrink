package main

import "strings"

const (
	wildcardChar = "*" // Each placeholder represents a sequence of one-or-more characters that does NOT contain what (comes behind the placeholder and before the next placeholder)
	maskChar     = '_' // Each placeholder represents a single character (never more, never less)
)

/*
wildcard syntax:

"*"  = replaces 1 or more characters (returning 1 field)
"**" = replaces as many characters as possible (returning longest possible match)
"***" = replaces zero or more fields (must be followed by the separator between those fields)

*/

// IsWildcard checks if the given string with value matches the given wildcard and returns the found matching fields
func IsWildcard(value string, wildcards ...string) (match bool, fields Strings) {
	if len(wildcards) == 0 { // no requirements specified?
		match = true
		return
	}
	lv := len(value)
next:
	for _, wildcard := range wildcards {
		fields = nil
		lw := len(wildcard)
		w := strings.Split(wildcard, wildcardChar)
		f := len(w)
		switch f { // wildcard ""
		case 0:
			match = value == ""

		case 1: // wildcard "*" (placeholder does not occur in wildcard)
			match = value == wildcard

		case 2: // wildcard with one placeholder (w[0] is what is before the placeholder, w[1] is what is behind the placeholder)
			if lv < lw {
				continue next // there are not enough characters in value to match the wildcard (we need at least one character per placeholder)
			}
			if w[0] == "" {
				if w[1] == "" { // wildcard "*" (match any value)
					fields.Add(value)
					match = true
				} else { // wildcard "*_" (value must end with _)
					l1 := len(w[1])
					if value[lv-l1:] == w[1] {
						fields.Add(value[:lv-l1])
						match = true
					}
				}
			} else if w[1] == "" { // wildcard "_*" (value must start with _)
				l0 := len(w[0])
				if value[:l0] == w[0] {
					fields.Add(value[l0:])
					match = true
				}
			} else {
				l0 := len(w[0])
				l1 := len(w[1])
				if lv >= l0+l1 && value[:l0] == w[0] && value[lv-l1:] == w[1] {
					fields.Add(value[l0 : lv-l1])
					match = true
				}
			}

		default: // more than one placeholder
			done := 0       // position in value that is the end of what we have already done
			greedy := false // activated by double seperator (catching both the first and the last occurence of next sub-value)
			list := false   // return greedy region as multiple fields
			for i, v := range w {
				if v == "" {
					if i == 0 {
						// not greedy, just start
					} else if i == f-1 { // end-of-wildcard?
						fields.Add(value[done:])
						done = lv
					} else if greedy {
						list = true
					} else {
						greedy = true
					}
				} else if i == 0 {
					l := len(v)
					if l > len(value) || value[:l] != v {
						continue next // no match found for this sub-value
					}
					done = l
				} else if greedy { // find both first and last occurrence of next sub-value
					greedy = false
					if list { // return greedy region as multiple fields ?
						list = false
						for {
							pos := strings.Index(value[done:], v)
							if pos < 1 {
								break // end-of-list reached, no next separator found
							}
							fields.Add(value[done : done+pos])
							done += pos + len(v)
						}

					} else { // return greedy region as single (extra) field
						pos := strings.Index(value[done:], v)
						if pos < 1 {
							continue next // no match found for this sub-value
						}
						pos = strings.LastIndex(value[done:], v)
						fields.Add(value[done : done+pos]) // field relative to last occurence
						done += pos + len(v)
					}

				} else { // find first occurence of next sub-value
					pos := strings.Index(value[done:], v)
					if pos == -1 {
						continue next // no match found for this sub-value
					}
					fields.Add(value[done : done+pos])
					done += pos + len(v)
				}
			}
			match = done == lv
		}
		if match {
			break next
		}
	}
	return
}

// Wildcard does the reverse of IsWildcard() it fills in the fields that were extracted from a wildcard mask
func (l Strings) Wildcard(wildcard string) string {
	result := []byte{}
	f := 0

	for _, chr := range []byte(wildcard) {
		if chr == '*' {
			if f < len(l) {
				result = append(result, []byte(l[f])...)
				f++
			}
		} else {
			result = append(result, chr)
		}
	}
	return string(result)
}
