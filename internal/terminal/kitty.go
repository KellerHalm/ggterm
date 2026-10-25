package terminal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type kittyParams struct {
	action      string
	quiet       int
	format      int
	medium      string
	width       int
	height      int
	dataSize    int
	dataOffset  int
	imageID     uint32
	imageNumber uint32
	placementID uint32
	compression string
	more        bool
	sourceX     int
	sourceY     int
	sourceW     int
	sourceH     int
	cellX       int
	cellY       int
	cols        int
	rows        int
	cursorMove  int
	z           int
	deleteSpec  string
}

type kittyPending struct {
	params kittyParams
	data   []byte
}

func (t *Terminal) executeAPC(data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case 'G':
		t.handleKitty(data[1:])
	case 'g':
		if strings.HasPrefix(string(data), "ggterm") {
			t.handleGgterm(data[len("ggterm"):])
		}
	}
}

func parseKittyParams(control []byte) (*kittyParams, error) {
	params := &kittyParams{
		action: "t",
		format: 32,
		medium: "d",
	}
	for _, field := range strings.Split(string(control), ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" {
			return nil, errors.New("EINVAL:invalid control data")
		}
		switch key {
		case "a":
			params.action = value
		case "q":
			params.quiet = parseInt(value)
		case "f":
			params.format = parseInt(value)
		case "t":
			params.medium = value
		case "s":
			params.width = parseInt(value)
		case "v":
			params.height = parseInt(value)
		case "S":
			params.dataSize = parseInt(value)
		case "O":
			params.dataOffset = parseInt(value)
		case "i":
			params.imageID = parseUint32(value)
		case "I":
			params.imageNumber = parseUint32(value)
		case "p":
			params.placementID = parseUint32(value)
		case "o":
			params.compression = value
		case "m":
			params.more = value == "1"
		case "x":
			params.sourceX = parseInt(value)
		case "y":
			params.sourceY = parseInt(value)
		case "w":
			params.sourceW = parseInt(value)
		case "h":
			params.sourceH = parseInt(value)
		case "X":
			params.cellX = parseInt(value)
		case "Y":
			params.cellY = parseInt(value)
		case "c":
			params.cols = parseInt(value)
		case "r":
			params.rows = parseInt(value)
		case "C":
			params.cursorMove = parseInt(value)
		case "z":
			params.z = parseInt(value)
		case "d":
			params.deleteSpec = value
		}
	}
	if params.action == "" {
		params.action = "t"
	}
	return params, nil
}

func (t *Terminal) handleKitty(data []byte) {
	control, payload := splitControl(data)
	params, err := parseKittyParams(control)
	if err != nil {
		t.queueGraphicResponse(0, 0, 0, err.Error())
		return
	}

	if t.pendingGraphic != nil {
		pending := t.pendingGraphic
		if len(pending.data)+len(payload) > maxGraphicPayload {
			t.pendingGraphic = nil
			t.queueGraphicResponse(pending.params.imageID, pending.params.placementID, pending.params.quiet, "ENOSPC:image is too large")
			return
		}
		pending.data = append(pending.data, payload...)
		if !params.more {
			t.pendingGraphic = nil
			t.executeKitty(&pending.params, pending.data)
		}
		return
	}

	if params.more {
		if len(payload) > maxGraphicPayload {
			t.queueGraphicResponse(params.imageID, params.placementID, params.quiet, "ENOSPC:image is too large")
			return
		}
		t.pendingGraphic = &kittyPending{
			params: *params,
			data:   append([]byte(nil), payload...),
		}
		return
	}
	t.executeKitty(params, payload)
}

func (t *Terminal) executeKitty(params *kittyParams, payload []byte) {
	switch params.action {
	case "q":
		t.queueGraphicResponse(params.imageID, params.placementID, params.quiet, "OK")
	case "t", "T":
		t.handleKittyTransmit(params, payload)
	case "p":
		t.handleKittyPlace(params)
	case "d":
		t.handleKittyDelete(params)
	default:
		t.queueGraphicResponse(params.imageID, params.placementID, params.quiet, "EINVAL:unsupported action")
	}
}

func (t *Terminal) handleKittyTransmit(params *kittyParams, payload []byte) {
	pixels, err := t.readKittyPixels(params, payload)
	if err != nil {
		t.queueGraphicResponse(params.imageID, params.placementID, params.quiet, err.Error())
		return
	}
	if params.imageID == 0 && params.imageNumber == 0 {
		params.imageID = t.allocGraphicIDLocked()
	}
	graphic, err := t.storeGraphicLocked(params.imageID, pixels)
	if err != nil {
		t.queueGraphicResponse(params.imageID, params.placementID, params.quiet, err.Error())
		return
	}
	params.imageID = graphic.ID
	if params.imageNumber != 0 {
		if t.imageNumbers == nil {
			t.imageNumbers = make(map[uint32]uint32)
		}
		t.imageNumbers[params.imageNumber] = graphic.ID
	}
	if params.action == "T" {
		t.placeGraphicLocked(graphic, params, params.cursorMove != 1)
	}
	t.queueGraphicResponse(params.imageID, params.placementID, params.quiet, "OK")
}

func (t *Terminal) handleKittyPlace(params *kittyParams) {
	graphic, id := t.resolveImageLocked(params)
	if graphic == nil {
		t.queueGraphicResponse(id, params.placementID, params.quiet, "ENOENT:image not found")
		return
	}
	t.placeGraphicLocked(graphic, params, params.cursorMove != 1)
	t.queueGraphicResponse(graphic.ID, params.placementID, params.quiet, "OK")
}

