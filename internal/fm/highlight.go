package fm

import (
	"path/filepath"
	"strings"
	"unicode"
)

// Token identifies the syntax role of a preview span.
type Token uint8

const (
	TokenPlain Token = iota
	TokenKeyword
	TokenString
	TokenComment
	TokenNumber
	TokenType
	TokenFunction
	TokenBuiltin
	TokenKey
)

// Span is a run of text with one syntax token.
type Span struct {
	Text  string
	Token Token
}

// Line is a highlighted preview line.
type Line []Span

type language struct {
	keywords     map[string]bool
	types        map[string]bool
	builtins     map[string]bool
	lineComments []string
	blockOpen    string
	blockClose   string
	keyValue     bool
	hashSpace    bool
	tripleQuote  bool
	markdown     bool
}

type lexState struct {
	block string
	quote string
}

// Highlight applies a lightweight syntax highlighter based on the file
// extension. Unknown formats are returned as plain text.
func Highlight(path string, lines []string) []Line {
	lang := languageFor(path)
	out := make([]Line, 0, len(lines))
	state := &lexState{}
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if lang == nil {
			out = append(out, Line{{Text: line, Token: TokenPlain}})
			continue
		}
		out = append(out, highlightLine(line, lang, state))
	}
	return out
}

func highlightLine(line string, lang *language, state *lexState) Line {
	if lang.markdown {
		return highlightMarkdown(line)
	}
	runes := []rune(line)
	var spans Line
	var current strings.Builder
	currentToken := TokenPlain

	appendCurrent := func() {
		if current.Len() == 0 {
			return
		}
		spans = append(spans, Span{Text: current.String(), Token: currentToken})
		current.Reset()
	}
	emit := func(token Token, value string) {
		if value == "" {
			return
		}
		if token != currentToken {
			appendCurrent()
			currentToken = token
		}
		current.WriteString(value)
	}

	i := 0
	for i < len(runes) {
		if state.block != "" {
			if end := indexOf(runes, i, state.block); end >= 0 {
				emit(TokenComment, string(runes[i:end+len([]rune(state.block))]))
				i = end + len([]rune(state.block))
				state.block = ""
				continue
			}
			emit(TokenComment, string(runes[i:]))
			return appendSpan(spans, current.String(), currentToken)
		}
		if state.quote != "" {
			if end := indexOf(runes, i, state.quote); end >= 0 {
				delim := len([]rune(state.quote))
				emit(TokenString, string(runes[i:end+delim]))
				i = end + delim
				state.quote = ""
				continue
			}
			emit(TokenString, string(runes[i:]))
			return appendSpan(spans, current.String(), currentToken)
		}

		if lang.blockOpen != "" && hasPrefixRunes(runes, i, lang.blockOpen) {
			state.block = lang.blockClose
			emit(TokenComment, lang.blockOpen)
			i += len([]rune(lang.blockOpen))
			continue
		}
		if marker := lineCommentAt(runes, i, lang); marker > 0 {
			emit(TokenComment, string(runes[i:]))
			return appendSpan(spans, current.String(), currentToken)
		}

		if lang.tripleQuote && (hasPrefixRunes(runes, i, `"""`) || hasPrefixRunes(runes, i, `'''`)) {
			delim := string(runes[i : i+3])
			end := indexOf(runes, i+3, delim)
			if end >= 0 {
				emit(TokenString, string(runes[i:end+3]))
				i = end + 3
				continue
			}
			state.quote = delim
			emit(TokenString, string(runes[i:]))
			return appendSpan(spans, current.String(), currentToken)
		}

		r := runes[i]
		if r == '"' || r == '\'' || r == '`' {
			start := i
			i++
			escaped := false
			for i < len(runes) {
				if r != '`' && escaped {
					escaped = false
					i++
					continue
				}
				if r != '`' && runes[i] == '\\' {
					escaped = true
					i++
					continue
				}
				if runes[i] == r {
					i++
					break
				}
				i++
			}
			token := TokenString
			if lang.keyValue && nextNonSpaceIs(runes, i, ':', '=') {
				token = TokenKey
			}
			emit(token, string(runes[start:i]))
			continue
		}

		if unicode.IsDigit(r) {
			start := i
			for i < len(runes) && isNumberRune(runes[i]) {
				i++
			}
			emit(TokenNumber, string(runes[start:i]))
			continue
		}

		if isIdentStart(r) {
			start := i
			for i < len(runes) && isIdentRune(runes[i]) {
				i++
			}
			word := string(runes[start:i])
			token := classifyWord(word, lang, runes, i)
			emit(token, word)
			continue
		}

		emit(TokenPlain, string(r))
		i++
	}
	return appendSpan(spans, current.String(), currentToken)
}

