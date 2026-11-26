package services

import (
	"context"
	"strconv"
	"strings"
)

// Boletin guarda los correos suscritos al boletin del blog en Usuarios_Api
// (usuarios.suscriptores_boletin).
type Boletin struct {
	usuarios *ClienteUpstream
}

// NuevoBoletin crea el servicio sobre Usuarios_Api.
func NuevoBoletin(u *ClienteUpstream) *Boletin { return &Boletin{usuarios: u} }

// Suscribir agrega el correo. Repetirlo no es error ni dice si ya estaba,
// para que el formulario no sirva para averiguar quien esta suscrito.
func (s *Boletin) Suscribir(ctx context.Context, correo string) error {
	correo = strings.ToLower(strings.TrimSpace(correo))
	if err := ValidarEmail(correo); err != nil {
		return err
	}
	var filas []struct {
		Id     int  `json:"Id"`
		Activo bool `json:"Activo"`
	}
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Correo": correo}))
	o.Query.Set("limit", "1")
	if err := s.usuarios.Get(ctx, "suscriptores_boletin", o, &filas); err != nil {
		return err
	}
	if len(filas) > 0 {
		if !filas[0].Activo {
			return s.usuarios.Fijar(ctx, "suscriptores_boletin/"+strconv.Itoa(filas[0].Id), map[string]any{"Activo": true})
		}
		return nil
	}
	return s.usuarios.Post(ctx, "suscriptores_boletin", map[string]any{"Correo": correo, "Activo": true}, nil)
}
