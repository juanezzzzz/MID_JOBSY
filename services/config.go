package services

import (
	"strconv"
	"time"
	"strings"
	"fmt"
	"github.com/beego/beego/v2/server/web"
)

type ConfigUpstream struct {
	Nombre string
	BaseURL string
}

type Config struct {
	Puerto string
	Entorno string
	JWTSecreto []byte
	JWTDuracion time.Duration
	OrigenesPermitidos []string
	SecretoPasarela string
	TimeoutUpstream time.Duration
	DirArchivos string
	URLPublica string
	Core ConfigUpstream
	Usuarios ConfigUpstream
	Empleo ConfigUpstream
	Catalogo ConfigUpstream
	Tienda ConfigUpstream
	Suscripciones ConfigUpstream
	Soporte ConfigUpstream
}

func CargarConfig() (*Config, error) {
	c := &Config{
		Puerto: strconv.Itoa(web.BConfig.Listen.HTTPPort),
		Entorno: conf("entorno", "dev"),
		JWTDuracion: confDuracion("jwt_duracion", 24*time.Hour),
		TimeoutUpstream: confDuracion("timeout_upstream", 10*time.Second),
		OrigenesPermitidos: confLista("origenes", []string{"http://localhost:4200"}),
		DirArchivos: conf("dir_archivos", "archivos"),
		URLPublica: conf("url_publica", "http://localhost:9090"),
		SecretoPasarela: conf("secreto_pasarela", ""),
		Core: ConfigUpstream{"core", conf("url_core", "http://localhost:8081")},
		Usuarios: ConfigUpstream{"usuarios", conf("url_usuarios", "http://localhost:8082")},
		Empleo: ConfigUpstream{"empleo", conf("url_empleo", "http://localhost:8083")},
		Catalogo: ConfigUpstream{"catalogo", conf("url_catalago", "http://localhost:8084")},
		Tienda: ConfigUpstream{"tienda", conf("url_tienda", "http://localhost:8085")},
		Suscripciones: ConfigUpstream{"suscripciones", conf("url_suscripciones", "http://localhost:8086")},
		Soporte: ConfigUpstream{"soporte", conf("url_soporte", "http://localhost:8087")},
	}
	secreto := conf("jwt_secreto", "")
	if secreto == "" {
		return nil, fmt.Errorf("MID_JWT_SECRETO es obligatorio y esta vacio")
	}
	if len(secreto) < 32 {
		return  nil, fmt.Errorf("MID_JWT_SECRETO debe tener al menos 32 caracteres, tiene %d", len(secreto))
	}
	c.JWTSecreto = []byte(secreto)
	if c.Entorno == "prod" {
		for _, o := range c.OrigenesPermitidos {
			if o == "*" {
				return nil, fmt.Errorf("MID_ORIGENES no puede ser \"*\" en produccion")
			}
		}
	}
	return c, nil
}

func conf(clave, porDefecto string) string {
	if v := strings.TrimSpace(web.AppConfig.DefaultString(clave, "")); v != "" {
		return v
	}
	return porDefecto
}

func confDuracion(clave string, porDefecto time.Duration) time.Duration {
	v := conf(clave, "")
	if v == "" {
		return porDefecto
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if s, err := strconv.Atoi(v); err == nil {
		return time.Duration(s) * time.Second
	}
	return porDefecto
}

func confLista(clave string, porDefecto []string) []string {
	v :=conf(clave, "")
	if v == "" {
		return porDefecto
	}
	partes := strings.Split(v, ",")
	out := make([]string, 0, len(partes))
	for _, p := range partes {
		if p = strings.TrimSpace(p); p != ""{
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return porDefecto
	}
	return out
}