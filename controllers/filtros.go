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

type clavePeticionID struct{}

func PeticionID(ctx context.Context) string {
	id, _ := ctx.Value(clavePeticionID{}).(string)
	return id
}

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

func Recuperar(ctx *beecontext.Context, _ *web.Config) {
	rec := recover()
	if rec == nil || esAbort(rec) {
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

type contadorBytes struct {
	http.ResponseWriter
	bytes int
}

func (c *contadorBytes) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.bytes += n
	return n, err
}

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
			"status", cb.bytes,
			"ms", time.Since(inicio).Milliseconds(),
		)
	}
}

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

type limitador struct {
	mu sync.Mutex
	cubos map[string]*cubo
	porMin int
	rafaga int
	ultLimpi time.Time
}

type cubo struct {
	fichas float64
	ultimoUso time.Time
}

func LimitarPeticiones(porMinuto, rafaga int) web.FilterChain {
	l := &limitador{
		cubos: map[string]*cubo{},
		porMin: porMinuto,
		rafaga: rafaga,
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