package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/sashabaranov/go-openai"
)

func main() {
	if err := main1(); err != nil {
		fmt.Fprintf(os.Stderr, "AI: %v\n", err)
		os.Exit(1)
	}
}

type params struct {
	Filename     string
	Delim        string
	Instructions string
	Content      string
	Selection    string
}

var contextPromptTmpl = template.Must(template.New("").Parse(`
I'm about to show you some content in a file.
{{if .Filename}}The file is named {{printf "%q" .Filename}}.{{end}}
I have selected a section of the file and indicated the start and end of the selection with the text {{printf "%q" .Delim}}.
Please follow these instructions regarding the highlighted section and show me exactly the new contents of the file after following the instructons. The instructions are quoted using the Go quoting rules:
The instructions are: {{printf "%q" .Instructions}}.
The content of the file is as follows: ` + "```" + `
{{.Content}}` + "```" + `
`))

var noContextPromptTmpl = template.Must(template.New("").Parse(`
I'm about to show you some content from a file.
{{if .Filename}}The file is named {{printf "%q" .Filename}}.{{end}}
The content holds a selection from within the file, quoted using Go quoting rules.
Please follow these instructions regarding the content and show me exactly the new contents of the selection after following them, in a Markdown-formatted code block. The instructions are quoted using the Go quoting rules:
The instructions are: {{printf "%q" .Instructions}}.
The content is: {{printf "%q" .Selection}}.
`))

var cflag = flag.Bool("c", false, "include entire file as context")

func main1() error {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `
usage: AI instructions...

This executes OpenAI with the given instructions on the selection
in the current file.
`)
		os.Exit(2)
	}

	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
	}

	win, err := acmeCurrentWin()
	if err != nil {
		return err
	}
	defer win.CloseFiles()

	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return fmt.Errorf("no value set for $OPENAI_API_KEY")
	}
	client := openai.NewClient(key)

	var buf bytes.Buffer
	if err := copyBody(&buf, win); err != nil {
		return fmt.Errorf("cannot copy window body: %v", err)
	}
	body := buf.Bytes()
	_, _, err = win.ReadAddr() // make sure address file is already open.
	if err != nil {
		return fmt.Errorf("cannot read address: %v", err)
	}
	if err := win.Ctl("addr=dot"); err != nil {
		return fmt.Errorf("cannot set address: %v", err)
	}
	delim := uniqID()
	a0, a1, err := win.ReadAddr()
	if err != nil {
		return fmt.Errorf("cannot get dot: %v", err)
	}
	a0b, a1b := runeOffset2ByteOffset(body, a0), runeOffset2ByteOffset(body, a1)
	selection := body[a0b:a1b]
	var hbody []byte
	hbody = append(hbody, body[:a0b]...)
	hbody = append(hbody, delim...)
	hbody = append(hbody, selection...)
	hbody = append(hbody, delim...)
	hbody = append(hbody, body[a1b:]...)

	tagBytes, err := win.ReadAll("tag")
	if err != nil {
		return fmt.Errorf("cannot read tag: %v", err)
	}
	filename, _, _ := strings.Cut(string(tagBytes), " ")
	instructions := strings.Join(flag.Args(), " ")

	var tmpl *template.Template
	if *cflag {
		tmpl = contextPromptTmpl
	} else {
		tmpl = noContextPromptTmpl
	}
	var finalPrompt bytes.Buffer
	if err := tmpl.Execute(&finalPrompt, params{
		Filename:     filename,
		Delim:        delim,
		Selection:    string(selection),
		Instructions: instructions,
		Content:      string(hbody),
	}); err != nil {
		return fmt.Errorf("failed to execute prompt template: %v", err)
	}
	resp, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: finalPrompt.String(),
				// Content: fmt.Sprintf("%s:\n%s", input, os.Args[1:]),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("cannot generate: %v (%#v)\n", err, err)
	}
	fmt.Printf("AI in progress: ")

	var result bytes.Buffer
	for {
		resp, err := resp.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error receiving data: %v", err)
		}
		if resp.Choices[0].FinishReason == "stop" {
			break
		}
		result.WriteString(resp.Choices[0].Delta.Content)
		fmt.Printf(".")
	}
	fmt.Printf("\ndone (%q)\n", result.Bytes())
	newBody := result.Bytes()
	i0 := bytes.Index(newBody, []byte("```"))
	if i0 == -1 {
		return fmt.Errorf("no markdown code block found in %q", newBody)
	}
	// Skip code block type string.
	nl := bytes.IndexByte(newBody[i0:], '\n')
	if nl == -1 {
		return fmt.Errorf("no newline found after code block start in %q", newBody)
	}
	i0 += nl + 1
	i1 := bytes.LastIndex(newBody, []byte("```"))
	if i0 == i1 {
		return fmt.Errorf("resulting block does not look right (%q)", newBody)
	}
	newBody = newBody[i0:i1]
	if *cflag {
		if err := doApply(win, body, newBody); err != nil {
			return fmt.Errorf("cannot apply results: %v", err)
		}
	} else {
		if _, err := win.Write("data", newBody); err != nil {
			return fmt.Errorf("cannot write selection: %v", err)
		}
	}
	return nil
}

func uniqID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", buf)
}
