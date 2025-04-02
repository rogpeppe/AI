package main

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/sashabaranov/go-openai"
)

var replyTypes = map[string]reflect.Type{
	"instruction":      reflect.TypeFor[FurtherInstructionNeeded](),
	"entire":           reflect.TypeFor[FullContent](),
	"selectionAppend":  reflect.TypeFor[SelectionAppend](),
	"selectionReplace": reflect.TypeFor[SelectionReplace](),
	"selectionInsert":  reflect.TypeFor[SelectionInsert](),
	"commentary":       reflect.TypeFor[Commentary](),
}

type replyPart struct {
	content any
}

func (r *replyPart) AsAny() any {
	return r.content
}

func (r *replyPart) UnmarshalJSON(data []byte) error {
	var gr GenericReply
	if err := json.Unmarshal(data, &gr); err != nil {
		return err
	}
	t := replyTypes[gr.Type]
	if t == nil {
		return fmt.Errorf("unknown discrimination type %q", gr.Type)
	}
	actual := reflect.New(t).Interface()
	if err := json.Unmarshal(data, actual); err != nil {
		return err
	}
	r.content = actual
	return nil
}

func (p Part) AsOpenAI() (openai.ChatMessagePart, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return openai.ChatMessagePart{}, err
	}
	return openai.ChatMessagePart{
		Type: openai.ChatMessagePartTypeText,
		Text: string(data),
	}, nil
}
