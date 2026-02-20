// Package smartypants translates basic ASCII punctuation into
// fancy Unicode punctuation according to some simple contextual rules:
//
//  Input           Output         Remarks
//
//  "hello world"   “hello world”  Left and right double quote
//  'hello world'   ‘hello world’  Left and right single quote
//  hello-world     hello–world    No change
//  hello - world   hello – world  N-dash
//  hello--world    hello—world    M-dash
//  hello -- world  hello — world  M-dash
//  ...             …              Horizontal ellipsis
//  1/2 1/4 3/4     ½ ¼ ¾
//  (c) (r) (tm)    © ® ™
//
// See http://daringfireball.net/projects/smartypants/.
//
// Extracted and modified from the Blackfriday Markdown processor
// available at http://github.com/russross/blackfriday.
// Copyright © 2011 Russ Ross <russ@russross.com>.
// Distributed under the Simplified BSD License.
// See file License for details.
package smartypants

import (
	"bytes"
	"io"
)

const (
	LatexDashes = 1 << iota // translate -, --, and --- according to LaTeX rules
	Fractions               // translate arbitrary fractions with <sup> and <sub>
)

type smartypantsData struct {
	inSingleQuote bool
	inDoubleQuote bool
}

func wordBoundary(c byte) bool {
	return c == 0 || isspace(c) || ispunct(c)
}

func tolower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

func isdigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func smartQuoteHelper(w io.Writer, prev, next, quote byte, isOpen *bool) bool {
	// edge of the buffer is likely to be a tag that we don't get to see,
	// so we treat it like text sometimes

	// enumerate all sixteen possibilities for (prev, next)
	// each can be one of {0, space, punct, other}
	switch {
	case prev == 0 && next == 0:
		// context is not any help here, so toggle
		*isOpen = !*isOpen
	case isspace(prev) && next == 0:
		// [ "] might be [ "<code>foo...]
		*isOpen = true
	case ispunct(prev) && next == 0:
		// [!"] hmm... could be [Run!"] or [("<code>...]
		*isOpen = false
	case /* isnormal(prev) && */ next == 0:
		// [a"] is probably a close
		*isOpen = false
	case prev == 0 && isspace(next):
		// [" ] might be [...foo</code>" ]
		*isOpen = false
	case isspace(prev) && isspace(next):
		// [ " ] context is not any help here, so toggle
		*isOpen = !*isOpen
	case ispunct(prev) && isspace(next):
		// [!" ] is probably a close
		*isOpen = false
	case /* isnormal(prev) && */ isspace(next):
		// [a" ] this is one of the easy cases
		*isOpen = false
	case prev == 0 && ispunct(next):
		// ["!] hmm... could be ["$1.95] or [</code>"!...]
		*isOpen = false
	case isspace(prev) && ispunct(next):
		// [ "!] looks more like [ "$1.95]
		*isOpen = true
	case ispunct(prev) && ispunct(next):
		// [!"!] context is not any help here, so toggle
		*isOpen = !*isOpen
	case /* isnormal(prev) && */ ispunct(next):
		// [a"!] is probably a close
		*isOpen = false
	case prev == 0 /* && isnormal(next) */ :
		// ["a] is probably an open
		*isOpen = true
	case isspace(prev) /* && isnormal(next) */ :
		// [ "a] this is one of the easy cases
		*isOpen = true
	case ispunct(prev) /* && isnormal(next) */ :
		// [!"a] is probably an open
		*isOpen = true
	default:
		// [a'b] maybe a contraction?
		*isOpen = false
	}

	io.WriteString(w, quotes[quottyp{quote, *isOpen}])
	return true
}

type quottyp struct {
	num  byte
	open bool
}

var quotes = map[quottyp]string{
	quottyp{'s', true}:  "‘",
	quottyp{'d', true}:  "“",
	quottyp{'s', false}: "’",
	quottyp{'d', false}: "”",
}

func smartSingleQuote(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if len(text) >= 2 {
		t1 := tolower(text[1])

		if t1 == '\'' {
			nextChar := byte(0)
			if len(text) >= 3 {
				nextChar = text[2]
			}
			if smartQuoteHelper(w, previousChar, nextChar, 'd', &smrt.inDoubleQuote) {
				return 1
			}
		}

		if (t1 == 's' || t1 == 't' || t1 == 'm' || t1 == 'd') && (len(text) < 3 || wordBoundary(text[2])) {
			io.WriteString(w, "’")
			return 0
		}

		if len(text) >= 3 {
			t2 := tolower(text[2])

			if ((t1 == 'r' && t2 == 'e') || (t1 == 'l' && t2 == 'l') || (t1 == 'v' && t2 == 'e')) &&
				(len(text) < 4 || wordBoundary(text[3])) {
				io.WriteString(w, "’")
				return 0
			}
		}
	}

	nextChar := byte(0)
	if len(text) > 1 {
		nextChar = text[1]
	}
	if smartQuoteHelper(w, previousChar, nextChar, 's', &smrt.inSingleQuote) {
		return 0
	}

	w.Write(text[0:1])
	return 0
}

