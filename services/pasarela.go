// Pasarela abstrae el cobro para que el resto del middleware no sepa
// con que proveedor se cobra.
//
// Jobsy no tiene ninguna pasarela integrada: la tabla tienda.pagos es un sitio
// donde insertar filas y nadie mueve dinero. Aqui se define el contrato que
// cumpliria un proveedor real (Wompi, PayU, Mercado Pago) y se incluyen dos
// implementaciones utilizables hoy:
//
//	PasarelaSimulada  aprueba o rechaza segun reglas deterministas, para desarrollo
//	          y pruebas automaticas.
//	Saldo     cobra contra el monedero interno del usuario. Esta si mueve
//	          dinero de verdad, porque el saldo vive en nuestra base.
//
// Integrar un proveedor real es escribir un tipo que cumpla Pasarela y
// registrarlo en el RegistroPasarelas. Nada del codigo que cobra cambia.

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Estado en que queda un cobro.
type EstadoPago string

const (
	// Pendiente: el proveedor acepto la orden pero aun no confirma. Es el
	// caso normal en PSE, donde el usuario sale a su banco y vuelve.
	PagoPendiente EstadoPago = "pendiente"
	// Aprobado: dinero capturado.
	PagoAprobado EstadoPago = "aprobado"
	// Rechazado: el proveedor lo nego (fondos, antifraude, datos malos).
	PagoRechazado EstadoPago = "rechazado"
)

// Cobro es lo que se le pide a la pasarela.
type Cobro struct {
	// Referencia es nuestro identificador del pedido o la contratacion. Sirve
	// para conciliar cuando el proveedor devuelve una notificacion asincrona.
	Referencia string
	Monto      float64
	Moneda     string
	UsuarioID  int
	Correo     string
	Concepto   string

	// Datos del instrumento. Cada pasarela usa los que necesita; el resto los
	// ignora. Nunca se guarda el numero completo de una tarjeta.
	Titular    string
	Ultimos4   string
	Banco      string
	TipoCuenta string
}

// ResultadoCobro es lo que devuelve la pasarela.
type ResultadoCobro struct {
	Estado EstadoPago
	// ReferenciaExterna es el identificador del proveedor. Es lo que permite
	// reclamar, conciliar o reembolsar despues.
	ReferenciaExterna string
	// URLRedireccion la usan los metodos que sacan al usuario del sitio, como
	// PSE. Vacia cuando el cobro se resuelve en linea.
	URLRedireccion string
	Mensaje        string
}

// Pasarela es el contrato que cumple cualquier proveedor de cobro.
type Pasarela interface {
	// Nombre coincide con tienda.metodo_pago.nombre.
	Nombre() string
	// Cobrar intenta capturar el dinero.
	Cobrar(ctx context.Context, c Cobro) (*ResultadoCobro, error)
	// Reembolsar devuelve el dinero de un cobro ya aprobado.
	Reembolsar(ctx context.Context, referenciaExterna string, monto float64) error
}

// ErrNoSoportado lo devuelve una pasarela que no puede reembolsar.
var ErrNoSoportado = errors.New("la pasarela no soporta esta operacion")

// ErrFondos indica saldo insuficiente. Se distingue de un rechazo generico
// porque al usuario se le puede decir exactamente que hacer.
var ErrFondos = errors.New("fondos insuficientes")

// RegistroPasarelas resuelve el nombre del metodo de pago a su pasarela.
type RegistroPasarelas struct {
	mu         sync.RWMutex
	pasarelas  map[string]Pasarela
	porDefecto string
}

// NuevoRegistroPasarelas crea un registro de pasarelas vacio.
func NuevoRegistroPasarelas() *RegistroPasarelas {
	return &RegistroPasarelas{pasarelas: map[string]Pasarela{}}
}

