package document

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/svera/coreander/v4/internal/webserver/model"
)

var errInvalidReadingPositionJSON = errors.New("invalid reading position JSON")

func parseReadingPositionJSON(raw []byte) (position string, fraction *float64, err error) {
	if len(raw) == 0 {
		return "", nil, errInvalidReadingPositionJSON
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", nil, err
	}
	get := func(lower, upper string) (json.RawMessage, bool) {
		if v, ok := m[lower]; ok {
			return v, true
		}
		if v, ok := m[upper]; ok {
			return v, true
		}
		return nil, false
	}
	rawPos, ok := get("position", "Position")
	if !ok {
		return "", nil, errInvalidReadingPositionJSON
	}
	if err := json.Unmarshal(rawPos, &position); err != nil {
		return "", nil, err
	}

	if rawFr, ok := get("fraction", "Fraction"); ok {
		frac, perr := parseFractionJSON(rawFr)
		if perr == nil && frac != nil {
			f := model.ClampReadingFraction(*frac)
			fraction = &f
		}
	}
	return position, fraction, nil
}

func parseFractionJSON(raw json.RawMessage) (*float64, error) {
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return &f, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	x, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return &x, nil
}

func (d *Controller) UpdatePosition(c fiber.Ctx) error {
	document, err := d.idx.Document(c.Params("slug"))
	if err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	if document.Slug == "" {
		return fiber.ErrNotFound
	}

	var session model.Session
	if val, ok := c.Locals("Session").(model.Session); ok {
		session = val
	}

	if session.ID == 0 {
		return fiber.ErrUnauthorized
	}

	position, fraction, err := parseReadingPositionJSON(c.Body())
	if err != nil {
		return fiber.ErrBadRequest
	}
	if fraction != nil && math.IsNaN(*fraction) {
		return fiber.ErrBadRequest
	}

	if err := d.readingRepository.Update(int(session.ID), document.Slug, position, fraction); err != nil {
		log.Println(err)
		return fiber.ErrInternalServerError
	}

	return c.SendStatus(fiber.StatusNoContent)
}
