package models

import "net/http"

type Error struct {
	Status int `json:"-"`
	Codigo string `json:"codigo"`
	Mensaje string `json: "Mensaje"`
	Causa error `json:"-"`
}

type errorDeAPI struct {
	api string
	causa error
}

func (e *Error) Error() string {
	if e.Causa != nil {
		return e.Codigo + ": " + e.Mensaje + ": " + e.Causa.Error()
	}
	return e.Codigo + ": " + e.Mensaje
}

func Validacion(mensaje string) *Error {
	return &Error{Status: http.StatusBadRequest, Codigo: "VALIDACION", Mensaje: mensaje}
}

func NoAutorizado(mensaje string) *Error {
	return &Error{Status: http.StatusUnauthorized, Codigo: "NO_AUTORIZADO", Mensaje: mensaje}
}

func Prohibido(mensaje string) *Error {
	return &Error{Status: http.StatusForbidden, Codigo: "PROHIBIDO", Mensaje: mensaje}
}

func NoEncontrado(mensaje string) *Error{
	return &Error{Status: http.StatusNotFound, Codigo: "NO_ENCONTRADO", Mensaje: mensaje}
}

func Conflicto(mensaje string) *Error{
	return &Error{Status: http.StatusConflict, Codigo: "CONFLICTO", Mensaje: mensaje}
}

func ErrorAPI(api string, causa error) *Error{
	return &Error{
		Status: http.StatusBadGateway,
		Codigo: "UPSTREAM",
		Mensaje: "Un servicio no esta disponible en este momento. Intentalo de nuevo.",
		Causa: &errorDeAPI{api: api, causa: causa},
	}
}

func Interno(causa error) *Error {
	return &Error{Status: http.StatusInternalServerError, Codigo: "INTERNO", Mensaje: "Algo salio mal de nuestro lado.", Causa: causa}
}

func (e *errorDeAPI) Error() string { return e.api + ": " + e.causa.Error() }
func (e *errorDeAPI) Unwrap() error { return e.causa}