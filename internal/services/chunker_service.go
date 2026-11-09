package services

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const (
	DefaultChunkMaxChars     = 450
	DefaultChunkOverlapRatio = 0.12
)

type ChunkOptions struct {
	MaxChars     int
	OverlapRatio float64
}

type TextChunk struct {
	Index   int
	Content string
}

var allowedExtensions = map[string]bool{
	".txt":  true,
	".md":   true,
	".csv":  true,
	".docx": true,
	".pdf":  true,
}

func ExtractTextFromUpload(fileHeader *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedExtensions[ext] {
		return "", fmt.Errorf("formato no permitido (%s): solo se aceptan .txt, .md, .pdf, .docx y .csv", ext)
	}

	f, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer f.Close()

	rawBytes, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	switch ext {
	case ".txt", ".md":
		return strings.TrimSpace(string(rawBytes)), nil
	case ".csv":
		return extractCSVText(rawBytes)
	case ".docx":
		return extractDOCXText(rawBytes)
	case ".pdf":
		return extractPDFText(rawBytes)
	default:
		return "", fmt.Errorf("formato no soportado: %s", ext)
	}
}

func extractCSVText(data []byte) (string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("error leyendo archivo CSV: %w", err)
	}
	if len(rows) == 0 {
		return "", nil
	}

	var headers []string
	var startIdx int
	if len(rows) > 1 {
		headers = rows[0]
		startIdx = 1
	}

	var lines []string
	for i := startIdx; i < len(rows); i++ {
		row := rows[i]
		var parts []string
		for j, val := range row {
			v := strings.TrimSpace(val)
			if v == "" {
				continue
			}
			if j < len(headers) && strings.TrimSpace(headers[j]) != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", strings.TrimSpace(headers[j]), v))
			} else {
				parts = append(parts, v)
			}
		}
		if len(parts) > 0 {
			lines = append(lines, strings.Join(parts, " | "))
		}
	}

	return strings.Join(lines, "\n\n"), nil
}

func extractDOCXText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("archivo DOCX inválido: %w", err)
	}

	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()

		decoder := xml.NewDecoder(rc)
		var paragraphs []string
		var currentPara strings.Builder

		for {
			tok, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}

			switch se := tok.(type) {
			case xml.StartElement:
				if se.Name.Local == "t" {
					var text string
					if err := decoder.DecodeElement(&text, &se); err == nil {
						currentPara.WriteString(text)
					}
				} else if se.Name.Local == "tab" {
					currentPara.WriteString(" ")
				} else if se.Name.Local == "br" {
					currentPara.WriteString("\n")
				}
			case xml.EndElement:
				if se.Name.Local == "p" {
					p := strings.TrimSpace(currentPara.String())
					if p != "" {
						paragraphs = append(paragraphs, p)
					}
					currentPara.Reset()
				}
			}
		}

		if p := strings.TrimSpace(currentPara.String()); p != "" {
			paragraphs = append(paragraphs, p)
		}

		return strings.Join(paragraphs, "\n\n"), nil
	}

	return "", fmt.Errorf("no se encontró contenido en el archivo DOCX")
}

var pdfStreamRegex = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
var pdfBTETRegex = regexp.MustCompile(`(?s)\bBT\b(.*?)\bET\b`)
var pdfParenTextRegex = regexp.MustCompile(`\((?:\\.|[^\\()])*\)`)

func extractPDFText(data []byte) (string, error) {
	if text, err := extractPDFWithQuartz(data); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}

	return extractPDFTextPureGo(data)
}

func extractPDFWithQuartz(data []byte) (string, error) {
	tmpFile, err := os.CreateTemp("", "thinker_pdf_*.pdf")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return "", err
	}
	_ = tmpFile.Close()

	script := `
ObjC.import('Foundation');
ObjC.import('Quartz');
function run(argv) {
	var url = $.NSURL.fileURLWithPath(argv[0]);
	var doc = $.PDFDocument.alloc.initWithURL(url);
	if (!doc || doc.isNil()) return "";
	var count = doc.pageCount;
	var pages = [];
	for (var i = 0; i < count; i++) {
		var p = doc.pageAtIndex(i);
		if (p && !p.isNil()) {
			var s = p.string;
			if (s && !s.isNil()) {
				pages.push(ObjC.unwrap(s));
			}
		}
	}
	return pages.join("\n\n");
}
`
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", script, tmpPath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	cleaned := cleanPDFExtractedText(string(out))
	if cleaned == "" {
		return "", fmt.Errorf("no se encontró texto legible en el PDF")
	}
	return cleaned, nil
}

