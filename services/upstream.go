// ClienteUpstream habla con los 7 servicios CRUD de Jobsy_API.
//
// Todo el conocimiento sobre sus rarezas vive aqui, para que el resto del mid
// trabaje con datos limpios:
//
//   - El envelope de respuesta tiene la clave "data" en Core, Catalogo y
//     Tienda, pero "date" (typo) en los POST y PUT de Empleo, Suscripciones,
//     Usuarios y Soporte. Son 56 sitios. Leemos ambas.
//   - El HTTP status es 200 aunque el body diga "status": 400. No se puede
//     enrutar por codigo HTTP: hay que mirar "success" del body.
//   - Si GetAll falla, la respuesta no es un objeto sino el string crudo del
//     error de PostgreSQL. Lo detectamos y lo convertimos en error sin
//     filtrarlo al cliente.
//   - Los modelos Go no tienen tags json, asi que los campos van en
//     PascalCase tanto de entrada como de salida.

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"api_MID/models"
)

// ClienteUpstream es un cliente HTTP apuntando a UN servicio de Jobsy_API.
type ClienteUpstream struct {
	servicio string
	baseURL  string
	http     *http.Client
}

// NuevoClienteUpstream crea el cliente HTTP de un API de datos con su nombre
// (para errores y logs), su URL base y el timeout de cada peticion.
func NuevoClienteUpstream(servicio, baseURL string, timeout time.Duration) *ClienteUpstream {
	return &ClienteUpstream{
		servicio: servicio,
		baseURL:  strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Servicio devuelve el nombre del upstream.
func (c *ClienteUpstream) Servicio() string { return c.servicio }

// sobre modela la respuesta de los upstream. Data y Date son el mismo campo
// escrito de dos formas; ver comentario del paquete.
type sobre struct {
	Success bool            `json:"success"`
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Date    json.RawMessage `json:"date"`
}

// carga devuelve el payload venga en "data" o en "date".
func (s sobre) carga() json.RawMessage {
	if len(s.Data) > 0 && !esNulo(s.Data) {
		return s.Data
	}
	return s.Date
}

// esNulo indica si el JSON crudo es el literal null.
func esNulo(r json.RawMessage) bool {
	return len(r) == 4 && string(r) == "null"
}

// OpcionesUpstream de una peticion a un upstream.
type OpcionesUpstream struct {
	// Query son los parametros de URL. Para filtrar, los upstream usan la
	// forma "query=Columna:valor,Otra:valor" de Beego.
	Query url.Values
	Body  any
}

// Get hace un GET a la ruta y decodifica los datos de la respuesta en destino.
func (c *ClienteUpstream) Get(ctx context.Context, ruta string, opts *OpcionesUpstream, destino any) error {
	return c.hacer(ctx, http.MethodGet, ruta, opts, destino)
}

// Post envia body como JSON a la ruta y decodifica la respuesta en destino.
func (c *ClienteUpstream) Post(ctx context.Context, ruta string, body any, destino any) error {
	return c.hacer(ctx, http.MethodPost, ruta, &OpcionesUpstream{Body: body}, destino)
}

// Put reemplaza el recurso completo. Ojo: el Update de Beego escribe TODAS las
// columnas, asi que un PUT parcial pone en cero los campos omitidos. Quien
// llame debe hacer Get, fusionar, y mandar el objeto entero.
func (c *ClienteUpstream) Put(ctx context.Context, ruta string, body any, destino any) error {
	return c.hacer(ctx, http.MethodPut, ruta, &OpcionesUpstream{Body: body}, destino)
}

// Parchear cambia solo las columnas indicadas de un recurso.
//
// Como el PUT de Beego escribe todas las columnas, primero lee la fila
// entera, le aplica los cambios y la reenvia completa. Asi lo que el mid no
// conoce, o no queria tocar, queda como estaba. Mandar un cuerpo armado a mano
// perdia columnas: una venta reescribia el producto con valores fijos y
// borraba su precio de lista, y una resena dejaba el perfil de la empleada
// con la modalidad y el turno por defecto.
//
// Las relaciones llegan anidadas con todas sus columnas; se reducen a su Id,
// que es lo que el backend necesita para escribir la clave foranea.
func (c *ClienteUpstream) Parchear(ctx context.Context, ruta string, cambiar func(fila map[string]any) error) (map[string]any, error) {
	var fila map[string]any
	if err := c.Get(ctx, ruta, nil, &fila); err != nil {
		return nil, err
	}
	if len(fila) == 0 {
		return nil, models.NoEncontrado("ese recurso")
	}
	if err := cambiar(fila); err != nil {
		return nil, err
	}
	for k, v := range fila {
		if anidada, ok := v.(map[string]any); ok {
			if id, ok := anidada["Id"]; ok {
				fila[k] = map[string]any{"Id": id}
			}
		}
	}
	if _, ok := fila["ActualizadoEn"]; ok {
		fila["ActualizadoEn"] = time.Now().Format(time.RFC3339)
	}
	var res map[string]any
	if err := c.Put(ctx, ruta, fila, &res); err != nil {
		return nil, err
	}
	return fila, nil
}

// Fijar es Parchear con valores concretos.
func (c *ClienteUpstream) Fijar(ctx context.Context, ruta string, cambios map[string]any) error {
	_, err := c.Parchear(ctx, ruta, func(fila map[string]any) error {
		for k, v := range cambios {
			fila[k] = v
		}
		return nil
	})
	return err
}

// Delete hace un DELETE a la ruta.
func (c *ClienteUpstream) Delete(ctx context.Context, ruta string) error {
	return c.hacer(ctx, http.MethodDelete, ruta, nil, nil)
}

// hacer arma y envia la peticion a <base>/v1/<ruta> y pasa la respuesta a
// interpretar. Los fallos de red se devuelven como error de upstream.
func (c *ClienteUpstream) hacer(ctx context.Context, metodo, ruta string, opts *OpcionesUpstream, destino any) error {
	u := c.baseURL + "/v1/" + strings.TrimLeft(ruta, "/")
	if opts != nil && len(opts.Query) > 0 {
		u += "?" + opts.Query.Encode()
	}

	var cuerpo io.Reader
	if opts != nil && opts.Body != nil {
		b, err := json.Marshal(opts.Body)
		if err != nil {
			return models.Interno(fmt.Errorf("serializando body para %s: %w", c.servicio, err))
		}
		cuerpo = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, metodo, u, cuerpo)
	if err != nil {
		return models.Interno(fmt.Errorf("construyendo peticion a %s: %w", c.servicio, err))
	}
	req.Header.Set("Accept", "application/json")
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		return models.Upstream(c.servicio, err)
	}
	defer res.Body.Close()

	crudo, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return models.Upstream(c.servicio, fmt.Errorf("leyendo respuesta: %w", err))
	}

	return c.interpretar(res.StatusCode, crudo, destino)
}

