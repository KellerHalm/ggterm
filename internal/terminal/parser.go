package terminal

import "strconv"

type parserState uint8

const (
	stateGround parserState = iota
	stateEscape
	stateEscapeCharset
	stateCSI
	stateOSC
	stateIgnore
)

type parser struct {
	state  parserState
	params []int
	inter  []byte
	oscBuf []byte
}

func newParser() *parser {
	return &parser{}
}

func (p *parser) reset() {
	p.state = stateGround
	p.params = p.params[:0]
	p.inter = p.inter[:0]
	p.oscBuf = p.oscBuf[:0]
}

func (p *parser) parseParams(data []byte) []int {
	p.params = p.params[:0]
	if len(data) == 0 {
		return p.params
	}
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == ';' {
			if i > start {
				if n, err := strconv.Atoi(string(data[start:i])); err == nil {
					p.params = append(p.params, n)
				}
			} else {
				p.params = append(p.params, 0)
			}
			start = i + 1
		}
	}
	if start < len(data) {
		if n, err := strconv.Atoi(string(data[start:])); err == nil {
			p.params = append(p.params, n)
		}
	} else if len(data) > 0 && data[len(data)-1] == ';' {
		p.params = append(p.params, 0)
	}
	return p.params
}

func (p *parser) param(i, def int) int {
	if i < len(p.params) && p.params[i] > 0 {
		return p.params[i]
	}
	return def
}

func (p *parser) hasPrivate() bool {
	return len(p.inter) > 0 && p.inter[0] == '?'
}
