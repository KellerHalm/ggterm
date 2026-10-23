package terminal

import (
	"fmt"
	"strconv"
)

const maxPendingResponses = 64

func (t *Terminal) deviceStatusReport(code int) {
	switch code {
	case 5:
		t.queueResponse("\x1b[0n")
	case 6:
		if t.parser.hasPrivate() {
			t.queueResponse(fmt.Sprintf("\x1b[?%d;%dR", t.cursor.Row+1, t.cursor.Col+1))
			return
		}
		t.queueResponse("\x1b[" + strconv.Itoa(t.cursor.Row+1) + ";" + strconv.Itoa(t.cursor.Col+1) + "R")
	}
}

func (t *Terminal) deviceAttributes() {
	if t.parser.hasPrivate() || len(t.parser.inter) > 0 {
		return
	}
	t.queueResponse("\x1b[?6c")
}

func (t *Terminal) queueResponse(response string) {
	if len(t.responses) >= maxPendingResponses {
		return
	}
	t.responses = append(t.responses, []byte(response))
}
