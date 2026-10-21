// Configuracion del middleware, leida de conf/app.conf.
//
// El app.conf toma cada valor de una variable de entorno con ${VAR||defecto},
// asi que en despliegue se sigue configurando por entorno como antes; el
// archivo solo deja escritos los nombres y los valores por defecto.
//
// Los 7 servicios de Jobsy_API no definen puerto por defecto: su app.conf usa
// ${VAR} sin fallback, asi que los 7 caen al 8080 de Beego y colisionan. El mid
// es quien decide en que puerto vive cada uno, por eso aqui hay un valor por
// defecto para todos.
package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/beego/beego/v2/server/web"
)

// ConfigUpstream es un servicio de Jobsy_API visto desde el mid.
type ConfigUpstream struct {
	Nombre  string
	BaseURL string
}

type Config struct {
	Puerto      string
	Entorno     string
	JWTSecreto  []byte
	JWTDuracion time.Duration

	// Origenes permitidos por CORS. Los upstream traen "*" con credentials,
	// que es justo lo que no se debe hacer; aqui va una lista explicita.
	OrigenesPermitidos []string

	// SecretoPasarela es la clave compartida con el proveedor de pago, con la
	// que se firman sus notificaciones asincronas.
	//
	// Vacia a proposito por defecto: sin ella la ruta de notificacion no
	// atiende, que es lo correcto mientras no haya un proveedor real. Lo
	// contrario seria aceptar a ciegas que cualquiera confirme un pago.
	SecretoPasarela string

	TimeoutUpstream time.Duration

	// DirArchivos es donde se guardan las subidas; URLPublica es el origen
	// con el que se construyen sus enlaces.
	DirArchivos string
	URLPublica  string

	Core          ConfigUpstream
	Usuarios      ConfigUpstream
	Empleo        ConfigUpstream
	Catalogo      ConfigUpstream
	Tienda        ConfigUpstream
	Suscripciones ConfigUpstream
	Soporte       ConfigUpstream
}

// CargarConfig lee app.conf y valida lo que no puede tener valor por defecto.
func CargarConfig() (*Config, error) {
	c := &Config{
		Puerto:             strconv.Itoa(web.BConfig.Listen.HTTPPort),
		Entorno:            conf("entorno", "dev"),
		JWTDuracion:        confDuracion("jwt_duracion", 24*time.Hour),
		TimeoutUpstream:    confDuracion("timeout_upstream", 10*time.Second),
		OrigenesPermitidos: confLista("origenes", []string{"http://localhost:4200"}),
		DirArchivos:        conf("dir_archivos", "archivos"),
		URLPublica:         conf("url_publica", "http://localhost:9090"),
		SecretoPasarela:    conf("secreto_pasarela", ""),

		Core:          ConfigUpstream{"core", conf("url_core", "http://localhost:8081")},
		Usuarios:      ConfigUpstream{"usuarios", conf("url_usuarios", "http://localhost:8082")},
		Empleo:        ConfigUpstream{"empleo", conf("url_empleo", "http://localhost:8083")},
		Catalogo:      ConfigUpstream{"catalogo", conf("url_catalogo", "http://localhost:8084")},
		Tienda:        ConfigUpstream{"tienda", conf("url_tienda", "http://localhost:8085")},
		Suscripciones: ConfigUpstream{"suscripciones", conf("url_suscripciones", "http://localhost:8086")},
		Soporte:       ConfigUpstream{"soporte", conf("url_soporte", "http://localhost:8087")},
	}

	secreto := conf("jwt_secreto", "")
	if secreto == "" {
		// Sin secreto no se firma nada. Preferimos no arrancar antes que
		// arrancar con una clave por defecto que alguien deje en produccion.
		return nil, fmt.Errorf("MID_JWT_SECRETO es obligatorio y esta vacio")
	}
	if len(secreto) < 32 {
		return nil, fmt.Errorf("MID_JWT_SECRETO debe tener al menos 32 caracteres, tiene %d", len(secreto))
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

// Upstreams devuelve los 7 servicios, para healthchecks y arranque.
func (c *Config) Upstreams() []ConfigUpstream {
	return []ConfigUpstream{c.Core, c.Usuarios, c.Empleo, c.Catalogo, c.Tienda, c.Suscripciones, c.Soporte}
}

// conf lee una clave de app.conf devolviendo porDefecto si esta vacia o falta.
func conf(clave, porDefecto string) string {
	if v := strings.TrimSpace(web.AppConfig.DefaultString(clave, "")); v != "" {
		return v
	}
	return porDefecto
}

// confDuracion lee una duracion de app.conf. Admite el formato de Go (p. ej.
// "10s", "24h") y tambien segundos pelados, como suelen venir de un .env.
func confDuracion(clave string, porDefecto time.Duration) time.Duration {
	v := conf(clave, "")
	if v == "" {
		return porDefecto
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	// Tambien admitimos segundos pelados, que es como suele venir de un .env.
	if s, err := strconv.Atoi(v); err == nil {
		return time.Duration(s) * time.Second
	}
	return porDefecto
}

// confLista lee una lista separada por comas de app.conf, recortando espacios
// y descartando vacios; devuelve porDefecto si no queda ningun valor.
func confLista(clave string, porDefecto []string) []string {
	v := conf(clave, "")
	if v == "" {
		return porDefecto
	}
	partes := strings.Split(v, ",")
	out := make([]string, 0, len(partes))
	for _, p := range partes {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return porDefecto
	}
	return out
}
