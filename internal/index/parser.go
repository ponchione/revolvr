package index

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	pythonDecl = regexp.MustCompile(`^(\s*)(?:async\s+def|def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	jsDecl     = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function|class|interface|type|enum)\s+([A-Za-z_$][A-Za-z0-9_$]*)|^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=.*(?:=>|function)`)
	sqlName    = regexp.MustCompile(`(?i)^\s*(?:create(?:\s+or\s+replace)?\s+)?(?:table|view|function|procedure|trigger|index)\s+(?:if\s+not\s+exists\s+)?([A-Za-z_][A-Za-z0-9_.$"]*)`)
	callName   = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_.$]*)\s*\(`)
)

type sourceRange struct {
	kind      string
	symbol    string
	signature string
	start     int
	end       int
	mode      string
	body      string
	refs      []edgeDraft
}

type edgeDraft struct {
	kind       string
	target     string
	targetPath string
	line       int
	from       string
}

func ParseFile(projectID string, file File, limits Limits) (ParsedFile, error) {
	limits, err := limits.normalized()
	if err != nil {
		return ParsedFile{}, err
	}
	language := languageForPath(file.Path)
	contentSHA := SHA256(file.Content)
	documentID := DeterministicID("document", projectID, file.Path)
	versionID := DeterministicID("document-version", documentID, contentSHA)
	provenance := StructuralProvenance{ParserVersion: ParserVersion, Parser: language + "-structural-v1", Mode: "syntax"}
	if len(file.Content) == 0 {
		provenance.Mode = "fallback"
		provenance.Reason = "empty file"
		return ParsedFile{
			Path: file.Path, Language: language, ContentSHA256: contentSHA, SizeBytes: 0,
			DocumentID: documentID, DocumentVersionID: versionID, StructuralProvenance: provenance,
		}, nil
	}

	var ranges []sourceRange
	switch language {
	case "go":
		ranges, err = parseGo(file.Content)
	case "python":
		ranges, err = parsePython(file.Content)
	case "typescript", "javascript":
		ranges, err = parseJavaScript(file.Content)
	case "markdown":
		ranges, err = parseMarkdown(file.Content)
	case "sql":
		ranges, err = parseSQL(file.Content)
	default:
		err = errors.New("unsupported syntax")
	}
	if err != nil || len(ranges) == 0 {
		provenance.Mode = "fallback"
		if err != nil {
			provenance.Reason = boundedReason(err.Error())
		} else {
			provenance.Reason = "no semantic declaration"
		}
		ranges = fallbackRanges(file.Content, limits)
	}

	parsed := ParsedFile{
		Path: file.Path, Language: language, ContentSHA256: contentSHA, SizeBytes: len(file.Content),
		DocumentID: documentID, DocumentVersionID: versionID, StructuralProvenance: provenance,
	}
	lineOffsets := lineOffsets(file.Content)
	ordinal := 0
	for _, candidate := range normalizeRanges(ranges, len(lineOffsets)-1) {
		parts := boundRange(candidate, file.Content, lineOffsets, limits)
		for partIndex, part := range parts {
			ordinal++
			body := part.body
			if body == "" {
				body = lines(file.Content, lineOffsets, part.start, part.end)
			}
			if strings.TrimSpace(body) == "" {
				continue
			}
			symbol := candidate.symbol
			kind := candidate.kind
			mode := candidate.mode
			if len(parts) > 1 {
				symbol = fmt.Sprintf("%s#part-%d", firstNonblank(symbol, file.Path), partIndex+1)
				kind += "_part"
				mode = "bounded-split"
			}
			chunkProvenance := provenance
			chunkProvenance.Mode = firstNonblank(mode, provenance.Mode)
			chunk := Chunk{
				DocumentID: documentID, DocumentVersionID: versionID, Ordinal: ordinal,
				Path: file.Path, Language: language, Kind: kind, Symbol: symbol,
				Signature: boundedSignature(firstNonblank(candidate.signature, firstLine(body))),
				StartLine: part.start, EndLine: part.end, Body: body, BodySHA256: SHA256([]byte(body)),
				StructuralProvenance: chunkProvenance,
			}
			chunk.ID = DeterministicID("chunk", versionID, fmt.Sprintf("%d", ordinal), chunk.BodySHA256, chunk.Symbol)
			parsed.Chunks = append(parsed.Chunks, chunk)
			if candidate.symbol != "" && partIndex == 0 {
				symbolRow := Symbol{
					DocumentVersionID: versionID, ChunkID: chunk.ID, Name: candidate.symbol,
					Kind: candidate.kind, Signature: chunk.Signature, StartLine: candidate.start, EndLine: candidate.end,
				}
				symbolRow.ID = DeterministicID("symbol", versionID, symbolRow.Name, fmt.Sprintf("%d", symbolRow.StartLine))
				parsed.Symbols = append(parsed.Symbols, symbolRow)
				for _, draft := range candidate.refs {
					parsed.Edges = append(parsed.Edges, Edge{
						DocumentVersionID: versionID, FromSymbolID: symbolRow.ID, Kind: draft.kind,
						TargetSymbol: draft.target, TargetPath: draft.targetPath, SourceLine: max(draft.line, candidate.start),
						Provenance: chunkProvenance,
					})
				}
			}
		}
	}
	for i := range parsed.Edges {
		edge := &parsed.Edges[i]
		edge.ID = DeterministicID("edge", versionID, edge.FromSymbolID, edge.Kind, edge.TargetSymbol, fmt.Sprintf("%d", edge.SourceLine))
	}
	if len(parsed.Chunks) == 0 {
		return ParsedFile{}, errors.New("code index: admitted text produced no bounded chunk")
	}
	return parsed, nil
}

func parseGo(content []byte) ([]sourceRange, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", content, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return nil, err
	}
	var ranges []sourceRange
	for _, declaration := range file.Decls {
		switch node := declaration.(type) {
		case *ast.FuncDecl:
			startPosition := node.Pos()
			if node.Doc != nil {
				startPosition = node.Doc.Pos()
			}
			start, end := fset.Position(startPosition).Line, fset.Position(node.End()).Line
			name := node.Name.Name
			if node.Recv != nil && len(node.Recv.List) > 0 {
				name = receiverName(node.Recv.List[0].Type) + "." + name
			}
			signatureEnd := node.Type.End()
			if node.Body != nil {
				signatureEnd = node.Body.Lbrace
			}
			signature := byteRange(content, fset.Position(node.Pos()).Offset, fset.Position(signatureEnd).Offset)
			candidate := sourceRange{kind: "function", symbol: name, signature: compact(signature), start: start, end: end, mode: "go-ast"}
			if node.Recv != nil {
				candidate.kind = "method"
			}
			ast.Inspect(node.Body, func(child ast.Node) bool {
				call, ok := child.(*ast.CallExpr)
				if !ok {
					return true
				}
				target := expressionName(call.Fun)
				if target != "" && target != name {
					candidate.refs = append(candidate.refs, edgeDraft{kind: "calls", target: target, line: fset.Position(call.Pos()).Line})
				}
				return true
			})
			ranges = append(ranges, candidate)
		case *ast.GenDecl:
			if node.Tok == token.IMPORT {
				continue
			}
			for _, spec := range node.Specs {
				name, kind := "", strings.ToLower(node.Tok.String())
				switch value := spec.(type) {
				case *ast.TypeSpec:
					name, kind = value.Name.Name, typeKind(value.Type)
				case *ast.ValueSpec:
					if len(value.Names) > 0 {
						name = value.Names[0].Name
					}
				}
				if name == "" {
					continue
				}
				start, end := fset.Position(spec.Pos()).Line, fset.Position(spec.End()).Line
				ranges = append(ranges, sourceRange{kind: kind, symbol: name, signature: compact(lines(content, lineOffsets(content), start, min(end, start+8))), start: start, end: end, mode: "go-ast"})
			}
		}
	}
	imports := make([]edgeDraft, 0, len(file.Imports))
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		imports = append(imports, edgeDraft{kind: "imports", target: path, targetPath: path, line: fset.Position(imported.Pos()).Line})
	}
	if len(ranges) > 0 && len(imports) > 0 {
		ranges[0].refs = append(ranges[0].refs, imports...)
	}
	return ranges, nil
}

func parsePython(content []byte) ([]sourceRange, error) {
	textLines := strings.Split(string(content), "\n")
	var ranges []sourceRange
	for i, line := range textLines {
		match := pythonDecl.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		indent := indentation(match[1])
		end := len(textLines)
		for j := i + 1; j < len(textLines); j++ {
			trimmed := strings.TrimSpace(textLines[j])
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if indentation(textLines[j]) <= indent {
				end = j
				break
			}
		}
		kind := "function"
		if strings.Contains(strings.TrimSpace(line), "class ") && !strings.HasPrefix(strings.TrimSpace(line), "def ") {
			kind = "class"
		} else if indent > 0 {
			kind = "method"
		}
		candidate := sourceRange{kind: kind, symbol: match[2], signature: compact(line), start: i + 1, end: max(i+1, end), mode: "python-indent"}
		for j := i; j < end; j++ {
			for _, call := range callName.FindAllStringSubmatch(textLines[j], -1) {
				if !pythonKeyword(call[1]) && call[1] != match[2] {
					candidate.refs = append(candidate.refs, edgeDraft{kind: "calls", target: call[1], line: j + 1})
				}
			}
		}
		ranges = append(ranges, candidate)
	}
	return ranges, nil
}

func parseJavaScript(content []byte) ([]sourceRange, error) {
	textLines := strings.Split(string(content), "\n")
	var ranges []sourceRange
	for i, line := range textLines {
		match := jsDecl.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		name := firstNonblank(match[1], match[2])
		end := braceRangeEnd(textLines, i)
		kind := "function"
		lower := strings.ToLower(line)
		for _, candidate := range []string{"class", "interface", "type", "enum"} {
			if strings.Contains(lower, candidate+" ") {
				kind = candidate
				break
			}
		}
		rangeItem := sourceRange{kind: kind, symbol: name, signature: compact(line), start: i + 1, end: end + 1, mode: "javascript-declaration"}
		for j := i; j <= end; j++ {
			for _, call := range callName.FindAllStringSubmatch(textLines[j], -1) {
				if call[1] != name && !javascriptKeyword(call[1]) {
					rangeItem.refs = append(rangeItem.refs, edgeDraft{kind: "calls", target: call[1], line: j + 1})
				}
			}
		}
		ranges = append(ranges, rangeItem)
	}
	return ranges, nil
}

func parseMarkdown(content []byte) ([]sourceRange, error) {
	textLines := strings.Split(string(content), "\n")
	var ranges []sourceRange
	for i, line := range textLines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level == 0 || level > 6 || level == len(trimmed) || trimmed[level] != ' ' {
			continue
		}
		end := len(textLines)
		for j := i + 1; j < len(textLines); j++ {
			next := strings.TrimSpace(textLines[j])
			nextLevel := 0
			for nextLevel < len(next) && next[nextLevel] == '#' {
				nextLevel++
			}
			if nextLevel > 0 && nextLevel <= level && nextLevel < len(next) && next[nextLevel] == ' ' {
				end = j
				break
			}
		}
		name := strings.TrimSpace(trimmed[level:])
		ranges = append(ranges, sourceRange{kind: "section", symbol: name, signature: trimmed, start: i + 1, end: max(i+1, end), mode: "markdown-heading"})
	}
	return ranges, nil
}

func parseSQL(content []byte) ([]sourceRange, error) {
	text := string(content)
	startLine, currentLine := 1, 1
	start := 0
	var ranges []sourceRange
	inSingle, inDouble := false, false
	for i, r := range text {
		if r == '\n' {
			currentLine++
		}
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case ';':
			if inSingle || inDouble {
				continue
			}
			statement := strings.TrimSpace(text[start : i+1])
			if statement != "" {
				name := ""
				if match := sqlName.FindStringSubmatch(statement); match != nil {
					name = strings.Trim(match[1], `"`)
				}
				ranges = append(ranges, sourceRange{kind: "statement", symbol: name, signature: boundedSignature(firstLine(statement)), start: startLine, end: currentLine, mode: "sql-statement"})
			}
			start, startLine = i+1, currentLine
		}
	}
	if strings.TrimSpace(text[start:]) != "" {
		statement := strings.TrimSpace(text[start:])
		name := ""
		if match := sqlName.FindStringSubmatch(statement); match != nil {
			name = strings.Trim(match[1], `"`)
		}
		ranges = append(ranges, sourceRange{kind: "statement", symbol: name, signature: boundedSignature(firstLine(statement)), start: startLine, end: currentLine, mode: "sql-statement"})
	}
	if inSingle || inDouble {
		return nil, errors.New("unterminated SQL quoted string")
	}
	return ranges, nil
}

