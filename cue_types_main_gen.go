// Code generated by "cue exp gengotypes"; DO NOT EDIT.

package main

// #Reply describes the full JSON response.
type Reply struct{ reply }

// #GenericReply describes the structure shared by all
// reply messages.
type GenericReply struct {
	Type string `json:"type"`
}

// #FurtherInstructionNeeded indicates that
// more information is needed before proceeding with
// the request.
type FurtherInstructionNeeded struct {
	Type string `json:"type"`

	// message holds the message to the user
	// in markdown format.
	Message string `json:"message"`
}

// #FullContent holds the entire new contents of the file.
type FullContent struct {
	Type string `json:"type"`

	FullContent string `json:"fullContent,omitempty"`
}

// #Commentary holds some text to be given to the user
// rather than changing the file.
type Commentary struct {
	Type string `json:"type"`

	Text string `json:"text"`
}

// #SelectionAppend holds some text to be appended to the
// current selection.
type SelectionAppend struct {
	Type string `json:"type"`

	Text string `json:"text"`
}

// #SelectionReplace holds some text to
type SelectionReplace struct {
	Type string `json:"type"`

	Text string `json:"text"`
}

// #SelectionInsert holds some text to be inserted at the
// start of the current selection.
type SelectionInsert struct {
	Type string `json:"type"`

	Text string `json:"text"`
}

// #Part describes the format of a part the request chat message.
type Part struct {
	// instructions holds any instructions associated with this part of the
	// content.
	Instructions string `json:"instructions"`

	// filename optionally holds the name of the file that contains this
	// content
	Filename string `json:"filename,omitempty"`

	// base64 specifies whether the content is base64-encoded.
	Base64 bool `json:"base64"`

	// content holds the actual content. If base64 is true, this
	// will be base64-encoded.
	Content string `json:"content"`
}
