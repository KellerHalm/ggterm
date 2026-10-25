package terminal

import (
	"errors"
	"strings"
)

type ggtermGraphics struct {
	action string
	id     uint32
	format string
	width  int
	height int
	cols   int
	rows   int
	x      int
	y      int
	z      int
	path   string
	move   bool

	loop        bool
	autoplay    bool
	offset      float64
	position    float64
	hasOffset   bool
	hasPosition bool
}

func (t *Terminal) handleGgterm(data []byte) {
	if len(data) == 0 || data[0] != ';' {
		return
	}
	control, payload := splitControl(data[1:])
	command := parseGgtermGraphics(control)
	switch command.action {
	case "", "display":
		t.handleGgtermDisplay(&command, payload)
	case "transmit":
		t.handleGgtermTransmit(&command, payload)
	case "delete":
		if command.id != 0 {
			t.removeGraphicLocked(command.id)
		}
	case "video":
		t.startVideoLocked(&command)
	case "video_play", "video_pause", "video_seek", "video_stop":
		t.controlVideoLocked(command.action, &command)
	}
}

func parseGgtermGraphics(control []byte) ggtermGraphics {
	command := ggtermGraphics{action: "display", move: true, autoplay: true}
	for _, field := range strings.Split(string(control), ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "action":
			command.action = strings.ToLower(value)
		case "id":
			command.id = parseUint32(value)
		case "format", "f":
			command.format = strings.ToLower(value)
		case "width", "s":
			command.width = parseInt(value)
		case "height", "v":
			command.height = parseInt(value)
		case "cols", "c":
			command.cols = parseInt(value)
		case "rows", "r":
			command.rows = parseInt(value)
		case "x":
			command.x = parseInt(value)
		case "y":
			command.y = parseInt(value)
		case "z":
			command.z = parseInt(value)
		case "path":
			command.path = value
		case "move":
			command.move = value != "0" && !strings.EqualFold(value, "false")
		case "loop":
			command.loop = value != "0" && !strings.EqualFold(value, "false")
		case "play", "autoplay":
			command.autoplay = value != "0" && !strings.EqualFold(value, "false")
		case "offset":
			command.offset = parseFloat(value)
			command.hasOffset = true
		case "position":
			command.position = parseFloat(value)
			command.hasPosition = true
		}
	}
	return command
}

func (t *Terminal) handleGgtermDisplay(command *ggtermGraphics, payload []byte) {
	var graphic *Graphic
	if len(payload) == 0 && command.path == "" && command.id != 0 {
		graphic = t.graphics[command.id]
		if graphic == nil {
			return
		}
	} else {
		var err error
		graphic, err = t.storeGgtermGraphic(command, payload)
		if err != nil {
			return
		}
	}
	params := kittyParams{
		imageID: graphic.ID,
		cols:    command.cols,
		rows:    command.rows,
		cellX:   command.x,
		cellY:   command.y,
		z:       command.z,
	}
	t.placeGraphicLocked(graphic, &params, command.move)
}

func (t *Terminal) handleGgtermTransmit(command *ggtermGraphics, payload []byte) {
	_, _ = t.storeGgtermGraphic(command, payload)
}

func (t *Terminal) storeGgtermGraphic(command *ggtermGraphics, payload []byte) (*Graphic, error) {
	var raw []byte
	if command.path != "" {
		data, err := readGraphicPath(command.path, false)
		if err != nil {
			return nil, err
		}
		raw = data
	} else {
		data, err := decodeBase64(payload)
		if err != nil {
			return nil, errors.New("EINVAL:invalid base64 data")
		}
		raw = data
	}
	var pixels graphicPixels
	var err error
	if ownFormatIsImage(command.format) {
		pixels, err = decodeGraphicFile(raw)
	} else {
		switch command.format {
		case "rgb":
			pixels, err = decodeRawPixels(24, command.width, command.height, raw)
		case "rgba":
			pixels, err = decodeRawPixels(32, command.width, command.height, raw)
		default:
			err = errors.New("EINVAL:unsupported image format")
		}
	}
	if err != nil {
		return nil, err
	}
	return t.storeGraphicLocked(command.id, pixels)
}
