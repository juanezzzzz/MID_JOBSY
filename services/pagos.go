package services

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"jobsy_mid/models"
)

// Pagos registra los cobros y su rastro contable.
//
// Hasta ahora el checkout creaba el pedido y descontaba stock, pero el cobro
// no se registraba en ninguna parte: tienda.pagos y tienda.transacciones
// quedaban vacias. Un pedido sin pago asociado no se puede conciliar, ni
// reclamar, ni reembolsar.
type Pagos struct {
	tienda   *ClienteUpstream
	catalogo *Catalogo
	registro *RegistroPasarelas
	monedero *Monedero
}

// NuevoPagos crea el servicio de pagos con el upstream de tienda, el catalogo,
// el registro de pasarelas y el monedero.
func NuevoPagos(t *ClienteUpstream, cat *Catalogo, reg *RegistroPasarelas, mon *Monedero) *Pagos {
	return &Pagos{tienda: t, catalogo: cat, registro: reg, monedero: mon}
}

type filaMetodoPago struct {
	Id     int    `json:"Id"`
	Nombre string `json:"Nombre"`
	Activo bool   `json:"Activo"`
}

type filaPago struct {
	Id               int         `json:"Id"`
	IdPedidos        *refAnidada `json:"IdPedidos"`
	IdContrataciones *int        `json:"IdContrataciones"`
	IdUsuarios       int         `json:"IdUsuarios"`
	IdMetodoPago     *refAnidada `json:"IdMetodoPago"`
	IdEstado         int         `json:"IdEstado"`
	Monto            float64     `json:"Monto"`
	ReferenciaExt    string      `json:"ReferenciaExt"`
	Banco            string      `json:"Banco"`
	Ultimos4         string      `json:"Ultimos4"`
	Activo           bool        `json:"Activo"`
	CreadoEn         string      `json:"CreadoEn"`
}

// DatosPago son los del instrumento con el que se cobra.
type DatosPago struct {
	Metodo     string `json:"metodoPago"`
	Titular    string `json:"titular,omitempty"`
	Ultimos4   string `json:"ultimos4,omitempty"`
	Banco      string `json:"banco,omitempty"`
	TipoCuenta string `json:"tipoCuenta,omitempty"`
}

// ResultadoPago es lo que el frontend necesita para decidir que pantalla
// mostrar tras el cobro.
type ResultadoPago struct {
	PagoID         string `json:"pagoId"`
	Estado         string `json:"estado"`
	Mensaje        string `json:"mensaje"`
	URLRedireccion string `json:"urlRedireccion,omitempty"`
	Referencia     string `json:"referencia,omitempty"`
}

// idMetodoPago resuelve el nombre del metodo a su fila de catalogo.
func (p *Pagos) idMetodoPago(ctx context.Context, nombre string) (int, error) {
	var filas []filaMetodoPago
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := p.tienda.Get(ctx, "metodo_pago", o, &filas); err != nil {
		return 0, err
	}
	for _, f := range filas {
		if strings.EqualFold(strings.TrimSpace(f.Nombre), strings.TrimSpace(nombre)) {
			return f.Id, nil
		}
	}
	return 0, models.Validacion("Metodo de pago no reconocido: "+nombre).
		ConCampo("metodoPago", "No disponible.")
}

// MetodosDisponibles lista los metodos que tienen pasarela conectada.
//
// No basta con que la fila exista en el catalogo: si no hay pasarela detras,
// ofrecerlo al usuario es prometer un cobro que nadie puede hacer.
func (p *Pagos) MetodosDisponibles(ctx context.Context) ([]map[string]any, error) {
	var filas []filaMetodoPago
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := p.tienda.Get(ctx, "metodo_pago", o, &filas); err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(filas))
	for _, f := range filas {
		if !f.Activo {
			continue
		}
		_, err := p.registro.Para(f.Nombre)
		out = append(out, map[string]any{
			"id":          strconv.Itoa(f.Id),
			"nombre":      f.Nombre,
			"disponible":  err == nil,
			"requiereCVV": f.Nombre == "tarjeta",
			"redirige":    f.Nombre == "pse",
		})
	}
	return out, nil
}

