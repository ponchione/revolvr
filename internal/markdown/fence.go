// Package markdown provides the small amount of Markdown structural scanning
// shared by Revolvr's task-import and receipt formats.
package markdown

import "strings"

// FenceLine classifies a line relative to a fenced code block.
type FenceLine uint8

const (
	LineOutsideFence FenceLine = iota
	LineFenceBoundary
	LineInsideFence
)

// Fence tracks one CommonMark-style backtick or tilde fence. Revolvr accepts
// fences indented by up to three spaces. A closing fence must use the opening
// marker, be at least as long, and contain only trailing horizontal whitespace.
type Fence struct {
	marker byte
	length int
}

// Scan classifies line and advances the fence state. Opening and closing lines
// are boundaries; every other line while a fence is open is inside the fence.
func (f *Fence) Scan(line string) FenceLine {
	line = strings.TrimSuffix(line, "\r")
	content, ok := fenceContent(line)
	if f.length > 0 {
		if ok && isClosingFence(content, f.marker, f.length) {
			f.marker = 0
			f.length = 0
			return LineFenceBoundary
		}
		return LineInsideFence
	}

	if !ok {
		return LineOutsideFence
	}
	marker, length, ok := openingFence(content)
	if !ok {
		return LineOutsideFence
	}
	f.marker = marker
	f.length = length
	return LineFenceBoundary
}

func fenceContent(line string) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return "", false
	}
	return line[indent:], true
}

func openingFence(content string) (byte, int, bool) {
	if len(content) < 3 || (content[0] != '`' && content[0] != '~') {
		return 0, 0, false
	}
	marker := content[0]
	length := markerLength(content, marker)
	if length < 3 {
		return 0, 0, false
	}
	if marker == '`' && strings.ContainsRune(content[length:], '`') {
		return 0, 0, false
	}
	return marker, length, true
}

func isClosingFence(content string, marker byte, openingLength int) bool {
	if content == "" || content[0] != marker {
		return false
	}
	length := markerLength(content, marker)
	return length >= openingLength && strings.Trim(content[length:], " \t") == ""
}

func markerLength(content string, marker byte) int {
	length := 0
	for length < len(content) && content[length] == marker {
		length++
	}
	return length
}
