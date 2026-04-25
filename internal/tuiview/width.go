package tuiview

import (
	"strings"
	"unicode/utf8"
)

func DisplayWidth(value string) int {
	width := 0
	for _, r := range stripANSI(value) {
		width += RuneWidth(r)
	}
	return width
}

func stripANSI(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); {
		if value[i] != 0x1b {
			r, size := utf8.DecodeRuneInString(value[i:])
			b.WriteRune(r)
			i += size
			continue
		}
		if i+1 >= len(value) {
			break
		}
		switch value[i+1] {
		case '[':
			i += 2
			for i < len(value) {
				c := value[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
		case ']':
			i += 2
			for i < len(value) {
				if value[i] == 0x07 {
					i++
					break
				}
				if value[i] == 0x1b && i+1 < len(value) && value[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default:
			i += 2
		}
	}
	return b.String()
}

func RuneWidth(r rune) int {
	switch {
	case r == '\n' || r == '\r' || r == '\t':
		return 0
	case r < 0x20:
		return 0
	case r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1faff)):
		return 2
	default:
		return 1
	}
}

func Truncate(value string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if DisplayWidth(value) <= maxWidth {
		return value
	}
	if maxWidth == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range value {
		rw := RuneWidth(r)
		if used+rw > maxWidth-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteRune('…')
	return b.String()
}

func PadRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	current := DisplayWidth(value)
	if current >= width {
		return value
	}
	return value + strings.Repeat(" ", width-current)
}

func Wrap(value string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		candidate := current + " " + word
		if DisplayWidth(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, splitLong(current, width)...)
		current = word
	}
	if current != "" {
		lines = append(lines, splitLong(current, width)...)
	}
	return lines
}

func splitLong(value string, width int) []string {
	if DisplayWidth(value) <= width {
		return []string{value}
	}
	var lines []string
	current := ""
	for _, r := range value {
		next := current + string(r)
		if current != "" && DisplayWidth(next) > width {
			lines = append(lines, current)
			current = string(r)
			continue
		}
		current = next
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
