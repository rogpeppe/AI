package main

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"flag"
	"fmt"
	"iter"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"9fans.net/go/acme"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

//go:generate cue exp gengotypes

//go:embed schema.cue
var schemaCUE string

func main() {
	if err := main1(); err != nil {
		fmt.Fprintf(os.Stderr, "AI: %v\n", err)
		os.Exit(1)
	}
}

type params struct {
	Filename     string
	Instructions string
}

var systemPrompt = `
I'm currently editing a file.

Your task is to produce the new contents of this file following any intent indicated
in the user instructions. Please produce me a JSON result with CUE schema
(indicated by #Reply); see the "schema" part for info on JSON schemas.

I've included other parts indicating aspects of the current edit session.
Each part is in JSON format described by the CUE #Part schema.
`

var (
	flagBig     = flag.Bool("big", false, "allow large files")
	flagModel   = flag.String("m", string(openai.ChatModelGPT4o), "OpenAI model to use")
	flagVerbose = flag.Bool("v", false, "enable verbose output")
)

func main1() error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `
usage: AI [<prompt> [file...]]

This executes OpenAI with the given instructions on the selection
in the current file.
Any files provided will be attached as context.
`)
		os.Exit(2)
	}
	flag.Parse()

	win, err := acmeCurrentWin()
	if err != nil {
		return err
	}
	defer win.CloseFiles()

	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return fmt.Errorf("no value set for $OPENAI_API_KEY")
	}

	var parts []Part
	parts = append(parts, Part{
		Instructions: "This holds the CUE schema for the JSON reply for you to send me",
		Content:      schemaCUE,
	})

	part, body, err := currentFilePart(win)
	if err != nil {
		return err
	}
	parts = append(parts, part)

	args := flag.Args()
	if len(args) > 0 {
		parts = append(parts, Part{
			Instructions: "This part holds the user instructions.",
			Content:      args[0],
		})
		args = args[1:]
	}
	for _, filename := range args {
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		if len(data) > 100*1024 && !*flagBig {
			return fmt.Errorf("refusing to send large file (%d bytes); use -big to override", len(data))
		}
		b64 := false
		if !utf8.Valid(data) {
			b64 = true
			data = base64.StdEncoding.AppendEncode(nil, data)
		}
		parts = append(parts, Part{
			Instructions: "this is a file attached by the user",
			Filename:     filename,
			Base64:       b64,
			Content:      string(data),
		})
	}

	// Build the messages
	systemMsg := responses.EasyInputMessageParam{
		Role: responses.EasyInputMessageRoleSystem,
		Content: responses.EasyInputMessageContentUnionParam{
			OfString: openai.Opt(systemPrompt),
		},
	}

	var userContent bytes.Buffer
	for _, p := range parts {
		aiPart, err := p.AsOpenAI()
		if err != nil {
			return fmt.Errorf("cannot marshal part: %v", err)
		}
		userContent.WriteString(aiPart.Text)
		userContent.WriteString("\n")
	}

	userMsg := responses.EasyInputMessageParam{
		Role: responses.EasyInputMessageRoleUser,
		Content: responses.EasyInputMessageContentUnionParam{
			OfString: openai.Opt(userContent.String()),
		},
	}

	// Create the client, relying on OPENAI_API_KEY in env
	client := openai.NewClient()

	ctx := context.Background()
	respIter := streamIter(client.Responses.NewStreaming(ctx, responses.ResponseNewParams{
		Model: responses.ChatModel(*flagModel),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				{OfMessage: &systemMsg},
				{OfMessage: &userMsg},
			},
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			},
		},
	}))

	var buf bytes.Buffer
	for part, err := range partsIter(respIter, &buf) {
		if err != nil {
			fmt.Printf("bad response:\n%s\n", buf.Bytes())
			return fmt.Errorf("error receiving reply: %v", err)
		}
		var newBody []byte
		switch r := part.AsAny().(type) {
		case *FurtherInstructionNeeded:
			fmt.Printf("further instruction needed: %s\n", r.Message)
			continue
		case *FullContent:
			newBody = body.text
		case *SelectionAppend:
			body.selection = slices.Concat(body.selection, []byte(r.Text))
			newBody = slices.Concat(body.head, body.selection, []byte(r.Text), body.tail)
		case *SelectionInsert:
			body.selection = slices.Concat([]byte(r.Text), body.selection)
			newBody = slices.Concat(body.head, body.selection, body.tail)
		case *SelectionReplace:
			body.selection = []byte(r.Text)
			newBody = slices.Concat(body.head, body.selection, body.tail)
		case *Commentary:
			fmt.Println(r.Text)
			continue
		default:
			return fmt.Errorf("unhandled reply type %T", r)
		}

		body.text = ensureNewline(body.text)
		newBody = ensureNewline(newBody)
		if err := doApply(win, body.text, newBody); err != nil {
			// TODO: ouch: this is racy.
			fmt.Printf("response:\n%s\n", buf.Bytes())
			return fmt.Errorf("cannot apply results to acme window: %v", err)
		}
		body.text = newBody
	}

	return nil
}

func ensureNewline(data []byte) []byte {
	if !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	return data
}

type bodyInfo struct {
	text      []byte
	head      []byte
	selection []byte
	tail      []byte
}

func currentFilePart(win *acme.Win) (part Part, info *bodyInfo, err error) {
	var buf bytes.Buffer
	if err := copyBody(&buf, win); err != nil {
		return Part{}, nil, fmt.Errorf("cannot copy window body: %v", err)
	}
	body := buf.Bytes()

	_, _, err = win.ReadAddr() // ensure address file is open
	if err != nil {
		return Part{}, nil, fmt.Errorf("cannot read address: %v", err)
	}
	if err := win.Ctl("addr=dot"); err != nil {
		return Part{}, nil, fmt.Errorf("cannot set address: %v", err)
	}
	a0, a1, err := win.ReadAddr()
	if err != nil {
		return Part{}, nil, fmt.Errorf("cannot get dot: %v", err)
	}
	a0b, a1b := runeOffset2ByteOffset(body, a0), runeOffset2ByteOffset(body, a1)

	head := body[:a0b]
	selection := body[a0b:a1b]
	tail := body[a1b:]

	delim := []byte(uniqID())
	hbody := slices.Concat(
		head,
		delim,
		selection,
		delim,
		tail,
	)

	tagBytes, err := win.ReadAll("tag")
	if err != nil {
		return Part{}, nil, fmt.Errorf("cannot read tag: %v", err)
	}
	filename, _, _ := strings.Cut(string(tagBytes), " ")

	part = Part{
		Instructions: fmt.Sprintf("Contents of the file currently being edited. The current selection is surrounded by the delimiter string %q", delim),
		Filename:     filename,
		Content:      string(hbody),
	}
	info = &bodyInfo{
		text:      body,
		head:      head,
		selection: selection,
		tail:      tail,
	}
	return part, info, nil
}

func uniqID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", buf)
}

type stream[T any] interface {
	Next() bool
	Current() T
	Err() error
}

func streamIter[T any](s stream[T]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for s.Next() {
			if !yield(s.Current(), s.Err()) {
				if s, _ := s.(interface{ Close() }); s != nil {
					s.Close()
				}
				return
			}
		}
	}
}
