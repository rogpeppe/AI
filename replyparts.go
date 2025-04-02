package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"iter"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/openai/openai-go/responses"
)

func partsIter(
	respIter iter.Seq2[responses.ResponseStreamEventUnion, error],
	save *bytes.Buffer,
) iter.Seq2[ReplyPart, error] {
	return func(yield func(ReplyPart, error) bool) {
		if err := yieldParts(respIter, yield, save); err != nil {
			yield(ReplyPart{}, err)
		}
	}
}

func yieldParts(
	respIter iter.Seq2[responses.ResponseStreamEventUnion, error],
	yield func(ReplyPart, error) bool,
	save *bytes.Buffer,
) error {
	pr, pw := io.Pipe()
	defer pr.Close()

	// Ensure that all text is saved in case something goes wrong.
	r := io.TeeReader(pr, save)

	go writeResponseText(pw, respIter)
	dec := jsontext.NewDecoder(r)

	if err := expectToken(dec, '{', ""); err != nil {
		return err
	}
	if err := expectToken(dec, '"', "parts"); err != nil {
		return err
	}
	if err := expectToken(dec, '[', ""); err != nil {
		return err
	}
	for {
		switch dec.PeekKind() {
		case ']':
			// Don't bother reading to the end; there's only one field.
			return nil
		case 0:
			_, err := dec.ReadToken()
			return err
		case '{':
			break
		default:
			tok, _ := dec.ReadToken()
			return fmt.Errorf("unexpected token; wanted '{' got %v", tok)
		}
		var part ReplyPart
		if err := json.UnmarshalDecode(dec, &part); err != nil {
			return err
		}
		if !yield(part, nil) {
			return nil
		}
	}
}

func expectToken(dec *jsontext.Decoder, kind jsontext.Kind, str string) error {
	tok, err := dec.ReadToken()
	if err != nil {
		return err
	}
	if tok.Kind() != kind {
		return fmt.Errorf("unexpected token; want %v but got %v", kind, tok.Kind())
	}
	if kind == '"' && tok.String() != str {
		return fmt.Errorf("unexpected token; want %q but got %q", str, tok.String())
	}
	return nil
}

func writeResponseText(pw *io.PipeWriter, respIter iter.Seq2[responses.ResponseStreamEventUnion, error]) {
	for ev, err := range respIter {
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			pw.CloseWithError(fmt.Errorf("streaming error: %w", err))
		}
		switch resp := ev.AsAny().(type) {
		case responses.ResponseTextDeltaEvent:
			if _, err := pw.Write([]byte(resp.Delta)); err != nil {
				return
			}
		case responses.ResponseTextDoneEvent:
		case responses.ResponseCompletedEvent:
			pw.Close()
			return
		}
	}
}
