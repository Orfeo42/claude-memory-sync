package ops

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	fileMarkerPrefix     = "===FILE: "
	deleteMarkerPrefix   = "===DELETE: "
	proposalMarkerPrefix = "===PROPOSAL: "
	markerSuffix         = "==="
	endMarker            = "===END==="
	nothingMarker        = "NOTHING"
)

type parseError string

func (e parseError) Error() string { return string(e) }

const (
	ErrUnterminatedBlock parseError = "unterminated op block"
	ErrEmptyOpName       parseError = "empty op name"
)

type FileOp struct {
	Name    string
	Content []byte
}

type Result struct {
	Files     []FileOp
	Deletes   []string
	Proposals []FileOp
	Nothing   bool
}

type blockKind int

const (
	blockNone blockKind = iota
	blockFile
	blockProposal
)

type parser struct {
	scanner    *bufio.Scanner
	result     Result
	sawNothing bool
	kind       blockKind
	name       string
	lines      []string
}

func Parse(r io.Reader) (Result, error) {
	p := &parser{scanner: bufio.NewScanner(r)}
	p.scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if err := p.run(); err != nil {
		return Result{}, err
	}

	if len(p.result.Files) == 0 && len(p.result.Deletes) == 0 && len(p.result.Proposals) == 0 {
		p.result.Nothing = p.sawNothing
	}

	return p.result, nil
}

func (p *parser) run() error {
	for p.scanner.Scan() {
		line := strings.TrimRight(p.scanner.Text(), " \t")
		if err := p.handleLine(line); err != nil {
			return err
		}
	}

	if err := p.scanner.Err(); err != nil {
		return fmt.Errorf("ops: scan input: %w", err)
	}

	if p.kind != blockNone {
		return fmt.Errorf("ops: %s: %w", p.name, ErrUnterminatedBlock)
	}

	return nil
}

func (p *parser) handleLine(line string) error {
	if p.kind != blockNone {
		return p.handleBlockLine(line)
	}

	return p.handleTopLevelLine(line)
}

func (p *parser) handleBlockLine(line string) error {
	if line == endMarker {
		p.closeBlock()
		return nil
	}

	p.lines = append(p.lines, line)
	return nil
}

func (p *parser) handleTopLevelLine(line string) error {
	switch {
	case strings.HasPrefix(line, fileMarkerPrefix):
		return p.openBlock(blockFile, line, fileMarkerPrefix)
	case strings.HasPrefix(line, proposalMarkerPrefix):
		return p.openBlock(blockProposal, line, proposalMarkerPrefix)
	case strings.HasPrefix(line, deleteMarkerPrefix):
		return p.handleDelete(line)
	case line == nothingMarker:
		p.sawNothing = true
	}

	return nil
}

func (p *parser) openBlock(kind blockKind, line, prefix string) error {
	name, err := extractName(line, prefix)
	if err != nil {
		return err
	}

	p.kind = kind
	p.name = name
	p.lines = nil
	return nil
}

func (p *parser) handleDelete(line string) error {
	name, err := extractName(line, deleteMarkerPrefix)
	if err != nil {
		return err
	}

	p.result.Deletes = append(p.result.Deletes, name)
	return nil
}

func (p *parser) closeBlock() {
	content := []byte(strings.Join(p.lines, "\n") + "\n")
	op := FileOp{Name: p.name, Content: content}

	switch p.kind {
	case blockFile:
		p.result.Files = append(p.result.Files, op)
	case blockProposal:
		p.result.Proposals = append(p.result.Proposals, op)
	case blockNone:
	}

	p.kind = blockNone
	p.name = ""
	p.lines = nil
}

func extractName(line, prefix string) (string, error) {
	rest := strings.TrimPrefix(line, prefix)
	name := strings.TrimSuffix(rest, markerSuffix)
	name = strings.TrimSpace(name)

	if name == "" {
		return "", fmt.Errorf("ops: marker %q: %w", strings.TrimSpace(line), ErrEmptyOpName)
	}

	return name, nil
}
