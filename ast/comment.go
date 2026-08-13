package ast

// Comment is one source comment. Text is Source[Start:End], including delimiters.
// The zero Position is Trailing.
type Comment struct {
	Start, End Idx // Inclusive start, exclusive end.
	AttachedTo Idx // Token start this comment is attached to.
	Kind       CommentKind
	Position   CommentPosition
	Newlines   CommentNewlines
	Content    CommentContent
}

type CommentKind uint8

const (
	CommentLine            CommentKind = iota // //
	CommentSingleLineBlock                    // /* ... */ with no line terminator
	CommentMultiLineBlock                     // /* ... */ with a line terminator
)

type CommentPosition uint8

const (
	// CommentTrailing is the zero value. New comments start here.
	CommentTrailing CommentPosition = iota
	CommentLeading
)

type CommentNewlines uint8

const (
	CommentNewlineNone     CommentNewlines = 0
	CommentNewlinePreceded CommentNewlines = 1 << 0
	CommentNewlineFollowed CommentNewlines = 1 << 1
)

type CommentContent uint8

const (
	ContentNone          CommentContent = iota
	ContentLegal                        // @license / @preserve / /*! / //!
	ContentJsdoc                        // /**
	ContentJsdocLegal                   // /** plus legal marker
	ContentPure                         // @__PURE__ / #__PURE__
	ContentNoSideEffects                // @__NO_SIDE_EFFECTS__ / #__NO_SIDE_EFFECTS__
	ContentDumpMeta                     // \d+ pc=\d+ dk=\d+
)

// Text returns src[Start:End]. It returns "" when the span is out of range.
func (c Comment) Text(src string) string {
	if int(c.End) > len(src) || c.End < c.Start {
		return ""
	}
	return src[c.Start:c.End]
}

// Body is the text between delimiters. It does not panic on unterminated /*.
// Line: drop the leading "//" when present.
// Block: drop "/*" and, only when the span ends in "*/", drop those two bytes.
func (c Comment) Body(src string) string {
	s := c.Text(src)
	if len(s) < 2 {
		return s
	}
	if c.Kind == CommentLine {
		if s[0] == '/' && s[1] == '/' {
			return s[2:]
		}
		return s
	}
	if s[0] != '/' || s[1] != '*' {
		return s
	}
	if len(s) >= 4 && s[len(s)-2] == '*' && s[len(s)-1] == '/' {
		return s[2 : len(s)-2]
	}
	return s[2:] // unterminated /*
}

func (c Comment) IsLeading() bool         { return c.Position == CommentLeading }
func (c Comment) IsTrailing() bool        { return c.Position == CommentTrailing }
func (c Comment) IsLine() bool            { return c.Kind == CommentLine }
func (c Comment) IsBlock() bool           { return c.Kind != CommentLine }
func (c Comment) PrecededByNewline() bool { return c.Newlines&CommentNewlinePreceded != 0 }
func (c Comment) FollowedByNewline() bool { return c.Newlines&CommentNewlineFollowed != 0 }
func (c Comment) IsLegal() bool           { return c.Content == ContentLegal || c.Content == ContentJsdocLegal }
func (c Comment) IsPure() bool            { return c.Content == ContentPure }
func (c Comment) IsNoSideEffects() bool   { return c.Content == ContentNoSideEffects }
func (c Comment) IsDumpMeta() bool        { return c.Content == ContentDumpMeta }
func (c Comment) IsAnnotation() bool {
	return c.Content == ContentPure || c.Content == ContentNoSideEffects || c.Content == ContentDumpMeta
}
func (c Comment) StayLeading() bool {
	switch c.Content {
	case ContentLegal, ContentJsdocLegal, ContentPure, ContentNoSideEffects, ContentDumpMeta:
		return true
	}
	return false
}

func (c *Comment) SetPrecededByNewline(v bool) {
	if v {
		c.Newlines |= CommentNewlinePreceded
	} else {
		c.Newlines &^= CommentNewlinePreceded
	}
}

func (c *Comment) SetFollowedByNewline(v bool) {
	if v {
		c.Newlines |= CommentNewlineFollowed
	} else {
		c.Newlines &^= CommentNewlineFollowed
	}
}