func fallbackRanges(content []byte, limits Limits) []sourceRange {
	offsets := lineOffsets(content)
	total := max(1, len(offsets)-1)
	var ranges []sourceRange
	for start := 1; start <= total; {
		end := min(total, start+limits.MaxChunkLines-1)
		for end > start && len(lines(content, offsets, start, end)) > limits.MaxChunkBytes {
			end--
		}
		if end == start && len(lines(content, offsets, start, end)) > limits.MaxChunkBytes {
			// A single overlong line is bounded by bytes and remains exact through
			// adjacent parts. This is intentionally rare source-recovery behavior.
			body := lines(content, offsets, start, end)
			for offset := 0; offset < len(body); offset += limits.MaxChunkBytes {
				partEnd := min(len(body), offset+limits.MaxChunkBytes)
				ranges = append(ranges, sourceRange{kind: "fallback", signature: fmt.Sprintf("line %d bytes %d-%d", start, offset, partEnd), start: start, end: end, mode: "fallback-byte-window", body: body[offset:partEnd]})
			}
			start++
			continue
		}
		ranges = append(ranges, sourceRange{kind: "fallback", signature: fmt.Sprintf("lines %d-%d", start, end), start: start, end: end, mode: "fallback-line-window"})
		start = end + 1
	}
	return ranges
}

