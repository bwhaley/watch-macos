package watch

import (
	"strconv"
	"strings"
)

const (
	attrBold      = 1
	attrDim       = 2
	attrItalic    = 3
	attrUnderline = 4
	attrBlink     = 5
	attrReverse   = 7
	attrHidden    = 8
	attrStrike    = 9
)

// style is the SGR state of one cell. A zero style is the terminal default.
type style struct {
	attrs uint16 // bit n set means SGR attribute n is on
	fg    string // SGR parameters, such as "31" or "38;5;208"
	bg    string
}

func (s style) with(attr int) style {
	s.attrs |= 1 << attr
	return s
}

// sgr is the escape sequence that sets s from any prior state.
func (s style) sgr() string {
	var b strings.Builder
	b.WriteString("\x1b[0")
	for attr := attrBold; attr <= attrStrike; attr++ {
		if s.attrs&(1<<attr) != 0 {
			b.WriteByte(';')
			b.WriteString(strconv.Itoa(attr))
		}
	}
	if s.fg != "" {
		b.WriteByte(';')
		b.WriteString(s.fg)
	}
	if s.bg != "" {
		b.WriteByte(';')
		b.WriteString(s.bg)
	}
	b.WriteByte('m')
	return b.String()
}

// apply updates s with the parameters of one SGR sequence, the text
// between "ESC [" and "m".
func (s *style) apply(params string) {
	if params == "" {
		*s = style{}
		return
	}
	fields := strings.FieldsFunc(
		params,
		func(r rune) bool { return r == ';' || r == ':' },
	)
	for i := 0; i < len(fields); i++ {
		code, err := strconv.Atoi(fields[i])
		if err != nil {
			continue
		}
		switch {
		case code == 0:
			*s = style{}
		case code >= attrBold && code <= attrStrike:
			s.attrs |= 1 << code
		case code == 21 || code == 22:
			s.attrs &^= 1<<attrBold | 1<<attrDim
		case code >= 23 && code <= 29:
			s.attrs &^= 1 << (code - 20)
		case code >= 30 && code <= 37, code >= 90 && code <= 97:
			s.fg = fields[i]
		case code == 39:
			s.fg = ""
		case code >= 40 && code <= 47, code >= 100 && code <= 107:
			s.bg = fields[i]
		case code == 49:
			s.bg = ""
		case code == 38 || code == 48:
			color, used := extendedColor(fields[i:])
			if code == 38 {
				s.fg = color
			} else {
				s.bg = color
			}
			i += used
		}
	}
}

// extendedColor reads a 256-color or truecolor parameter run that starts
// at fields[0] (38 or 48). It returns the run as SGR parameters and the
// number of fields it consumed after fields[0].
func extendedColor(fields []string) (string, int) {
	if len(fields) < 2 {
		return "", len(fields) - 1
	}
	n := 0
	switch fields[1] {
	case "5":
		n = 3
	case "2":
		n = 5
	default:
		return "", 1
	}
	if len(fields) < n {
		return "", len(fields) - 1
	}
	return strings.Join(fields[:n], ";"), n - 1
}