func extractPDFTextPureGo(data []byte) (string, error) {
	var extracted []string
	matches := pdfStreamRegex.FindAllSubmatch(data, -1)

	for _, m := range matches {
		streamData := bytes.TrimSpace(m[1])
		zr, err := zlib.NewReader(bytes.NewReader(streamData))
		if err != nil {
			continue
		}
		decoded, rErr := io.ReadAll(zr)
		_ = zr.Close()
		if rErr != nil || len(decoded) == 0 {
			continue
		}

		btBlocks := pdfBTETRegex.FindAllSubmatch(decoded, -1)
		if len(btBlocks) == 0 {
			continue
		}

		var pageBuilder strings.Builder
		for _, block := range btBlocks {
			tokens := pdfParenTextRegex.FindAll(block[1], -1)
			for _, tok := range tokens {
				if len(tok) <= 2 {
					continue
				}
				inner := string(tok[1 : len(tok)-1])
				inner = strings.ReplaceAll(inner, `\n`, "\n")
				inner = strings.ReplaceAll(inner, `\r`, " ")
				inner = strings.ReplaceAll(inner, `\t`, " ")
				inner = strings.ReplaceAll(inner, `\(`, "(")
				inner = strings.ReplaceAll(inner, `\)`, ")")
				inner = strings.ReplaceAll(inner, `\\`, `\`)

				cleaned := cleanPrintableText(inner)
				if cleaned != "" {
					pageBuilder.WriteString(cleaned)
					pageBuilder.WriteString(" ")
				}
			}
		}

		segment := strings.TrimSpace(pageBuilder.String())
		if len(segment) > 2 {
			extracted = append(extracted, segment)
		}
	}

	if len(extracted) == 0 {
		return "", fmt.Errorf("no se pudo extraer texto legible del PDF")
	}

	return strings.Join(extracted, "\n\n"), nil
}

func isReadableLatinRune(r rune) bool {
	if r == '\n' || r == '\t' || r == ' ' {
		return true
	}
	if unicode.In(r, unicode.Latin) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '.', ',', ';', ':', '!', '¡', '?', '¿', '(', ')', '[', ']', '-', '–', '—', '"', '\'', '«', '»', '/', '%', '+', '=', '*':
		return true
	}
	return false
}

func cleanPDFExtractedText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	var cleanedLines []string

	for _, line := range lines {
		var b strings.Builder
		for _, r := range line {
			if isReadableLatinRune(r) {
				b.WriteRune(r)
			} else if unicode.IsSpace(r) {
				b.WriteRune(' ')
			}
		}
		trimmed := strings.Join(strings.Fields(b.String()), " ")
		if trimmed == "" {
			cleanedLines = append(cleanedLines, "")
			continue
		}
		cleanedLines = append(cleanedLines, trimmed)
	}

	raw := strings.Join(cleanedLines, "\n")
	multiNewline := regexp.MustCompile(`\n{3,}`)
	raw = multiNewline.ReplaceAllString(raw, "\n\n")
	return strings.TrimSpace(raw)
}

func cleanPrintableText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if isReadableLatinRune(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func ChunkText(text string, maxChars int) []string {
	if maxChars <= 0 {
		maxChars = DefaultChunkMaxChars
	}
	chunks := ChunkTextWithMetadata(text, ChunkOptions{
		MaxChars:     maxChars,
		OverlapRatio: DefaultChunkOverlapRatio,
	})
	if len(chunks) == 0 {
		return nil
	}
	res := make([]string, len(chunks))
	for i, c := range chunks {
		res[i] = c.Content
	}
	return res
}

func ChunkTextWithMetadata(text string, opts ChunkOptions) []TextChunk {
	if opts.MaxChars <= 0 {
		opts.MaxChars = DefaultChunkMaxChars
	}
	if opts.OverlapRatio < 0 || opts.OverlapRatio > 0.5 {
		opts.OverlapRatio = DefaultChunkOverlapRatio
	}

	rawChunks := chunkTextInternal(text, opts.MaxChars, opts.OverlapRatio)
	result := make([]TextChunk, len(rawChunks))
	for i, c := range rawChunks {
		result[i] = TextChunk{
			Index:   i + 1,
			Content: c,
		}
	}
	return result
}

func chunkTextInternal(text string, maxChars int, overlapRatio float64) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	multiNewline := regexp.MustCompile(`\n{3,}`)
	normalized = multiNewline.ReplaceAllString(normalized, "\n\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return nil
	}

	if len([]rune(normalized)) <= maxChars {
		return []string{normalized}
	}

	units := splitUnitsRecursively(normalized, maxChars)
	if len(units) == 0 {
		return nil
	}

	maxOverlap := int(float64(maxChars) * overlapRatio)
	if maxOverlap < 0 {
		maxOverlap = 0
	}

	var chunks []string
	var currentChunk strings.Builder
	currentLen := 0

	for _, unit := range units {
		unit = strings.TrimSpace(unit)
		if unit == "" {
			continue
		}

		unitRunes := len([]rune(unit))
		sep := " "
		if currentLen == 0 {
			sep = ""
		}
		needed := currentLen + len([]rune(sep)) + unitRunes

		if currentLen > 0 && needed > maxChars {
			chunkStr := strings.TrimSpace(currentChunk.String())
			if chunkStr != "" {
				chunks = append(chunks, chunkStr)
			}
			currentChunk.Reset()
			currentLen = 0

			if maxOverlap > 0 && len([]rune(chunkStr)) > 0 {
				overlapText := extractTailOverlap(chunkStr, maxOverlap)
				if overlapText != "" && len([]rune(overlapText))+1+unitRunes <= maxChars {
					currentChunk.WriteString(overlapText)
					currentChunk.WriteString(" ")
					currentLen = len([]rune(overlapText)) + 1
				}
			}
		}

		if currentLen > 0 && !strings.HasSuffix(currentChunk.String(), " ") {
			currentChunk.WriteString(" ")
			currentLen++
		}
		currentChunk.WriteString(unit)
		currentLen += unitRunes
	}

	if currentLen > 0 {
		chunkStr := strings.TrimSpace(currentChunk.String())
		if chunkStr != "" {
			chunks = append(chunks, chunkStr)
		}
	}

	return chunks
}

func splitUnitsRecursively(text string, maxChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	runes := []rune(text)
	if len(runes) <= maxChars {
		return []string{text}
	}

	if strings.Contains(text, "\n\n") {
		return splitByDelimAndRecurse(text, "\n\n", maxChars, false)
	}

	if strings.Contains(text, "\n") {
		return splitByDelimAndRecurse(text, "\n", maxChars, false)
	}

	sentences := splitSentencesNatural(text)
	if len(sentences) > 1 {
		var result []string
		for _, s := range sentences {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if len([]rune(s)) <= maxChars {
				result = append(result, s)
			} else {
				result = append(result, splitUnitsRecursively(s, maxChars)...)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	for _, punct := range []string{"; ", ", ", " - "} {
		if strings.Contains(text, punct) {
			return splitByDelimAndRecurse(text, punct, maxChars, true)
		}
	}

	if strings.Contains(text, " ") {
		return splitByDelimAndRecurse(text, " ", maxChars, false)
	}

	var rawParts []string
	for start := 0; start < len(runes); start += maxChars {
		end := start + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		rawParts = append(rawParts, string(runes[start:end]))
	}
	return rawParts
}

func splitByDelimAndRecurse(text string, delim string, maxChars int, preserveDelim bool) []string {
	parts := strings.Split(text, delim)
	var result []string
	for i, part := range parts {
		if preserveDelim && i < len(parts)-1 {
			part = part + strings.TrimSpace(delim)
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if len([]rune(part)) <= maxChars {
			result = append(result, part)
		} else {
			result = append(result, splitUnitsRecursively(part, maxChars)...)
		}
	}
	return result
}

func splitSentencesNatural(s string) []string {
	var res []string
	var buf strings.Builder
	runes := []rune(s)

	for i := 0; i < len(runes); i++ {
		buf.WriteRune(runes[i])
		if (runes[i] == '.' || runes[i] == ';' || runes[i] == '!' || runes[i] == '?') &&
			(i+1 == len(runes) || unicode.IsSpace(runes[i+1])) {
			trimmed := strings.TrimSpace(buf.String())
			if trimmed != "" {
				res = append(res, trimmed)
			}
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		trimmed := strings.TrimSpace(buf.String())
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

func extractTailOverlap(text string, maxOverlap int) string {
	runes := []rune(text)
	if len(runes) <= maxOverlap {
		return ""
	}
	tail := runes[len(runes)-maxOverlap:]
	firstSpace := -1
	for i, r := range tail {
		if unicode.IsSpace(r) {
			firstSpace = i
			break
		}
	}
	if firstSpace >= 0 && firstSpace+1 < len(tail) {
		return strings.TrimSpace(string(tail[firstSpace+1:]))
	}
	return strings.TrimSpace(string(tail))
}