func normalizeRanges(input []sourceRange, totalLines int) []sourceRange {
	copyRanges := append([]sourceRange(nil), input...)
	sort.SliceStable(copyRanges, func(i, j int) bool {
		if copyRanges[i].start != copyRanges[j].start {
			return copyRanges[i].start < copyRanges[j].start
		}
		return copyRanges[i].end < copyRanges[j].end
	})
	result := make([]sourceRange, 0, len(copyRanges))
	for _, candidate := range copyRanges {
		candidate.start = max(1, candidate.start)
		candidate.end = min(max(candidate.start, candidate.end), max(1, totalLines))
		if len(result) > 0 && candidate.start == result[len(result)-1].start && candidate.end == result[len(result)-1].end && candidate.symbol == result[len(result)-1].symbol && candidate.body == result[len(result)-1].body && candidate.kind == result[len(result)-1].kind {
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func boundRange(candidate sourceRange, content []byte, offsets []int, limits Limits) []sourceRange {
	var parts []sourceRange
	if candidate.body != "" {
		for offset := 0; offset < len(candidate.body); offset += limits.MaxChunkBytes {
			end := min(len(candidate.body), offset+limits.MaxChunkBytes)
			parts = append(parts, sourceRange{start: candidate.start, end: candidate.end, body: candidate.body[offset:end]})
		}
		return parts
	}
	for start := candidate.start; start <= candidate.end; {
		end := min(candidate.end, start+limits.MaxChunkLines-1)
		for end > start && len(lines(content, offsets, start, end)) > limits.MaxChunkBytes {
			end--
		}
		body := lines(content, offsets, start, end)
		if len(body) > limits.MaxChunkBytes {
			for offset := 0; offset < len(body); offset += limits.MaxChunkBytes {
				partEnd := min(len(body), offset+limits.MaxChunkBytes)
				parts = append(parts, sourceRange{start: start, end: end, body: body[offset:partEnd]})
			}
		} else {
			parts = append(parts, sourceRange{start: start, end: end})
		}
		start = end + 1
	}
	return parts
}

func lineOffsets(content []byte) []int {
	offsets := []int{0}
	for i, value := range content {
		if value == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	if offsets[len(offsets)-1] != len(content) {
		offsets = append(offsets, len(content))
	} else if len(offsets) == 1 {
		offsets = append(offsets, len(content))
	}
	return offsets
}

func lines(content []byte, offsets []int, start, end int) string {
	if start < 1 || end < start || start >= len(offsets) {
		return ""
	}
	endOffset := len(content)
	if end < len(offsets)-1 {
		endOffset = offsets[end]
	}
	return string(content[offsets[start-1]:endOffset])
}

func byteRange(content []byte, start, end int) string {
	if start < 0 || end < start || start > len(content) {
		return ""
	}
	return string(content[start:min(end, len(content))])
}

func receiverName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	default:
		return "receiver"
	}
}

func expressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := expressionName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	case *ast.IndexExpr:
		return expressionName(value.X)
	case *ast.IndexListExpr:
		return expressionName(value.X)
	default:
		return ""
	}
}