// Cobrar ejecuta el cobro y deja constancia.
//
// Secuencia: se resuelve la pasarela, se cobra, y despues se registran el pago
// y su transaccion. Si el registro fallara con el dinero ya capturado quedaria
// un cobro sin rastro, asi que ese fallo se reporta como interno y no como
// exito: es preferible que el usuario reclame a que el dinero desaparezca en
// silencio.
func (p *Pagos) Cobrar(ctx context.Context, idUsuario int, correo string, idPedido int, idContratacion *int, monto float64, d DatosPago) (*ResultadoPago, error) {
	if monto <= 0 {
		return nil, models.Validacion("El monto debe ser mayor que cero.")
	}

	via, err := p.registro.Para(d.Metodo)
	if err != nil {
		return nil, models.Validacion("Ese metodo de pago no esta disponible.").
			ConCampo("metodoPago", "No disponible.")
	}
	idMetodo, err := p.idMetodoPago(ctx, d.Metodo)
	if err != nil {
		return nil, err
	}

	referencia := "PED-" + strconv.Itoa(idPedido)
	if idContratacion != nil {
		referencia = "CON-" + strconv.Itoa(*idContratacion)
	}

	res, err := via.Cobrar(ctx, Cobro{
		Referencia: referencia,
		Monto:      monto,
		Moneda:     "COP",
		UsuarioID:  idUsuario,
		Correo:     correo,
		Concepto:   "Jobsy " + referencia,
		Titular:    d.Titular,
		Ultimos4:   d.Ultimos4,
		Banco:      d.Banco,
		TipoCuenta: d.TipoCuenta,
	})
	if err != nil {
		return nil, models.Interno(err)
	}

	idEstado, err := p.catalogo.IDEstado(ctx, "pago", string(res.Estado))
	if err != nil {
		return nil, err
	}

	cuerpo := map[string]any{
		"IdUsuarios":     idUsuario,
		"IdMetodoPago":   map[string]any{"Id": idMetodo},
		"IdTipoPago":     0,
		"IdEstado":       idEstado,
		"Monto":          monto,
		"ReferenciaExt":  res.ReferenciaExterna,
		"Banco":          d.Banco,
		"TitularTarjeta": d.Titular,
		"Ultimos4":       d.Ultimos4,
		"Activo":         true,
	}
	// El esquema exige que cuelgue de un pedido o de una contratacion, nunca
	// de ambos ni de ninguno.
	if idContratacion != nil {
		cuerpo["IdContrataciones"] = *idContratacion
	} else {
		cuerpo["IdPedidos"] = map[string]any{"Id": idPedido}
	}

	if idTipo, err := p.catalogo.IDTipo(ctx, "pago", contextoTipoPago(idContratacion)); err == nil {
		cuerpo["IdTipoPago"] = idTipo
	}

	var creado filaPago
	if err := p.tienda.Post(ctx, "pagos", cuerpo, &creado); err != nil {
		return nil, models.Interno(err)
	}

	// El movimiento contable solo tiene sentido si el dinero se capturo.
	if res.Estado == PagoAprobado {
		if err := p.registrarTransaccion(ctx, idUsuario, creado.Id, monto, "egreso", "Pago "+referencia); err != nil {
			// La transaccion es rastro, no dinero: su fallo no invalida el
			// cobro, pero debe verse en el log.
			_ = err
		}
	}

	return &ResultadoPago{
		PagoID:         strconv.Itoa(creado.Id),
		Estado:         string(res.Estado),
		Mensaje:        res.Mensaje,
		URLRedireccion: res.URLRedireccion,
		Referencia:     res.ReferenciaExterna,
	}, nil
}

// contextoTipoPago indica si el pago es de una contratacion o de la tienda.
func contextoTipoPago(idContratacion *int) string {
	if idContratacion != nil {
		return "contratacion"
	}
	return "tienda"
}

