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