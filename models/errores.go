
package models

import (
	"errors"
	"fmt"
	"net/http"
)

// Codigo es un identificador estable que el frontend puede usar para el switch.
// A diferencia del mensaje, no cambia aunque reescribamos el texto.

type Codigo string

const (
	CodValidacion           Codigo = "VALIDACION"
	CodNoAutorizado         Codigo = "NO_AUTORIZADO"
	CodProhibido            Codigo = "PROHIBIDO"
	CodNoEncontrado         Codigo = "NO_ENCONTRADO"
	CodConflicto            Codigo = "CONFLICTO"
	CodUpstream             Codigo = "UPSTREAM"
	CodInterno              Codigo = "INTERNO"
	CodLimite               Codigo = "LIMITE_PETICIONES"
	CodNoImplem             Codigo = "NO_IMPLEMENTADO"
)

// Error es el error de dominio del mid. Lleva el status HTTP que le
// Corresponde, Para que el handler no tenga que decidirlo.

type Error struct {
	Codigo Codigo
	Mensaje string
	Status int

	// Campos lleva los errores por campo de un formulario. El frontend los
	// pinta al lado del input correspondiente.
	Campos map[string]string

	// causa es el error original. Se registra en el log pero NUNCA se
	// serializa hacia el cliente: puede traer nombres de tabla y constraints.
	causa error
}

// Error implementa la interfaz error, Incluyendo la causa si la hay.

func (e *Error) Error() string {
	if e.causa != nil {
		return fmt.Sprintf("%s: %s: %v", e.Codigo, e.Mensaje, e.causa)
	}
	return fmt.Sprintf("%s: %s", e.Codigo, e.Mensaje)
}

// Unwrap expone la causa para errors.Is y errors.As.
func (e *Error) Unwrap() error {
	return e.causa
}

// Causa expone el error original para el logger. No usar en la respuesta.
func (e *Error) Causa() error {
	return e.causa
}

// Con adjunta la causa original sin exponerla al cliente.
func (e *Error) Con(causa error) *Error {
	e.causa = causa
	return e
}

// Con Campo agrega un error de validacion asociado a un campo del formulario.
func (e *Error) ConCampo(campo, mensaje string) *Error {
	if e.Campos == nil {
		e.Campos = map[string]string{}
	}
	e.Campos[campo] = mensaje
	return e
}