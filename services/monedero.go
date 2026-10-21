package services

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"api_MID/models"
)

// Monedero gestiona el saldo interno de Jobsy.
//
// La tabla tienda.saldo_jobsy existe y su endpoint permite hoy fijar cualquier
// saldo con un PUT sin autenticacion. Aqui el saldo solo se mueve por abono o
// por cargo, cada movimiento deja rastro en tienda.transacciones, y nunca
// puede quedar negativo.
//
// LIMITACION IMPORTANTE: el backend no ofrece transacciones de base de datos,
// asi que leer-el-saldo y escribirlo son dos llamadas separadas. Entre ambas
// cabe otra operacion del mismo usuario. El bloqueo por usuario de abajo cierra
// esa ventana mientras haya una sola instancia del middleware; con varias
// instancias hara falta un bloqueo compartido o, mejor, mover el descuento a
// una sentencia atomica en la base.
type Monedero struct {
	tienda *ClienteUpstream

	// bloqueos serializa las operaciones sobre el mismo monedero.
	mu       sync.Mutex
	bloqueos map[int]*sync.Mutex
}

// NuevoMonedero crea el monedero (saldo Jobsy) sobre el upstream de tienda.
func NuevoMonedero(t *ClienteUpstream) *Monedero {
	return &Monedero{tienda: t, bloqueos: map[int]*sync.Mutex{}}
}

// bloqueoDe devuelve el mutex del usuario, creandolo si no existe, para que dos
// movimientos del mismo saldo no se pisen.
func (m *Monedero) bloqueoDe(idUsuario int) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.bloqueos[idUsuario]; ok {
		return l
	}
	l := &sync.Mutex{}
	m.bloqueos[idUsuario] = l
	return l
}

type filaSaldo struct {
	Id              int     `json:"Id"`
	IdUsuarios      int     `json:"IdUsuarios"`
	SaldoDisponible float64 `json:"SaldoDisponible"`
	TotalGanado     float64 `json:"TotalGanado"`
	TotalRetirado   float64 `json:"TotalRetirado"`
	Activo          bool    `json:"Activo"`
}

// Saldo describe el monedero tal como lo ve el frontend.
type Saldo struct {
	Disponible    float64 `json:"disponible"`
	TotalGanado   float64 `json:"totalGanado"`
	TotalRetirado float64 `json:"totalRetirado"`
	Moneda        string  `json:"moneda"`
}

// De devuelve el saldo del usuario, creando el monedero si aun no existe.
func (m *Monedero) De(ctx context.Context, idUsuario string) (*Saldo, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	f, err := m.obtenerOCrear(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Saldo{
		Disponible:    f.SaldoDisponible,
		TotalGanado:   f.TotalGanado,
		TotalRetirado: f.TotalRetirado,
		Moneda:        "COP",
	}, nil
}

// obtenerOCrear devuelve el saldo activo del usuario y, si no tiene, le crea
// uno en cero.
func (m *Monedero) obtenerOCrear(ctx context.Context, idUsuario int) (*filaSaldo, error) {
	var filas []filaSaldo
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "10")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := m.tienda.Get(ctx, "saldo_jobsy", o, &filas); err != nil {
		return nil, err
	}
	for i := range filas {
		if filas[i].Activo {
			return &filas[i], nil
		}
	}

	cuerpo := map[string]any{
		"IdUsuarios":      idUsuario,
		"SaldoDisponible": 0,
		"TotalGanado":     0,
		"TotalRetirado":   0,
		"Activo":          true,
	}
	var creado filaSaldo
	if err := m.tienda.Post(ctx, "saldo_jobsy", cuerpo, &creado); err != nil {
		return nil, err
	}
	creado.IdUsuarios = idUsuario
	return &creado, nil
}

// escribir guarda los importes del monedero.
func (m *Monedero) escribir(ctx context.Context, f *filaSaldo) error {
	return m.tienda.Fijar(ctx, "saldo_jobsy/"+strconv.Itoa(f.Id), map[string]any{
		"SaldoDisponible": f.SaldoDisponible,
		"TotalGanado":     f.TotalGanado,
		"TotalRetirado":   f.TotalRetirado,
	})
}