// interpretar traduce la respuesta cruda de un upstream a un error del mid o a
// un valor deserializado. Aqui es donde no confiamos en el status HTTP.
func (c *ClienteUpstream) interpretar(statusHTTP int, crudo []byte, destino any) error {
	limpio := bytes.TrimSpace(crudo)

	// Core_api responde 200 con body vacio cuando el Unmarshal del POST falla
	// (no hay rama else en el controller). Para el cliente eso es
	// indistinguible de un exito, asi que lo tratamos como fallo del upstream.
	if len(limpio) == 0 {
		return models.Upstream(c.servicio, fmt.Errorf("respuesta vacia con HTTP %d", statusHTTP))
	}

	// GetAll fallido devuelve el error de PostgreSQL como string JSON pelado.
	// Nunca se lo pasamos al cliente: lleva nombres de tabla y constraints.
	if limpio[0] == '"' {
		var msg string
		if err := json.Unmarshal(limpio, &msg); err == nil {
			return models.Upstream(c.servicio, fmt.Errorf("error crudo: %s", msg))
		}
	}

	var s sobre
	if err := json.Unmarshal(limpio, &s); err != nil {
		return models.Upstream(c.servicio, fmt.Errorf("respuesta ilegible (HTTP %d): %w", statusHTTP, err))
	}

	// success:false es siempre un fallo. Hubo aqui una heuristica que trataba
	// cierto mensaje de GetAll como lista vacia, porque el CRUD devolvia la
	// misma respuesta para "sin filas" y para "la consulta fallo". Eso hacia
	// que una base caida pareciera un listado vacio. Corregido el CRUD (sin
	// filas ahora es success:true con data vacia), la heuristica sobraba.
	if !s.Success {
		return c.desdeSobre(s, statusHTTP)
	}

	if destino == nil {
		return nil
	}

	carga := s.carga()
	if len(carga) == 0 || esNulo(carga) {
		// Un listado sin resultados llega como data:null con success:true en
		// unos servicios y como error 400 en otros. En ambos casos, para un
		// destino de tipo slice, son cero filas y no un fallo.
		if esSlice(destino) {
			return nil
		}
		return models.Upstream(c.servicio, fmt.Errorf("respuesta exitosa sin payload (message=%q)", s.Message))
	}

	if err := json.Unmarshal(carga, destino); err != nil {
		return models.Upstream(c.servicio, fmt.Errorf("payload no encaja en el destino: %w", err))
	}
	return nil
}

// esSlice indica si quien llama espera una coleccion.
func esSlice(destino any) bool {
	if destino == nil {
		return false
	}
	v := reflect.ValueOf(destino)
	return v.Kind() == reflect.Ptr && !v.IsNil() && v.Elem().Kind() == reflect.Slice
}

// desdeSobre mapea el status que el upstream puso DENTRO del body al error del
// mid que le corresponde.
func (c *ClienteUpstream) desdeSobre(s sobre, statusHTTP int) error {
	causa := fmt.Errorf("HTTP %d, body status %d: %s", statusHTTP, s.Status, s.Message)
	switch s.Status {
	case http.StatusNotFound:
		return models.NoEncontrado("el recurso").Con(causa)
	case http.StatusBadRequest:
		// El upstream no distingue validacion de violacion de integridad:
		// todo cae en 400 con el mensaje de PostgreSQL. Lo reportamos como
		// fallo de upstream para no dar por buena una validacion que no hizo.
		return models.Upstream(c.servicio, causa)
	case http.StatusConflict:
		return models.Conflicto("El recurso ya existe o esta en uso.").Con(causa)
	default:
		return models.Upstream(c.servicio, causa)
	}
}

// QueryBeego construye el parametro "query" que espera GetAll:
// query=Columna:valor,Otra:valor
func QueryBeego(pares map[string]string) string {
	if len(pares) == 0 {
		return ""
	}
	partes := make([]string, 0, len(pares))
	for k, v := range pares {
		partes = append(partes, k+":"+v)
	}
	return strings.Join(partes, ",")
}

// EnParalelo ejecuta varias llamadas a la vez y devuelve el primer error.
//
// Esta es la pieza que responde a "si el front pide 2 tablas, el mid hace los
// GET y los devuelve en 1": las lecturas independientes no deben ir en serie.
func EnParalelo(fns ...func() error) error {
	var wg sync.WaitGroup
	errs := make([]error, len(fns))
	for i, fn := range fns {
		wg.Add(1)
		go func(i int, fn func() error) {
			defer wg.Done()
			errs[i] = fn()
		}(i, fn)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
