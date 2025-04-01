package main

// #Reply describes the full JSON response.
#Reply:
	#Commentary |
	#FullContent |
	#SelectionAppend |
	#SelectionReplace |
	#SelectionInsert |
	#FurtherInstructionNeeded	@go(,type=struct{reply})

// #GenericReply describes the structure shared by all
// reply messages.
#GenericReply: {
	type!: string
}

// #FurtherInstructionNeeded indicates that
// more information is needed before proceeding with
// the request.
#FurtherInstructionNeeded: {
	#GenericReply
	type!: "instruction"
	// message holds the message to the user
	// in markdown format.
	message!: string
}

// #FullContent holds the entire new contents of the file.
#FullContent: {
	#GenericReply
	type!:        "entire"
	fullContent?: string
}

// #Commentary holds some text to be given to the user
// rather than changing the file.
#Commentary: {
	#GenericReply
	type!: "commentary"
	text!: string
}

// #SelectionAppend holds some text to be appended to the
// current selection.
#SelectionAppend: {
	#GenericReply
	type!: "selectionAppend"
	text!: string
}

// #SelectionReplace holds some text to
#SelectionReplace: {
	#GenericReply
	type!: "selectionReplace"
	text!: string
}

// #SelectionInsert holds some text to be inserted at the
// start of the current selection.
#SelectionInsert: {
	#GenericReply
	type!: "selectionInsert"
	text!: string
}

// #Part describes the format of a part the request chat message.
#Part: {
	// instructions holds any instructions associated with this part of the
	// content.
	instructions!: string
	// filename optionally holds the name of the file that contains this
	// content
	filename?: string
	// base64 specifies whether the content is base64-encoded.
	base64!: bool
	// content holds the actual content. If base64 is true, this
	// will be base64-encoded.
	content!: string
}