func smartParens(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if len(text) >= 3 {
		t1 := tolower(text[1])
		t2 := tolower(text[2])

		if t1 == 'c' && t2 == ')' {
			io.WriteString(w, "©")
			return 2
		}

		if t1 == 'r' && t2 == ')' {
			io.WriteString(w, "®")
			return 2
		}

		if len(text) >= 4 && t1 == 't' && t2 == 'm' && text[3] == ')' {
			io.WriteString(w, "™")
			return 3
		}
	}

	w.Write(text[0:1])
	return 0
}

func smartDash(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if len(text) >= 2 {
		if text[1] == '-' {
			io.WriteString(w, "—")
			return 1
		}

		if wordBoundary(previousChar) && wordBoundary(text[1]) {
			io.WriteString(w, "–")
			return 0
		}
	}

	w.Write(text[0:1])
	return 0
}

func smartDashLatex(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if len(text) >= 3 && text[1] == '-' && text[2] == '-' {
		io.WriteString(w, "—")
		return 2
	}
	if len(text) >= 2 && text[1] == '-' {
		io.WriteString(w, "–")
		return 1
	}

	w.Write(text[0:1])
	return 0
}

func smartAmp(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if bytes.HasPrefix(text, []byte("&quot;")) {
		nextChar := byte(0)
		if len(text) >= 7 {
			nextChar = text[6]
		}
		if smartQuoteHelper(w, previousChar, nextChar, 'd', &smrt.inDoubleQuote) {
			return 5
		}
	}

	if bytes.HasPrefix(text, []byte("&#0;")) {
		return 3
	}

	w.Write([]byte{'&'})
	return 0
}

func smartPeriod(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if len(text) >= 3 && text[1] == '.' && text[2] == '.' {
		io.WriteString(w, "…")
		return 2
	}

	if len(text) >= 5 && text[1] == ' ' && text[2] == '.' && text[3] == ' ' && text[4] == '.' {
		io.WriteString(w, "…")
		return 4
	}

	w.Write(text[0:1])
	return 0
}

func smartBacktick(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if len(text) >= 2 && text[1] == '`' {
		nextChar := byte(0)
		if len(text) >= 3 {
			nextChar = text[2]
		}
		if smartQuoteHelper(w, previousChar, nextChar, 'd', &smrt.inDoubleQuote) {
			return 1
		}
	}

	return 0
}

func smartNumberGeneric(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if wordBoundary(previousChar) && len(text) >= 3 {
		// is it of the form digits/digits(word boundary)?, i.e., \d+/\d+\b
		// note: check for regular slash (/) or fraction slash (⁄, 0x2044, or 0xe2 81 84 in utf-8)
		numEnd := 0
		for len(text) > numEnd && isdigit(text[numEnd]) {
			numEnd++
		}
		if numEnd == 0 {
			w.Write(text[0:1])
			return 0
		}
		denStart := numEnd + 1
		if len(text) > numEnd+3 && text[numEnd] == 0xe2 && text[numEnd+1] == 0x81 && text[numEnd+2] == 0x84 {
			denStart = numEnd + 3
		} else if len(text) < numEnd+2 || text[numEnd] != '/' {
			w.Write(text[0:1])
			return 0
		}
		denEnd := denStart
		for len(text) > denEnd && isdigit(text[denEnd]) {
			denEnd++
		}
		if denEnd == denStart {
			w.Write(text[0:1])
			return 0
		}
		if len(text) == denEnd || wordBoundary(text[denEnd]) {
			io.WriteString(w, "<sup>")
			w.Write(text[:numEnd])
			io.WriteString(w, "</sup>&frasl;<sub>")
			w.Write(text[denStart:denEnd])
			io.WriteString(w, "</sub>")
			return denEnd - 1
		}
	}

	w.Write(text[0:1])
	return 0
}