func typeKind(expression ast.Expr) string {
	switch expression.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return "type"
	}
}

func braceRangeEnd(lines []string, start int) int {
	depth, seen := 0, false
	for i := start; i < len(lines); i++ {
		for _, value := range lines[i] {
			switch value {
			case '{':
				depth++
				seen = true
			case '}':
				depth--
			}
		}
		if seen && depth <= 0 {
			return i
		}
		if !seen && i > start && strings.TrimSpace(lines[i]) != "" {
			return i - 1
		}
	}
	return len(lines) - 1
}

func indentation(value string) int {
	count := 0
	for _, r := range value {
		if r == ' ' {
			count++
		} else if r == '\t' {
			count += 8
		} else {
			break
		}
	}
	return count
}

func compact(value string) string { return strings.Join(strings.Fields(value), " ") }
func firstLine(value string) string {
	if index := strings.IndexByte(value, '\n'); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return strings.TrimSpace(value)
}
func boundedSignature(value string) string {
	value = compact(value)
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}
func boundedReason(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = compact(value)
	if len(value) > 256 {
		return value[:256]
	}
	return value
}
func firstNonblank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
func pythonKeyword(value string) bool {
	switch value {
	case "if", "for", "while", "return", "yield", "print", "len", "str", "int", "list", "dict", "set", "super":
		return true
	default:
		return false
	}
}
func javascriptKeyword(value string) bool {
	switch value {
	case "if", "for", "while", "switch", "catch", "function", "typeof":
		return true
	default:
		return false
	}
}
