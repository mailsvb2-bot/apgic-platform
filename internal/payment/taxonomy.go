package payment

import (
	"errors"
	"strings"
)

type Provider string
type Method string
type Rail string

var ErrInvalidRoute = errors.New("invalid payment route")

type Route struct {
	Provider Provider
	Method   Method
	Rail     Rail
}

func (r Route) Validate() error {
	if strings.TrimSpace(string(r.Provider)) == "" ||
		strings.TrimSpace(string(r.Method)) == "" ||
		strings.TrimSpace(string(r.Rail)) == "" {
		return ErrInvalidRoute
	}
	return nil
}