func (t *Terminal) handleKittyDelete(params *kittyParams) {
	spec := params.deleteSpec
	if spec == "" {
		spec = "a"
	}
	removeMatching := func(match func(graphicPlacement) bool, freeData bool) {
		ids := make(map[uint32]bool)
		if freeData {
			for _, placement := range t.placements {
				if match(placement) {
					ids[placement.imageID] = true
				}
			}
		}
		t.removePlacementsLocked(match)
		for id := range ids {
			t.removeGraphicLocked(id)
		}
	}
	switch spec {
	case "a", "A":
		if spec == "A" {
			t.graphics = nil
			t.placements = nil
			t.graphicBytes = 0
			t.imageNumbers = nil
			break
		}
		t.removePlacementsLocked(func(graphicPlacement) bool { return true })
	case "i", "I":
		id := params.imageID
		if id == 0 && params.imageNumber != 0 {
			id = t.latestImageForNumberLocked(params.imageNumber)
		}
		if id == 0 {
			break
		}
		if spec == "I" {
			t.removeGraphicLocked(id)
		} else if params.placementID != 0 {
			t.removePlacementsLocked(func(p graphicPlacement) bool {
				return p.imageID == id && p.placementID == params.placementID
			})
		} else {
			t.removePlacementsLocked(func(p graphicPlacement) bool { return p.imageID == id })
		}
	case "p", "P":
		row, col := params.sourceY, params.sourceX
		removeMatching(func(p graphicPlacement) bool {
			screenRow := p.absRow
			if !p.alt {
				screenRow -= t.scrollBase
			}
			return intersectsCell(p, screenRow, p.col, row, col)
		}, spec == "P")
	case "x", "X":
		col := params.sourceX
		removeMatching(func(p graphicPlacement) bool {
			return p.col <= col && col < p.col+p.cols
		}, spec == "X")
	case "y", "Y":
		row := params.sourceY
		removeMatching(func(p graphicPlacement) bool {
			screenRow := p.absRow
			if !p.alt {
				screenRow -= t.scrollBase
			}
			return screenRow <= row && row < screenRow+p.rows
		}, spec == "Y")
	case "z", "Z":
		removeMatching(func(p graphicPlacement) bool { return p.z == params.z }, spec == "Z")
	case "c", "C":
		row, col := t.cursor.Row, t.cursor.Col
		removeMatching(func(p graphicPlacement) bool {
			screenRow := p.absRow
			if !p.alt {
				screenRow -= t.scrollBase
			}
			return intersectsCell(p, screenRow, p.col, row, col)
		}, spec == "C")
	}
	if params.imageID != 0 || params.imageNumber != 0 {
		t.queueGraphicResponse(params.imageID, params.placementID, params.quiet, "OK")
	}
}

func intersectsCell(p graphicPlacement, row, col, checkRow, checkCol int) bool {
	return row <= checkRow && checkRow < row+p.rows &&
		col <= checkCol && checkCol < col+p.cols
}

func (t *Terminal) readKittyPixels(params *kittyParams, payload []byte) (graphicPixels, error) {
	var data []byte
	switch params.medium {
	case "", "d":
		decoded, err := decodeBase64(payload)
		if err != nil {
			return graphicPixels{}, errors.New("EINVAL:invalid base64 data")
		}
		data = decoded
	case "f", "t":
		decoded, err := decodeBase64(payload)
		if err != nil {
			return graphicPixels{}, errors.New("EBADF:Failed to read image file")
		}
		path := string(decoded)
		if params.dataOffset > 0 || params.dataSize > 0 {
			raw, err := readGraphicPath(path, params.medium == "t")
			if err != nil {
				return graphicPixels{}, err
			}
			start := params.dataOffset
			if start < 0 || start > len(raw) {
				return graphicPixels{}, errors.New("EBADF:Failed to read image file")
			}
			end := len(raw)
			if params.dataSize > 0 && start+params.dataSize < end {
				end = start + params.dataSize
			}
			data = raw[start:end]
		} else {
			raw, err := readGraphicPath(path, params.medium == "t")
			if err != nil {
				return graphicPixels{}, err
			}
			data = raw
		}
	default:
		return graphicPixels{}, errors.New("ENOTSUP:unsupported transmission medium")
	}
	if params.compression == "z" {
		inflated, err := inflate(data)
		if err != nil {
			return graphicPixels{}, err
		}
		data = inflated
	}
	switch params.format {
	case 100:
		return decodeGraphicFile(data)
	case 24, 32:
		return decodeRawPixels(params.format, params.width, params.height, data)
	default:
		return graphicPixels{}, fmt.Errorf("EINVAL:unsupported image format %d", params.format)
	}
}

func (t *Terminal) queueGraphicResponse(id, placement uint32, quiet int, message string) {
	if message == "OK" {
		if quiet >= 1 {
			return
		}
	} else if quiet >= 2 {
		return
	}
	if id == 0 && placement == 0 {
		return
	}
	var response strings.Builder
	response.WriteString("\x1b_G")
	if id != 0 {
		response.WriteString("i=")
		response.WriteString(strconv.FormatUint(uint64(id), 10))
	}
	if placement != 0 {
		if id != 0 {
			response.WriteByte(',')
		}
		response.WriteString("p=")
		response.WriteString(strconv.FormatUint(uint64(placement), 10))
	}
	response.WriteByte(';')
	response.WriteString(message)
	response.WriteString("\x1b\\")
	t.queueResponse(response.String())
}