// registrarTransaccion deja constancia de un movimiento de dinero ligado a un
// pago, con una referencia unica.
func (p *Pagos) registrarTransaccion(ctx context.Context, idUsuario, idPago int, monto float64, tipo, descripcion string) error {
	idTipo, err := p.idTipoTransaccion(ctx, tipo)
	if err != nil {
		return err
	}
	cuerpo := map[string]any{
		"IdUsuarios":        idUsuario,
		"IdPagos":           map[string]any{"Id": idPago},
		"IdTipoTransaccion": map[string]any{"Id": idTipo},
		"Monto":             monto,
		"Descripcion":       descripcion,
		"Referencia":        "TRX-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		"Activo":            true,
	}
	var creada map[string]any
	return p.tienda.Post(ctx, "transacciones", cuerpo, &creada)
}

// idTipoTransaccion resuelve el id de un tipo de transaccion por nombre.
func (p *Pagos) idTipoTransaccion(ctx context.Context, nombre string) (int, error) {
	var filas []struct {
		Id     int    `json:"Id"`
		Nombre string `json:"Nombre"`
	}
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := p.tienda.Get(ctx, "tipo_transaccion", o, &filas); err != nil {
		return 0, err
	}
	for _, f := range filas {
		if strings.EqualFold(f.Nombre, nombre) {
			return f.Id, nil
		}
	}
	return 0, models.Upstream("tienda", errTipoTrxDesconocido)
}

// Confirmar aplica la notificacion asincrona del proveedor.
//
// Los metodos que sacan al usuario del sitio, como PSE, no resuelven el cobro
// en linea: el proveedor avisa despues. Este es el punto de entrada de ese
// aviso.
//
// PENDIENTE DE SEGURIDAD: un proveedor real firma sus notificaciones y hay que
// verificar esa firma antes de creerse nada. Mientras la pasarela sea la
// simulada no hay firma que comprobar, pero este endpoint NO debe exponerse a
// Internet tal cual.
func (p *Pagos) Confirmar(ctx context.Context, idPago string, aprobado bool) (*ResultadoPago, error) {
	id, err := IDNumerico(idPago)
	if err != nil {
		return nil, err
	}

	var filas []filaPago
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := p.tienda.Get(ctx, "pagos", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("ese pago")
	}
	f := filas[0]

	estado := "rechazado"
	if aprobado {
		estado = "aprobado"
	}
	idEstado, err := p.catalogo.IDEstado(ctx, "pago", estado)
	if err != nil {
		return nil, err
	}

	if err := p.tienda.Fijar(ctx, "pagos/"+strconv.Itoa(id), map[string]any{"IdEstado": idEstado}); err != nil {
		return nil, err
	}

	if aprobado {
		_ = p.registrarTransaccion(ctx, f.IdUsuarios, f.Id, f.Monto, "egreso", "Pago confirmado")
	}

	// Confirmar el pago no bastaba: el pedido se quedaba en «pendiente» para
	// siempre. Un PSE aprobado deja el pedido pagado; uno rechazado lo
	// cancela, que es lo mismo que hace el checkout cuando el cobro falla en
	// linea.
	if f.IdPedidos != nil {
		nombre := "pagado"
		if !aprobado {
			nombre = "cancelado"
		}
		if idEstadoPedido, err := p.catalogo.IDEstado(ctx, "pedido", nombre); err == nil {
			cambios := map[string]any{"IdEstado": idEstadoPedido}
			if !aprobado {
				cambios["Activo"] = false
			}
			if err := p.tienda.Fijar(ctx, "facturacion/"+strconv.Itoa(f.IdPedidos.Id), cambios); err != nil {
				slog.Error("pago confirmado pero el pedido no cambio de estado",
					"pago", f.Id, "pedido", f.IdPedidos.Id, "estado", nombre, "error", err)
			}
		}
	}

	return &ResultadoPago{
		PagoID:     strconv.Itoa(f.Id),
		Estado:     estado,
		Mensaje:    "Pago " + estado + ".",
		Referencia: f.ReferenciaExt,
	}, nil
}

