package services

import (
	"context"
	"strconv"

	"api_MID/models"
)

// ---- Resenas ----

// Configuraciones gestiona las preferencias del usuario (privacidad y
// notificaciones). Las guarda en el servicio de usuarios.
type Configuraciones struct {
	usuarios *ClienteUpstream
}

// NuevoConfiguraciones crea el servicio de configuraciones sobre el upstream de
// usuarios.
func NuevoConfiguraciones(u *ClienteUpstream) *Configuraciones {
	return &Configuraciones{usuarios: u}
}

type filaConfiguracion struct {
	Id                   int  `json:"Id"`
	IdUsuarios           int  `json:"IdUsuarios"`
	PerfilPublico        bool `json:"PerfilPublico"`
	MostrarCorreo        bool `json:"MostrarCorreo"`
	MostrarTelefono      bool `json:"MostrarTelefono"`
	HvVisible            bool `json:"HvVisible"`
	AparecerBusquedas    bool `json:"AparecerBusquedas"`
	MostrarUbicacion     bool `json:"MostrarUbicacion"`
	DobleFactor          bool `json:"DobleFactor"`
	NotifCorreo          bool `json:"NotifCorreo"`
	NotifPlataforma      bool `json:"NotifPlataforma"`
	NotifNuevasOfertas   bool `json:"NotifNuevasOfertas"`
	NotifActualizaciones bool `json:"NotifActualizaciones"`
	NotifMensajes        bool `json:"NotifMensajes"`
	NotifPostulaciones   bool `json:"NotifPostulaciones"`
	Activo               bool `json:"Activo"`
}

// aDominio convierte la fila de la base (nombres y campos del upstream) al
// modelo de configuracion que consume el front.
func (f filaConfiguracion) aDominio() models.Configuracion {
	return models.Configuracion{
		PerfilPublico:        f.PerfilPublico,
		MostrarCorreo:        f.MostrarCorreo,
		MostrarTelefono:      f.MostrarTelefono,
		HojaVidaVisible:      f.HvVisible,
		AparecerEnBusquedas:  f.AparecerBusquedas,
		MostrarUbicacion:     f.MostrarUbicacion,
		DobleFactor:          f.DobleFactor,
		NotifCorreo:          f.NotifCorreo,
		NotifPlataforma:      f.NotifPlataforma,
		NotifNuevasOfertas:   f.NotifNuevasOfertas,
		NotifActualizaciones: f.NotifActualizaciones,
		NotifMensajes:        f.NotifMensajes,
		NotifPostulaciones:   f.NotifPostulaciones,
	}
}

// Mias devuelve la configuracion del usuario, creandola con valores por
// defecto si todavia no existe.
func (s *Configuraciones) Mias(ctx context.Context, idUsuario string) (*models.Configuracion, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	f, err := s.obtenerOCrear(ctx, id)
	if err != nil {
		return nil, err
	}
	c := f.aDominio()
	return &c, nil
}

// obtenerOCrear busca la fila de configuracion del usuario y, si no hay,
// inserta una con los valores por defecto y la devuelve.
func (s *Configuraciones) obtenerOCrear(ctx context.Context, idUsuario int) (*filaConfiguracion, error) {
	var filas []filaConfiguracion
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "10")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := s.usuarios.Get(ctx, "configuraciones", o, &filas); err != nil {
		return nil, err
	}
	for i := range filas {
		if filas[i].Activo {
			return &filas[i], nil
		}
	}

	// Valores iniciales conservadores: se muestra lo minimo y se avisa por la
	// plataforma. Que el usuario abra lo que quiera, no al reves.
	cuerpo := map[string]any{
		"IdUsuarios":           idUsuario,
		"PerfilPublico":        true,
		"MostrarCorreo":        false,
		"MostrarTelefono":      false,
		"HvVisible":            true,
		"AparecerBusquedas":    true,
		"MostrarUbicacion":     false,
		"DobleFactor":          false,
		"NotifCorreo":          false,
		"NotifPlataforma":      true,
		"NotifNuevasOfertas":   true,
		"NotifActualizaciones": true,
		"NotifMensajes":        true,
		"NotifPostulaciones":   true,
		"Activo":               true,
	}
	var creada filaConfiguracion
	if err := s.usuarios.Post(ctx, "configuraciones", cuerpo, &creada); err != nil {
		return nil, err
	}
	creada.IdUsuarios = idUsuario
	return &creada, nil
}

// Actualizar guarda las preferencias.
func (s *Configuraciones) Actualizar(ctx context.Context, idUsuario string, c models.Configuracion) (*models.Configuracion, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	actual, err := s.obtenerOCrear(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.usuarios.Fijar(ctx, "configuraciones/"+strconv.Itoa(actual.Id), map[string]any{
		"PerfilPublico":        c.PerfilPublico,
		"MostrarCorreo":        c.MostrarCorreo,
		"MostrarTelefono":      c.MostrarTelefono,
		"HvVisible":            c.HojaVidaVisible,
		"AparecerBusquedas":    c.AparecerEnBusquedas,
		"MostrarUbicacion":     c.MostrarUbicacion,
		"DobleFactor":          c.DobleFactor,
		"NotifCorreo":          c.NotifCorreo,
		"NotifPlataforma":      c.NotifPlataforma,
		"NotifNuevasOfertas":   c.NotifNuevasOfertas,
		"NotifActualizaciones": c.NotifActualizaciones,
		"NotifMensajes":        c.NotifMensajes,
		"NotifPostulaciones":   c.NotifPostulaciones,
	}); err != nil {
		return nil, err
	}
	return s.Mias(ctx, idUsuario)
}