func appendSpan(spans Line, text string, token Token) Line {
	if text != "" {
		spans = append(spans, Span{Text: text, Token: token})
	}
	return spans
}

func classifyWord(word string, lang *language, runes []rune, position int) Token {
	switch {
	case lang.keywords[word]:
		return TokenKeyword
	case lang.types[word]:
		return TokenType
	case lang.builtins[word]:
		return TokenBuiltin
	}
	if lang.keyValue && nextNonSpaceIs(runes, position, ':', '=') {
		return TokenKey
	}
	if nextNonSpaceRune(runes, position) == '(' {
		return TokenFunction
	}
	return TokenPlain
}

func nextNonSpaceRune(runes []rune, position int) rune {
	for i := position; i < len(runes); i++ {
		switch runes[i] {
		case ' ', '\t':
			continue
		default:
			return runes[i]
		}
	}
	return 0
}

func nextNonSpaceIs(runes []rune, position int, targets ...rune) bool {
	next := nextNonSpaceRune(runes, position)
	for _, target := range targets {
		if next == target {
			return true
		}
	}
	return false
}

func isNumberRune(r rune) bool {
	if unicode.IsDigit(r) || unicode.IsLetter(r) {
		return true
	}
	return r == '.' || r == '_'
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func hasPrefixRunes(runes []rune, position int, prefix string) bool {
	want := []rune(prefix)
	if position+len(want) > len(runes) {
		return false
	}
	for i, r := range want {
		if runes[position+i] != r {
			return false
		}
	}
	return true
}

func indexOf(runes []rune, position int, value string) int {
	want := []rune(value)
	if len(want) == 0 || position >= len(runes) {
		return -1
	}
	for i := position; i+len(want) <= len(runes); i++ {
		match := true
		for j, r := range want {
			if runes[i+j] != r {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func lineCommentAt(runes []rune, position int, lang *language) int {
	for _, marker := range lang.lineComments {
		if !hasPrefixRunes(runes, position, marker) {
			continue
		}
		if lang.hashSpace && marker == "#" && position > 0 {
			previous := runes[position-1]
			if previous != ' ' && previous != '\t' {
				continue
			}
		}
		return len([]rune(marker))
	}
	return 0
}

func highlightMarkdown(line string) Line {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "#"):
		return Line{{Text: line, Token: TokenKeyword}}
	case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "~~~"):
		return Line{{Text: line, Token: TokenComment}}
	case strings.HasPrefix(trimmed, ">"):
		return Line{{Text: line, Token: TokenComment}}
	}
	return Line{{Text: line, Token: TokenPlain}}
}

func languageFor(path string) *language {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case ext == ".go":
		return goLanguage
	case ext == ".py" || ext == ".pyw":
		return pythonLanguage
	case ext == ".js" || ext == ".mjs" || ext == ".cjs" || ext == ".jsx",
		ext == ".ts" || ext == ".tsx" || ext == ".mts" || ext == ".cts":
		return jsLanguage
	case ext == ".rs":
		return rustLanguage
	case ext == ".c" || ext == ".h" || ext == ".cc" || ext == ".cpp" ||
		ext == ".cxx" || ext == ".hpp" || ext == ".hh" || ext == ".java" ||
		ext == ".cs" || ext == ".kt" || ext == ".kts" || ext == ".swift" ||
		ext == ".m" || ext == ".mm" || ext == ".scala" || ext == ".dart":
		return cLikeLanguage
	case ext == ".sh" || ext == ".bash" || ext == ".zsh" || ext == ".fish" ||
		ext == ".ps1" || ext == ".psm1":
		return shellLanguage
	case ext == ".json" || ext == ".jsonc" || ext == ".json5":
		return jsonLanguage
	case ext == ".yaml" || ext == ".yml":
		return yamlLanguage
	case ext == ".toml":
		return tomlLanguage
	case ext == ".ini" || ext == ".cfg" || ext == ".conf":
		return iniLanguage
	case ext == ".md" || ext == ".markdown":
		return markdownLanguage
	}
	switch name {
	case "dockerfile", "makefile", "gemfile", "rakefile":
		return shellLanguage
	}
	return nil
}

func wordSet(words string) map[string]bool {
	set := make(map[string]bool)
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

var (
	goLanguage = &language{
		keywords:     wordSet("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var"),
		types:        wordSet("bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr any"),
		builtins:     wordSet("append cap clear close complex copy delete imag len make max min new panic print println real recover"),
		lineComments: []string{"//"},
		blockOpen:    "/*",
		blockClose:   "*/",
	}
	pythonLanguage = &language{
		keywords:     wordSet("and as assert async await break case class continue def del elif else except finally for from global if import in is lambda match nonlocal not or pass raise return try while with yield"),
		types:        wordSet("int float str bool list dict set tuple bytes complex object type None True False"),
		builtins:     wordSet("print len range enumerate zip map filter open super isinstance issubclass getattr setattr hasattr repr sorted sum min max abs any all iter next input"),
		lineComments: []string{"#"},
		hashSpace:    false,
		tripleQuote:  true,
	}
	jsLanguage = &language{
		keywords:     wordSet("const let var function return if else for while do switch case break continue new class extends super import export default try catch finally throw typeof instanceof in of delete void yield async await static get set this true false null undefined interface type enum implements namespace public private protected readonly abstract declare as keyof infer never unknown"),
		types:        wordSet("string number boolean any unknown never void object symbol bigint"),
		builtins:     wordSet("console document window JSON Math Object Array Promise String Number Boolean Date RegExp Map Set WeakMap WeakSet Symbol Error parseInt parseFloat isNaN require module exports"),
		lineComments: []string{"//"},
		blockOpen:    "/*",
		blockClose:   "*/",
	}
	rustLanguage = &language{
		keywords:     wordSet("fn let mut const static struct enum impl trait pub use mod crate super self Self match if else loop while for in return break continue where as dyn ref move unsafe async await type"),
		types:        wordSet("i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64 bool char str String Vec Option Result Box"),
		builtins:     wordSet("println print format vec panic assert assert_eq Some None Ok Err"),
		lineComments: []string{"//"},
		blockOpen:    "/*",
		blockClose:   "*/",
	}
	cLikeLanguage = &language{
		keywords:     wordSet("int char float double void if else for while do switch case break continue return struct union enum typedef static const sizeof goto extern volatile register inline class public private protected virtual override new delete namespace using template typename try catch throw this null nullptr true false bool auto import package extends implements interface final"),
		types:        wordSet("int8 int16 int32 int64 uint8 uint16 uint32 uint64 float32 float64 string byte boolean object"),
		builtins:     wordSet("printf scanf malloc free strlen strcpy memcpy memset"),
		lineComments: []string{"//"},
		blockOpen:    "/*",
		blockClose:   "*/",
	}
	shellLanguage = &language{
		keywords:     wordSet("if then else elif fi for while until do done case esac in function select time"),
		types:        nil,
		builtins:     wordSet("echo cd pwd export unset set local read printf test source alias unalias exit return shift trap wait kill jobs bg fg pushd popd type command exec eval"),
		lineComments: []string{"#"},
		hashSpace:    true,
	}
	jsonLanguage = &language{
		keywords: wordSet("true false null"),
		keyValue: true,
	}
	yamlLanguage = &language{
		keywords:     wordSet("true false null yes no on off"),
		types:        nil,
		lineComments: []string{"#"},
		hashSpace:    true,
		keyValue:     true,
	}
	tomlLanguage = &language{
		keywords:     wordSet("true false"),
		lineComments: []string{"#"},
		keyValue:     true,
	}
	iniLanguage = &language{
		keywords:     wordSet("true false"),
		lineComments: []string{"#", ";"},
		keyValue:     true,
	}
	markdownLanguage = &language{markdown: true}
)