func smartNumber(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	if wordBoundary(previousChar) && len(text) >= 3 {
		if text[0] == '1' && text[1] == '/' && text[2] == '2' {
			if len(text) < 4 || wordBoundary(text[3]) {
				io.WriteString(w, "½")
				return 2
			}
		}

		if text[0] == '1' && text[1] == '/' && text[2] == '4' {
			if len(text) < 4 || wordBoundary(text[3]) || (len(text) >= 5 && tolower(text[3]) == 't' && tolower(text[4]) == 'h') {
				io.WriteString(w, "¼")
				return 2
			}
		}

		if text[0] == '3' && text[1] == '/' && text[2] == '4' {
			if len(text) < 4 || wordBoundary(text[3]) || (len(text) >= 6 && tolower(text[3]) == 't' && tolower(text[4]) == 'h' && tolower(text[5]) == 's') {
				io.WriteString(w, "¾")
				return 2
			}
		}
	}

	w.Write(text[0:1])
	return 0
}

func smartDoubleQuote(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	nextChar := byte(0)
	if len(text) > 1 {
		nextChar = text[1]
	}
	if !smartQuoteHelper(w, previousChar, nextChar, 'd', &smrt.inDoubleQuote) {
		io.WriteString(w, "&quot;")
	}

	return 0
}

func smartLeftAngle(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int {
	i := 0

	for i < len(text) && text[i] != '>' {
		i++
	}

	w.Write(text[:i+1])
	return i
}

type callback func(w io.Writer, smrt *smartypantsData, previousChar byte, text []byte) int

type renderer [256]callback

func smartypants(flags int) *renderer {
	r := new(renderer)
	r['"'] = smartDoubleQuote
	r['&'] = smartAmp
	r['\''] = smartSingleQuote
	r['('] = smartParens
	if flags&LatexDashes == 0 {
		r['-'] = smartDash
	} else {
		r['-'] = smartDashLatex
	}
	r['.'] = smartPeriod
	if flags&Fractions == 0 {
		r['1'] = smartNumber
		r['3'] = smartNumber
	} else {
		for ch := '1'; ch <= '9'; ch++ {
			r[ch] = smartNumberGeneric
		}
	}
	r['<'] = smartLeftAngle
	r['`'] = smartBacktick
	return r
}

type writer struct {
	w io.Writer
	s *renderer
	d smartypantsData
}

// New creates a smartypants filter. When data is written, the filter
// translates punctuation, then writes the translated data to w.
//
// Parameter flag selects alternate behavior (bitwise OR of LatexDashes etc.).
func New(w io.Writer, flag int) io.Writer {
	return &writer{
		w: w,
		s: smartypants(flag),
	}
}

func (w *writer) Write(p []byte) (int, error) {
	// first do normal entity escaping
	escaped := new(bytes.Buffer)
	attrEscape(escaped, p)
	p = escaped.Bytes()

	mark := 0
	for i := 0; i < len(p); i++ {
		if action := w.s[p[i]]; action != nil {
			if i > mark {
				w.w.Write(p[mark:i])
			}

			previousChar := byte(0)
			if i > 0 {
				previousChar = p[i-1]
			}
			i += action(w.w, &w.d, previousChar, p[i:])
			mark = i + 1
		}
	}
	if mark < len(p) {
		w.w.Write(p[mark:])
	}
	return len(p), nil
}

// Test if a character is a punctuation symbol.
// Taken from a private function in regexp in the stdlib.
func ispunct(c byte) bool {
	for _, r := range []byte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~") {
		if c == r {
			return true
		}
	}
	return false
}

// Test if a character is a whitespace character.
func isspace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func attrEscape(w *bytes.Buffer, src []byte) {
	org := 0
	for i, ch := range src {
		// using if statements is a bit faster than a switch statement.
		// as the compiler improves, this should be unnecessary
		// this is only worthwhile because attrEscape is the single
		// largest CPU user in normal use
		if ch == '"' {
			if i > org {
				// copy all the normal characters since the last escape
				w.Write(src[org:i])
			}
			org = i + 1
			io.WriteString(w, "&quot;")
			continue
		}
		if ch == '&' {
			if i > org {
				w.Write(src[org:i])
			}
			org = i + 1
			io.WriteString(w, "&amp;")
			continue
		}
		if ch == '<' {
			if i > org {
				w.Write(src[org:i])
			}
			org = i + 1
			io.WriteString(w, "&lt;")
			continue
		}
		if ch == '>' {
			if i > org {
				w.Write(src[org:i])
			}
			org = i + 1
			io.WriteString(w, "&gt;")
			continue
		}
	}
	if org < len(src) {
		w.Write(src[org:])
	}
}