// Abonar suma dinero al monedero.
func (m *Monedero) Abonar(ctx context.Context, idUsuario int, monto float64, concepto string) error {
	if monto <= 0 {
		return models.Validacion("El monto a abonar debe ser mayor que cero.")
	}

	l := m.bloqueoDe(idUsuario)
	l.Lock()
	defer l.Unlock()

	f, err := m.obtenerOCrear(ctx, idUsuario)
	if err != nil {
		return err
	}
	f.SaldoDisponible += monto
	f.TotalGanado += monto
	return m.escribir(ctx, f)
}

// Cargar descuenta del monedero. Falla si no alcanza.
func (m *Monedero) Cargar(ctx context.Context, idUsuario int, monto float64, concepto string) error {
	if monto <= 0 {
		return models.Validacion("El monto a cobrar debe ser mayor que cero.")
	}

	l := m.bloqueoDe(idUsuario)
	l.Lock()
	defer l.Unlock()

	f, err := m.obtenerOCrear(ctx, idUsuario)
	if err != nil {
		return err
	}
	if f.SaldoDisponible < monto {
		return models.Conflicto(fmt.Sprintf(
			"Saldo insuficiente: tienes %.0f COP y el cobro es de %.0f COP.", f.SaldoDisponible, monto))
	}
	f.SaldoDisponible -= monto
	f.TotalRetirado += monto
	return m.escribir(ctx, f)
}

// ---- Pasarela contra el monedero ----

// PasarelaSaldo cobra contra el saldo interno. A diferencia de la simulada,
// esta mueve dinero de verdad: el saldo vive en nuestra base.
type PasarelaSaldo struct {
	monedero *Monedero
}

// NuevaPasarelaSaldo crea la pasarela que cobra contra el saldo Jobsy.
func NuevaPasarelaSaldo(m *Monedero) *PasarelaSaldo {
	return &PasarelaSaldo{monedero: m}
}

// Nombre identifica la pasarela en el registro.
func (p *PasarelaSaldo) Nombre() string { return "saldo_jobsy" }

// Cobrar descuenta el monto del saldo del usuario. El saldo insuficiente se
// devuelve como cobro rechazado, no como error.
func (p *PasarelaSaldo) Cobrar(ctx context.Context, c Cobro) (*ResultadoCobro, error) {
	err := p.monedero.Cargar(ctx, c.UsuarioID, c.Monto, c.Concepto)
	if err != nil {
		// El saldo insuficiente no es un fallo del sistema sino un rechazo del
		// cobro: se devuelve como resultado para que quien llama lo trate
		// igual que el rechazo de un banco.
		var e *models.Error
		if ok := comoErrorApi(err, &e); ok && e.Codigo == models.CodConflicto {
			return &ResultadoCobro{Estado: PagoRechazado, Mensaje: e.Mensaje}, nil
		}
		return nil, err
	}
	return &ResultadoCobro{
		Estado:            PagoAprobado,
		ReferenciaExterna: fmt.Sprintf("SALDO-%d-%.0f", c.UsuarioID, c.Monto),
		Mensaje:           "Pago descontado de tu saldo Jobsy.",
	}, nil
}

// Reembolsar devuelve el dinero al monedero.
func (p *PasarelaSaldo) Reembolsar(ctx context.Context, referenciaExterna string, monto float64) error {
	var idUsuario int
	var m float64
	if _, err := fmt.Sscanf(referenciaExterna, "SALDO-%d-%f", &idUsuario, &m); err != nil {
		return fmt.Errorf("referencia de saldo ilegible: %q", referenciaExterna)
	}
	return p.monedero.Abonar(ctx, idUsuario, monto, "Reembolso")
}

// comoErrorApi evita importar errors solo para un As.
func comoErrorApi(err error, destino **models.Error) bool {
	e := models.Desde(err)
	if e == nil {
		return false
	}
	*destino = e
	return true
}
