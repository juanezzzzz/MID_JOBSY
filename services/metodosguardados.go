package services

import (
	"context"
	"strconv"
	"strings"

	"api_MID/models"
)

// MetodosGuardados administra los metodos de pago que el usuario deja
// guardados (tarjeta, cuenta...). El numero se almacena enmascarado.
type MetodosGuardados struct {
	tienda   *ClienteUpstream
	catalogo *Catalogo
}

// NuevoMetodosGuardados crea el servicio de metodos guardados sobre el upstream
// de tienda y el catalogo.
func NuevoMetodosGuardados(t *ClienteUpstream, cat *Catalogo) *MetodosGuardados {
	return &MetodosGuardados{tienda: t, catalogo: cat}
}

// tipoCuentaPorDefecto resuelve el tipo de cuenta con el que se guardan los
// metodos nuevos.
func (s *MetodosGuardados) tipoCuentaPorDefecto(ctx context.Context) (int, error) {
	return s.catalogo.IDTipo(ctx, "cuenta_bancaria", "ahorros")
}

type filaMetodoGuardado struct {
	Id           int         `json:"Id"`
	IdUsuarios   int         `json:"IdUsuarios"`
	IdMetodoPago *refAnidada `json:"IdMetodoPago"`
	Alias        string      `json:"Alias"`
	NumeroCuenta string      `json:"NumeroCuenta"`
	// IdTipoCuenta es NOT NULL con clave foranea a catalogo.tipos. Hay que
	// leerlo y reenviarlo tal cual: mandar cero viola la foranea y el PUT
	// falla, cosa que ademas no se veia porque el backend responde 200.
	IdTipoCuenta int  `json:"IdTipoCuenta"`
	EsPrincipal  bool `json:"EsPrincipal"`
	Activo       bool `json:"Activo"`
}

// Mios lista los metodos guardados, siempre enmascarados.
func (s *MetodosGuardados) Mios(ctx context.Context, idUsuario string) ([]models.MetodoGuardado, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var filas []filaMetodoGuardado
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "50")
	if err := s.tienda.Get(ctx, "metodos_pago_guardados", o, &filas); err != nil {
		return nil, err
	}

	out := make([]models.MetodoGuardado, 0, len(filas))
	for _, f := range filas {
		idMetodo := ""
		if f.IdMetodoPago != nil {
			idMetodo = strconv.Itoa(f.IdMetodoPago.Id)
		}
		out = append(out, models.MetodoGuardado{
			ID:          strconv.Itoa(f.Id),
			MetodoID:    idMetodo,
			Alias:       f.Alias,
			Enmascarado: enmascarar(f.NumeroCuenta),
			EsPrincipal: f.EsPrincipal,
		})
	}
	return out, nil
}

// enmascarar deja solo los ultimos cuatro digitos.
//
// El numero completo no sale nunca del servidor: no hay razon para que el
// navegador lo tenga y si para que no lo tenga.
func enmascarar(numero string) string {
	numero = strings.TrimSpace(numero)
	if len(numero) <= 4 {
		return strings.Repeat("*", len(numero))
	}
	return "****" + numero[len(numero)-4:]
}

// Guardar anade un metodo de pago del usuario.
func (s *MetodosGuardados) Guardar(ctx context.Context, idUsuario, idMetodoPago, alias, numero string, principal bool) (*models.MetodoGuardado, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	metodo, err := IDNumerico(idMetodoPago)
	if err != nil {
		return nil, models.Validacion("Metodo de pago invalido.").ConCampo("metodoId", "Invalido.")
	}
	if err := ValidarTexto("alias", "El alias", alias, 2, 40); err != nil {
		return nil, err
	}
	// Cuentas, tarjetas, Nequi y Daviplata son siempre numeros; los espacios
	// y guiones con que se suelen escribir se quitan.
	numero = strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(numero))
	if !reNumeroCuenta.MatchString(numero) {
		return nil, models.Validacion("El numero de cuenta debe tener entre 6 y 20 digitos.").
			ConCampo("numeroCuenta", "Solo digitos, de 6 a 20.")
	}

	idTipoCuenta, err := s.tipoCuentaPorDefecto(ctx)
	if err != nil {
		return nil, err
	}

	// Solo puede haber un principal por usuario, y la base lo impone con un
	// indice unico: hay que retirar el anterior antes de insertar.
	if principal {
		if err := s.quitarPrincipales(ctx, id); err != nil {
			return nil, err
		}
	}

	var creado filaMetodoGuardado
	if err := s.tienda.Post(ctx, "metodos_pago_guardados", map[string]any{
		"IdUsuarios":   id,
		"IdMetodoPago": map[string]any{"Id": metodo},
		"Alias":        strings.TrimSpace(alias),
		"NumeroCuenta": numero,
		"IdTipoCuenta": idTipoCuenta,
		"EsPrincipal":  principal,
		"Activo":       true,
	}, &creado); err != nil {
		return nil, err
	}

	return &models.MetodoGuardado{
		ID:          strconv.Itoa(creado.Id),
		MetodoID:    idMetodoPago,
		Alias:       alias,
		Enmascarado: enmascarar(numero),
		EsPrincipal: principal,
	}, nil
}

// quitarPrincipales desmarca cualquier metodo principal previo del usuario,
// para que al guardar uno nuevo como principal no queden dos.
func (s *MetodosGuardados) quitarPrincipales(ctx context.Context, idUsuario int) error {
	var filas []filaMetodoGuardado
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(idUsuario),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "50")
	if err := s.tienda.Get(ctx, "metodos_pago_guardados", o, &filas); err != nil {
		return err
	}
	for _, f := range filas {
		if !f.EsPrincipal {
			continue
		}
		if err := s.tienda.Fijar(ctx, "metodos_pago_guardados/"+strconv.Itoa(f.Id), map[string]any{"EsPrincipal": false}); err != nil {
			return err
		}
	}
	return nil
}

// Borrar retira un metodo guardado del usuario.
func (s *MetodosGuardados) Borrar(ctx context.Context, idUsuario, idMetodo string) error {
	usuario, err := IDNumerico(idUsuario)
	if err != nil {
		return err
	}
	id, err := IDNumerico(idMetodo)
	if err != nil {
		return err
	}

	var filas []filaMetodoGuardado
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.tienda.Get(ctx, "metodos_pago_guardados", o, &filas); err != nil {
		return err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return models.NoEncontrado("ese metodo de pago")
	}
	f := filas[0]
	if f.IdUsuarios != usuario {
		return models.Prohibido("")
	}

	return s.tienda.Fijar(ctx, "metodos_pago_guardados/"+strconv.Itoa(id), map[string]any{
		"EsPrincipal": false,
		"Activo":      false,
	})
}
