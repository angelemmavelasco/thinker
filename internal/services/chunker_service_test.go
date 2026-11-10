package services

import (
	"strings"
	"testing"
)

func TestChunkText_SmallText(t *testing.T) {
	text := "Este es un texto corto que cabe perfectamente en un unico bloque."
	chunks := ChunkText(text, 200)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected chunk to equal text, got %q", chunks[0])
	}
}

func TestChunkText_EmptyAndBlank(t *testing.T) {
	if res := ChunkText("", 100); res != nil {
		t.Errorf("expected nil for empty text, got %v", res)
	}
	if res := ChunkText("   \n\n  \t ", 100); res != nil {
		t.Errorf("expected nil for whitespace text, got %v", res)
	}
}

func TestChunkText_ParagraphSeparators(t *testing.T) {
	p1 := "Primer parrafo con informacion sobre politicas de admision y requisitos generales."
	p2 := "Segundo parrafo completamente diferente que trata sobre reglas disciplinarias y sanciones."
	p3 := "Tercer parrafo que detalla la gestion de reclamos y apelaciones academicas."

	fullText := p1 + "\n\n" + p2 + "\n\n" + p3

	chunks := ChunkText(fullText, 120)

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}

	for _, c := range chunks {
		if strings.Contains(c, "admision") && strings.Contains(c, "disciplinarias") {
			t.Errorf("chunk bleeds between admission and disciplinary paragraphs: %q", c)
		}
	}
}

func TestChunkText_SentenceBoundary(t *testing.T) {
	text := "Primera oracion completa del documento normativo. Segunda oracion que complementa el articulo con precision. Tercera oracion con disposiciones finales del protocolo."

	chunks := ChunkText(text, 90)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	for _, c := range chunks {
		trimmed := strings.TrimSpace(c)
		if strings.HasSuffix(trimmed, "del") || strings.HasSuffix(trimmed, "con") {
			t.Errorf("chunk was cut mid-phrase without proper boundary: %q", trimmed)
		}
	}
}

func TestChunkTextWithMetadata(t *testing.T) {
	text := "Seccion de Seguridad.\n\nReglas de cifrado de contraseñas y llaves asimetricas.\n\nAuditorias periodicas y control de acceso."

	opts := ChunkOptions{
		MaxChars:     80,
		OverlapRatio: 0.10,
	}

	metaChunks := ChunkTextWithMetadata(text, opts)

	if len(metaChunks) == 0 {
		t.Fatalf("expected non-empty chunks with metadata")
	}

	for i, mc := range metaChunks {
		if mc.Index != i+1 {
			t.Errorf("expected chunk index %d, got %d", i+1, mc.Index)
		}
		if strings.TrimSpace(mc.Content) == "" {
			t.Errorf("chunk %d has empty content", mc.Index)
		}
	}
}

func TestChunkText_OverlapContinuity(t *testing.T) {
	text := "El sistema de proteccion de datos garantiza la confidencialidad de la victima y el secreto sumarial. El comite decretara medidas cautelares inmediatas dentro de las 24 horas siguientes a la recepcion de la denuncia."

	opts := ChunkOptions{
		MaxChars:     130,
		OverlapRatio: 0.15,
	}

	chunks := ChunkTextWithMetadata(text, opts)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	c1 := chunks[0].Content
	c2 := chunks[1].Content

	words1 := strings.Fields(c1)
	words2 := strings.Fields(c2)

	hasOverlapWord := false
	tailWords := words1[len(words1)-3:]
	for _, tw := range tailWords {
		for _, w2 := range words2[:4] {
			if strings.EqualFold(tw, w2) {
				hasOverlapWord = true
				break
			}
		}
		if hasOverlapWord {
			break
		}
	}

	if !hasOverlapWord {
		t.Errorf("expected semantic overlap between chunk 1 and chunk 2. Chunk 1: %q, Chunk 2: %q", c1, c2)
	}
}

func TestExtractPDFTextPureGo(t *testing.T) {
	fakePDF := []byte("stream\nBT /F1 12 Tf (Hello Pure Go World) Tj ET\nendstream")
	text, err := extractPDFTextPureGo(fakePDF)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello Pure Go World") {
		t.Errorf("expected extracted text to contain 'Hello Pure Go World', got: %q", text)
	}
}

func TestExtractPDFTextPureGo_CMapHex(t *testing.T) {
	cmapStream := "stream\nbegincmap\nbeginbfchar\n<0028> <0041>\n<004A> <0063>\nendbfchar\nendcmap\nendstream"
	contentStream := "stream\nBT /F1 12 Tf <0028004A> Tj ET\nendstream"
	fullPDF := []byte(cmapStream + "\n" + contentStream)

	text, err := extractPDFTextPureGo(fullPDF)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Ac") {
		t.Errorf("expected 'Ac', got: %q", text)
	}
}

func TestCollapseSpacedArtifacts(t *testing.T) {
	spaced := "L i n e a m i e n t o 1 7 . A u t o r i d a d R e s p o n s a b l e . L a C G A I"
	collapsed := collapseSpacedArtifacts(spaced)
	expected := "Lineamiento 17. Autoridad Responsable. La CGAI"
	if collapsed != expected {
		t.Errorf("expected %q, got %q", expected, collapsed)
	}

	normal := "Este es un parrafo completamente normal que no debe alterarse."
	if collapseSpacedArtifacts(normal) != normal {
		t.Errorf("expected normal text to remain unchanged")
	}
}
