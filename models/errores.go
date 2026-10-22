// Contrato de errores de errores del middleware.
//
// Los upstream de Jobsy_API son inconsistentes de tres formas a la vez:
//
//  1. El HTTP status es 200 casi siempre; el codigo real viaja dentro del body.
//  2. El envelope tiene tres formas: {"data":...}, {"date":...} (typo, en 56
//     sitios) y a veces un string pelado con el error crudo de PostgreSQL.
//  3. Un POST con JSON invalido en Core_api responde 200 con body vacio.
//
// El mid no propaga nada de eso. Hacia el frontend hay un unico formato, con
// el status HTTP correcto, y los mensajes de PostgreSQL nunca salen al cliente.

package models

import (
	"errors"
	"fmt"
	"net/http"
)

// Codigo es un identificador estable que el frontend puede usar en un switch.
// A diferencia del mensaje, no cambia aunque reescribamos el texto.
type Codigo string

const (
	CodValidacion   Codigo = "VALIDACION"
	CodNoAutorizado Codigo = "NO_AUTORIZADO"
	CodProhibido    Codigo = "PROHIBIDO"
	CodNoEncontrado Codigo = "NO_ENCONTRADO"
	CodConflicto    Codigo = "CONFLICTO"
	CodUpstream     Codigo = "UPSTREAM"
	CodInterno      Codigo = "INTERNO"
	CodLimite       Codigo = "LIMITE_PETICIONES"
	CodNoImplem     Codigo = "NO_IMPLEMENTADO"
)

// Error es el error de dominio del mid. Lleva el status HTTP que le
// corresponde, para que el handler no tenga que decidirlo.
type Error struct {
	Codigo  Codigo
	Mensaje string
	Status  int

	// Campos lleva los errores por campo de un formulario. El frontend los
	// pinta al lado del input correspondiente.
	Campos map[string]string

	// causa es el error original. Se registra en el log pero NUNCA se
	// serializa hacia el cliente: puede traer nombres de tabla y constraints.
	causa error
}

// Error implementa la interfaz error, incluyendo la causa si la hay.
func (e *Error) Error() string {
	if e.causa != nil {
		return fmt.Sprintf("%s: %s: %v", e.Codigo, e.Mensaje, e.causa)
	}
	return fmt.Sprintf("%s: %s", e.Codigo, e.Mensaje)
}

// Unwrap expone la causa para errors.Is y errors.As.
func (e *Error) Unwrap() error { return e.causa }

// Causa expone el error original para el logger. No usar en la respuesta.
func (e *Error) Causa() error { return e.causa }

// Con adjunta la causa original sin exponerla al cliente.
func (e *Error) Con(causa error) *Error {
	e.causa = causa
	return e
}

// ConCampo agrega un error de validacion asociado a un campo del formulario.
func (e *Error) ConCampo(campo, mensaje string) *Error {
	if e.Campos == nil {
		e.Campos = map[string]string{}
	}
	e.Campos[campo] = mensaje
	return e
}

// Validacion crea un error 400 por datos de entrada invalidos.
func Validacion(mensaje string) *Error {
	return &Error{Codigo: CodValidacion, Mensaje: mensaje, Status: http.StatusBadRequest}
}

// NoAutorizado crea un error 401 (falta sesion o no es valida).
func NoAutorizado(mensaje string) *Error {
	if mensaje == "" {
		mensaje = "Necesitas iniciar sesion."
	}
	return &Error{Codigo: CodNoAutorizado, Mensaje: mensaje, Status: http.StatusUnauthorized}
}

// Prohibido crea un error 403 (hay sesion pero no permiso).
func Prohibido(mensaje string) *Error {
	if mensaje == "" {
		mensaje = "No tienes permiso para hacer esto."
	}
	return &Error{Codigo: CodProhibido, Mensaje: mensaje, Status: http.StatusForbidden}
}

// NoEncontrado crea un error 404; recurso completa la frase "No encontramos ...".
func NoEncontrado(recurso string) *Error {
	return &Error{
		Codigo:  CodNoEncontrado,
		Mensaje: fmt.Sprintf("No encontramos %s.", recurso),
		Status:  http.StatusNotFound,
	}
}

// Conflicto crea un error 409 (la operacion choca con el estado actual).
func Conflicto(mensaje string) *Error {
	return &Error{Codigo: CodConflicto, Mensaje: mensaje, Status: http.StatusConflict}
}

// Upstream representa un fallo de un servicio de Jobsy_API. El mensaje que ve
// el cliente es generico a proposito: el detalle va al log.
func Upstream(servicio string, causa error) *Error {
	return &Error{
		Codigo:  CodUpstream,
		Mensaje: "Un servicio no esta disponible en este momento. Intentalo de nuevo.",
		Status:  http.StatusBadGateway,
		causa:   fmt.Errorf("upstream %s: %w", servicio, causa),
	}
}

// Interno crea un error 500 con un mensaje generico; la causa solo va al log.
func Interno(causa error) *Error {
	return &Error{
		Codigo:  CodInterno,
		Mensaje: "Algo salio mal de nuestro lado.",
		Status:  http.StatusInternalServerError,
		causa:   causa,
	}
}

// NoImplementado es para funcionalidad que el sistema aun no soporta, a
// diferencia de un fallo. El frontend puede distinguirla y avisar en vez de
// mostrar un error generico.
func NoImplementado(mensaje string) *Error {
	if mensaje == "" {
		mensaje = "Esta funcion todavia no esta disponible."
	}
	return &Error{Codigo: CodNoImplem, Mensaje: mensaje, Status: http.StatusNotImplemented}
}

// Limite crea un error 429 por exceso de peticiones.
func Limite() *Error {
	return &Error{
		Codigo:  CodLimite,
		Mensaje: "Demasiadas peticiones. Espera un momento.",
		Status:  http.StatusTooManyRequests,
	}
}

// Desde normaliza cualquier error a un *Error. Un error que no reconocemos se
// convierte en interno: preferimos un 500 generico antes que filtrar detalles.
func Desde(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Interno(err)
}