// Registrar agrega una pasarela por su nombre. La primera registrada queda
// como pasarela por defecto.
func (r *RegistroPasarelas) Registrar(p Pasarela) {
	r.mu.Lock()
	defer r.mu.Unlock()
	nombre := strings.ToLower(p.Nombre())
	r.pasarelas[nombre] = p
	if r.porDefecto == "" {
		r.porDefecto = nombre
	}
}

// Para devuelve la pasarela de un metodo de pago.
func (r *RegistroPasarelas) Para(metodo string) (Pasarela, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.pasarelas[strings.ToLower(strings.TrimSpace(metodo))]
	if !ok {
		return nil, fmt.Errorf("no hay pasarela para el metodo de pago %q", metodo)
	}
	return p, nil
}

// Metodos lista los metodos con pasarela disponible.
func (r *RegistroPasarelas) Metodos() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.pasarelas))
	for n := range r.pasarelas {
		out = append(out, n)
	}
	return out
}

// ---- Pasarela simulada ----

// PasarelaSimulada imita a un proveedor real sin salir a la red.
//
// No es aleatoria a proposito: unas reglas deterministas permiten escribir
// pruebas que comprueben tambien el camino del rechazo, que es justo el que
// nadie prueba y el que mas rompe en produccion.
//
// Reglas:
//   - monto <= 0            rechazado
//   - ultimos4 == "0000"    rechazado, simula tarjeta sin fondos
//   - metodo pse            pendiente con url de redireccion, como el real
//   - resto                 aprobado
type PasarelaSimulada struct {
	metodo string
	// demora imita la latencia del proveedor para que los tiempos de espera
	// del cliente se ejerciten de verdad.
	demora time.Duration
}

// NuevaPasarelaSimulada crea una pasarela de pruebas para el metodo dado, con
// una demora artificial por cobro.
func NuevaPasarelaSimulada(metodo string, demora time.Duration) *PasarelaSimulada {
	return &PasarelaSimulada{metodo: metodo, demora: demora}
}

// Nombre identifica la pasarela en el registro.
func (s *PasarelaSimulada) Nombre() string { return s.metodo }

// Cobrar simula un cobro: rechaza montos no positivos y la tarjeta terminada en
// 0000 (fondos insuficientes), deja PSE pendiente con la URL del banco y
// aprueba el resto.
func (s *PasarelaSimulada) Cobrar(ctx context.Context, c Cobro) (*ResultadoCobro, error) {
	if s.demora > 0 {
		select {
		case <-time.After(s.demora):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if c.Monto <= 0 {
		return &ResultadoCobro{
			Estado:  PagoRechazado,
			Mensaje: "El monto debe ser mayor que cero.",
		}, nil
	}
	if c.Ultimos4 == "0000" {
		return &ResultadoCobro{
			Estado:  PagoRechazado,
			Mensaje: "La entidad rechazo el pago por fondos insuficientes.",
		}, nil
	}

	ref := fmt.Sprintf("SIM-%s-%s-%d", strings.ToUpper(s.metodo), c.Referencia, time.Now().Unix())

	if s.metodo == "pse" {
		return &ResultadoCobro{
			Estado:            PagoPendiente,
			ReferenciaExterna: ref,
			URLRedireccion:    "https://sandbox.pse.example/pagar?ref=" + ref,
			Mensaje:           "Continua en el portal de tu banco para autorizar el pago.",
		}, nil
	}

	return &ResultadoCobro{
		Estado:            PagoAprobado,
		ReferenciaExterna: ref,
		Mensaje:           "Pago aprobado.",
	}, nil
}

// Reembolsar simula un reembolso; solo acepta referencias emitidas por esta
// pasarela (prefijo SIM-) y montos positivos.
func (s *PasarelaSimulada) Reembolsar(ctx context.Context, referenciaExterna string, monto float64) error {
	if monto <= 0 {
		return fmt.Errorf("el monto a reembolsar debe ser mayor que cero")
	}
	if !strings.HasPrefix(referenciaExterna, "SIM-") {
		return fmt.Errorf("referencia externa desconocida: %q", referenciaExterna)
	}
	return nil
}
