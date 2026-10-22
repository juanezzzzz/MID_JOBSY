package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/v2/server/web"
	beecontext "github.com/beego/beego/v2/server/web/context"

	"api_MID/models"
)

// Los filtros se registran en routers con web.InsertFilterChain y se
// ejecutan en el orden en que se insertan, envolviendo a todo lo demas.

// ---- Identificador de peticion ----

type clavePeticionID struct{}

// PeticionID recupera el identificador de correlacion de la peticion.
func PeticionID(ctx context.Context) string {
	id, _ := ctx.Value(clavePeticionID{}).(string)
	return id
}

// ConPeticionID asigna un id a cada peticion para poder cruzar el log del mid
// con lo que el cliente ve en el campo peticionId del error.
func ConPeticionID(siguiente web.FilterFunc) web.FilterFunc {
	return func(ctx *beecontext.Context) {
		id := ctx.Input.Header("X-Request-Id")
		if id == "" || len(id) > 64 {
			b := make([]byte, 8)
			if _, err := rand.Read(b); err == nil {
				id = hex.EncodeToString(b)
			}
		}
		ctx.Output.Header("X-Request-Id", id)
		ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), clavePeticionID{}, id))
		siguiente(ctx)
	}
}

// ---- Recuperacion de panicos ----

// Recuperar evita que un panico tumbe el proceso entero. Se instala como
// web.BConfig.RecoverFunc, que Beego difiere alrededor de cada peticion.
//
// Importa especialmente aqui: el bug de LoadRelated sobre campos no
// relacionales hace que 26 de las 55 entidades de Jobsy_API puedan responder
// 500. Eso no debe arrastrarse al mid.
func Recuperar(ctx *beecontext.Context, _ *web.Config) {
	rec := recover()
	if rec == nil || esAbort(rec) {
		// StopRun corta la accion con un panico controlado: la respuesta ya
		// esta escrita.
		return
	}
	slog.Error("panico atendiendo la peticion",
		"peticion_id", PeticionID(ctx.Request.Context()),
		"ruta", ctx.Request.URL.Path,
		"panico", rec,
		"stack", string(debug.Stack()),
	)
	if !ctx.ResponseWriter.Started {
		escribirError(ctx, models.Interno(nil))
	}
}

// ---- Registro ----

type contadorBytes struct {
	http.ResponseWriter
	bytes int
}

// Write delega en el ResponseWriter real y va sumando los bytes escritos, para
// poder registrar cuanto peso la respuesta.
func (c *contadorBytes) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.bytes += n
	return n, err
}

// Registrar es el filtro que deja una linea de log por peticion con metodo,
// ruta, status, bytes y duracion, correlacionada por el id de peticion.
func Registrar(siguiente web.FilterFunc) web.FilterFunc {
	return func(ctx *beecontext.Context) {
		inicio := time.Now()
		cb := &contadorBytes{ResponseWriter: ctx.ResponseWriter.ResponseWriter}
		ctx.ResponseWriter.ResponseWriter = cb
		siguiente(ctx)

		status := ctx.ResponseWriter.Status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Info("peticion",
			"peticion_id", PeticionID(ctx.Request.Context()),
			"metodo", ctx.Request.Method,
			"ruta", ctx.Request.URL.Path,
			"status", status,
			"bytes", cb.bytes,
			"ms", time.Since(inicio).Milliseconds(),
		)
	}
}

// ---- CORS ----

// CORS responde solo a los origenes de la lista.
//
// Los upstream usan AllowAllOrigins con AllowCredentials, que en la practica
// refleja el origen de quien pregunte: cualquier web podria leerles
// /v1/credenciales. Aqui la lista es explicita.
func CORS(siguiente web.FilterFunc) web.FilterFunc {
	return func(ctx *beecontext.Context) {
		origen := ctx.Input.Header("Origin")
		if origen != "" && slices.Contains(svc.Cfg.OrigenesPermitidos, origen) {
			ctx.Output.Header("Access-Control-Allow-Origin", origen)
			ctx.Output.Header("Access-Control-Allow-Credentials", "true")
			ctx.Output.Header("Vary", "Origin")
		}

		if ctx.Input.Method() == http.MethodOptions {
			ctx.Output.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			ctx.Output.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-Id")
			ctx.Output.Header("Access-Control-Max-Age", "600")
			ctx.ResponseWriter.WriteHeader(http.StatusNoContent)
			return
		}
		siguiente(ctx)
	}
}

// ---- Limite de peticiones ----

// limitador es un token bucket por clave (IP o usuario).
type limitador struct {
	mu       sync.Mutex
	cubos    map[string]*cubo
	porMin   int
	rafaga   int
	ultLimpi time.Time
}

type cubo struct {
	fichas    float64
	ultimoUso time.Time
}

// LimitarPeticiones aplica un limite por IP.
//
// Los upstream no tienen ninguno, y ademas aceptan ?limit=-1, que en Beego
// significa "sin limite" y descarga la tabla entera. El mid es quien frena eso.
func LimitarPeticiones(porMinuto, rafaga int) web.FilterChain {
	l := &limitador{
		cubos:    map[string]*cubo{},
		porMin:   porMinuto,
		rafaga:   rafaga,
		ultLimpi: time.Now(),
	}
	return func(siguiente web.FilterFunc) web.FilterFunc {
		return func(ctx *beecontext.Context) {
			if !l.permitir(clienteIP(ctx.Request)) {
				escribirError(ctx, models.Limite())
				return
			}
			siguiente(ctx)
		}
	}
}

// permitir aplica el algoritmo token bucket para una clave (IP): repone fichas
// segun el tiempo transcurrido y devuelve false si no quedan, es decir, si hay
// que rechazar la peticion por exceso de trafico.
func (l *limitador) permitir(clave string) bool {
	ahora := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Limpieza perezosa: sin esto el mapa crece sin limite y es un vector de
	// agotamiento de memoria.
	if ahora.Sub(l.ultLimpi) > 5*time.Minute {
		for k, c := range l.cubos {
			if ahora.Sub(c.ultimoUso) > 10*time.Minute {
				delete(l.cubos, k)
			}
		}
		l.ultLimpi = ahora
	}

	c, ok := l.cubos[clave]
	if !ok {
		l.cubos[clave] = &cubo{fichas: float64(l.rafaga) - 1, ultimoUso: ahora}
		return true
	}

	// Reponer en proporcion al tiempo transcurrido.
	transcurrido := ahora.Sub(c.ultimoUso).Minutes()
	c.fichas += transcurrido * float64(l.porMin)
	if c.fichas > float64(l.rafaga) {
		c.fichas = float64(l.rafaga)
	}
	c.ultimoUso = ahora

	if c.fichas < 1 {
		return false
	}
	c.fichas--
	return true
}

// clienteIP determina la IP del cliente para el rate limit: usa
// X-Forwarded-For si hay un proxy propio delante, o RemoteAddr en otro caso.
func clienteIP(r *http.Request) string {
	// X-Forwarded-For solo es fiable detras de un proxy propio. Si el mid se
	// expone directo, esta cabecera la puede falsear el cliente; en ese caso
	// hay que ignorarla y quedarse con RemoteAddr.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