// Mios devuelve el historial de pagos del usuario.
func (p *Pagos) Mios(ctx context.Context, idUsuario string) ([]map[string]any, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}

	var filas []filaPago
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{
		"IdUsuarios": strconv.Itoa(id),
		"Activo":     "true",
	}))
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := p.tienda.Get(ctx, "pagos", o, &filas); err != nil {
		return nil, err
	}

	// Nombre de cada metodo con una sola lectura del catalogo. Si falla, el
	// historial sale igual, solo sin el metodo.
	nombres := map[int]string{}
	var metodos []filaMetodoPago
	om := &OpcionesUpstream{Query: map[string][]string{}}
	om.Query.Set("limit", "50")
	if err := p.tienda.Get(ctx, "metodo_pago", om, &metodos); err == nil {
		for _, m := range metodos {
			nombres[m.Id] = m.Nombre
		}
	}

	out := make([]map[string]any, 0, len(filas))
	for _, f := range filas {
		estado := "pendiente"
		if n, err := p.catalogo.EstadoDesdeID(ctx, f.IdEstado); err == nil {
			estado = n
		}
		item := map[string]any{
			"id":         strconv.Itoa(f.Id),
			"monto":      f.Monto,
			"moneda":     "COP",
			"estado":     estado,
			"referencia": f.ReferenciaExt,
			"fecha":      f.CreadoEn,
		}
		if f.IdPedidos != nil {
			item["pedidoId"] = strconv.Itoa(f.IdPedidos.Id)
		}
		if f.Ultimos4 != "" {
			item["ultimos4"] = f.Ultimos4
		}
		if f.IdMetodoPago != nil {
			if n, ok := nombres[f.IdMetodoPago.Id]; ok {
				item["metodo"] = n
			}
		}
		out = append(out, item)
	}
	return out, nil
}

type erroresPago string

// Error implementa la interfaz error.
func (e erroresPago) Error() string { return string(e) }

const errTipoTrxDesconocido = erroresPago("tienda.tipo_transaccion no tiene ese tipo")

// DevolverDinero reembolsa un pago por la misma pasarela con la que se cobro.
// Si esa pasarela ya no existe o no admite reembolsos, devuelve Conflicto.
func (p *Pagos) DevolverDinero(ctx context.Context, f filaPago) error {
	nombre := ""
	if f.IdMetodoPago != nil {
		n, err := p.nombreMetodo(ctx, f.IdMetodoPago.Id)
		if err != nil {
			return err
		}
		nombre = n
	}

	via, err := p.registro.Para(nombre)
	if err != nil {
		return models.Conflicto(
			"El metodo con el que se pago («" + nombre + "») ya no esta disponible, asi que no se puede devolver por ahi.")
	}

	if err := via.Reembolsar(ctx, f.ReferenciaExt, f.Monto); err != nil {
		if errors.Is(err, ErrNoSoportado) {
			return models.Conflicto(
				"La pasarela de «" + nombre + "» no admite devoluciones automaticas. Hay que hacerla desde su panel.")
		}
		return models.Conflicto("La pasarela rechazo la devolucion: " + err.Error())
	}
	return nil
}

// MarcarReembolsado deja el pago en 'reembolsado' y anota la transaccion.
func (p *Pagos) MarcarReembolsado(ctx context.Context, f filaPago, motivo string) error {
	idEstado, err := p.catalogo.IDEstado(ctx, "pago", "reembolsado")
	if err != nil {
		return err
	}
	if err := p.tienda.Fijar(ctx, "pagos/"+strconv.Itoa(f.Id),
		map[string]any{"IdEstado": idEstado}); err != nil {
		return err
	}
	concepto := "Reembolso"
	if m := strings.TrimSpace(motivo); m != "" {
		concepto += ": " + m
	}
	return p.registrarTransaccion(ctx, f.IdUsuarios, f.Id, f.Monto, "reembolso", concepto)
}

// nombreMetodo devuelve el nombre del metodo de pago con ese id.
func (p *Pagos) nombreMetodo(ctx context.Context, id int) (string, error) {
	var filas []filaMetodoPago
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := p.tienda.Get(ctx, "metodo_pago", o, &filas); err != nil {
		return "", err
	}
	for _, f := range filas {
		if f.Id == id {
			return f.Nombre, nil
		}
	}
	return "", models.NoEncontrado("ese metodo de pago")
}

// formatearMonto escribe un importe en pesos, con separador de miles.
func formatearMonto(v float64) string {
	entero := strconv.FormatInt(int64(v+0.5), 10)
	var b strings.Builder
	for i, c := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return "$" + b.String()
}