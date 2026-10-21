// Catalogo traduce entre los enums de texto del frontend y las claves
// foraneas enteras de los catalogos de PostgreSQL.
//
// El frontend habla en cadenas ('jornada_completa'). La base guarda
// id_modalidad_trabajo INT con FK a catalogo.modalidades_trabajo. Nadie mas en
// el mid deberia saber que existe esa diferencia.
//
// Los catalogos son tablas pequenas que casi nunca cambian, asi que se cargan
// una vez al arrancar y se refrescan cada TTL. Si un catalogo no se puede
// cargar, el mid no arranca: prefiero fallar al inicio que traducir mal en
// caliente.

package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"api_MID/models"
)

// DESAJUSTE CONOCIDO entre el front y la base, en modalidades de trabajo.
//
//	front (4)                base (6)
//	---------                --------
//	por_horas          <->   por_horas
//	jornada_completa   <->   tiempo_completo
//	fin_de_semana      <->   fines_semana
//	presencial         <->   (no existe)
//	(no existe)        <->   por_jornadas
//	(no existe)        <->   interno
//	(no existe)        <->   dias_especificos
//
// 'presencial' no es una modalidad de jornada sino de lugar: esta en la
// dimension equivocada. Y la base tiene tres modalidades que el front no sabe
// pintar. Esto hay que decidirlo a nivel de producto; mientras tanto el mid
// traduce lo que puede y devuelve un error explicito para 'presencial' en vez
// de inventarse un id.
var modalidadFrontABase = map[models.Modalidad]string{
	models.ModalidadPorHoras:     "por_horas",
	models.ModalidadJornadaCompl: "tiempo_completo",
	models.ModalidadFinDeSemana:  "fines_semana",
}

var modalidadBaseAFront = map[string]models.Modalidad{
	"por_horas":       models.ModalidadPorHoras,
	"tiempo_completo": models.ModalidadJornadaCompl,
	"fines_semana":    models.ModalidadFinDeSemana,
	// Las tres que el front no modela se acercan a lo mas parecido para no
	// romper el listado. Se registra en el log cuando ocurre.
	"por_jornadas":     models.ModalidadJornadaCompl,
	"interno":          models.ModalidadJornadaCompl,
	"dias_especificos": models.ModalidadPorHoras,
}

// ModalidadesSinEquivalenteEnFront son las que la base soporta y el front no
// sabe representar. Se exponen para que el handler pueda avisar.
var ModalidadesSinEquivalenteEnFront = []string{"por_jornadas", "interno", "dias_especificos"}

// Tamano: el front tiene 3 valores + esLujo; la base mete 'lujoso' como cuarto
// tamano. Traducimos en ambos sentidos sin perder informacion.
var tamanoBaseAFront = map[string]struct {
	Tamano models.Tamano
	EsLujo bool
}{
	"pequeno": {models.TamanoPequeno, false},
	"mediano": {models.TamanoMediano, false},
	"grande":  {models.TamanoGrande, false},
	"lujoso":  {models.TamanoGrande, true},
}

// campoID es el nombre bajo el que llega la clave primaria en el JSON de los
// upstream. Beego serializa por el nombre del campo Go, y en todos estos
// modelos ese campo se llama Id, no IdTamanoInmueble ni IdEstado. Indexar por
// el nombre de la columna devolvia cero filas sin dar ningun error: la
// peticion HTTP tenia exito y el mapa quedaba vacio.
const campoID = "Id"

// entrada es una fila de un catalogo simple (id + nombre).
type entrada struct {
	ID     int
	Nombre string
}

// filaUpstream refleja el JSON que devuelven los CRUD de Beego. Los modelos Go
// de Jobsy_API no tienen tags json, asi que los campos llegan en PascalCase y
// con el nombre de la columna PK, que cambia por tabla.
type filaUpstream map[string]any

// Catalogo mantiene en memoria los catalogos maestros.
type Catalogo struct {
	cliente *ClienteUpstream
	// empleo sirve tamano_inmueble, que pese a ser un catalogo no vive en
	// Catalogo_api.
	empleo *ClienteUpstream
	ttl    time.Duration

	mu           sync.RWMutex
	cargadoEn    time.Time
	modalidades  map[string]int // nombre -> id
	modalidadesR map[int]string // id -> nombre
	tamanos      map[string]int
	tamanosR     map[int]string
	tamanosEn    time.Time
	estados      map[string]map[string]int // contexto -> nombre -> id
	estadosR     map[int]string            // id -> nombre
	tipos        map[string]map[string]int
	tiposR       map[int]string
	dias         map[string]int
	diasR        map[int]string
}

// NuevoCatalogo crea el catalogo sobre su upstream. Si ttl no es positivo se
// usan 15 minutos entre recargas.
func NuevoCatalogo(cliente *ClienteUpstream, ttl time.Duration) *Catalogo {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &Catalogo{cliente: cliente, ttl: ttl}
}

// Precargar trae todos los catalogos. Llamar al arrancar; si falla, no arranques.
func (c *Catalogo) Precargar(ctx context.Context) error {
	return c.recargar(ctx)
}

// asegurar recarga los catalogos si la cache vencio. Si la recarga falla pero
// ya habia datos, sigue sirviendo los anteriores.
func (c *Catalogo) asegurar(ctx context.Context) error {
	c.mu.RLock()
	fresco := time.Since(c.cargadoEn) < c.ttl && c.modalidades != nil
	hayDatos := c.modalidades != nil
	c.mu.RUnlock()
	if fresco {
		return nil
	}

	err := c.recargar(ctx)
	if err == nil {
		return nil
	}
	// Si la recarga falla pero ya hubo una carga buena, se siguen sirviendo
	// esos datos: los catalogos casi nunca cambian y tumbar todos los
	// listados porque un refresco fallo es peor que responder con la version
	// de hace un rato. Sin ninguna carga previa no hay nada que servir.
	if hayDatos {
		slog.Warn("recarga de catalogos fallida; se sirve la cache anterior", "error", err)
		return nil
	}
	return err
}

// recargar trae en paralelo modalidades, estados, tipos y dias de la semana y
// reemplaza los indices en memoria.
func (c *Catalogo) recargar(ctx context.Context) error {
	var (
		modalidades []filaUpstream
		estados     []filaUpstream
		tipos       []filaUpstream
		dias        []filaUpstream
	)

	// Cuatro lecturas independientes: van en paralelo.
	// tamano_inmueble no esta aqui porque lo sirve Empleo_API, no Catalogo_api;
	// se carga con CargarTamanos.
	err := EnParalelo(
		func() error { return c.cliente.Get(ctx, "modalidades_trabajo", sinLimite(), &modalidades) },
		func() error { return c.cliente.Get(ctx, "estados", sinLimite(), &estados) },
		func() error { return c.cliente.Get(ctx, "tipos", sinLimite(), &tipos) },
		func() error { return c.cliente.Get(ctx, "dias_semana", sinLimite(), &dias) },
	)
	if err != nil {
		return fmt.Errorf("cargando catalogos: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.modalidades, c.modalidadesR = indexar(modalidades, campoID)
	c.dias, c.diasR = indexar(dias, campoID)
	c.estados, c.estadosR = indexarPorContexto(estados, campoID)
	c.tipos, c.tiposR = indexarPorContexto(tipos, campoID)
	c.cargadoEn = time.Now()

	// Los tamanos se refrescan en la misma pasada: si la carga inicial fallo,
	// sin esto la cache se quedaria vacia indefinidamente.
	if c.empleo != nil {
		go func() {
			if err := c.recargarTamanos(context.WithoutCancel(ctx)); err != nil {
				_ = err
			}
		}()
	}
	return nil
}

// CargarTamanos registra el cliente de Empleo y trae tamano_inmueble, que pese
// a ser un catalogo vive en Empleo_API y no en Catalogo_api.
func (c *Catalogo) CargarTamanos(ctx context.Context, clienteEmpleo *ClienteUpstream) error {
	c.mu.Lock()
	c.empleo = clienteEmpleo
	c.mu.Unlock()
	return c.recargarTamanos(ctx)
}

// asegurarTamanos garantiza que tamano_inmueble este cargado.
//
// asegurar() solo refresca lo que sirve Catalogo_api. Los tamanos vienen de
// Empleo_API y se cargaban una unica vez al arrancar: si Empleo no respondia
// en ese instante, la cache quedaba vacia para el resto de la vida del
// proceso, y con ella se caian el listado de ofertas (descartaba cada fila) y
// la contratacion directa. Se observo tras un reinicio.
func (c *Catalogo) asegurarTamanos(ctx context.Context) error {
	c.mu.RLock()
	fresco := time.Since(c.tamanosEn) < c.ttl && c.tamanos != nil
	hayDatos := c.tamanos != nil
	c.mu.RUnlock()
	if fresco {
		return nil
	}

	err := c.recargarTamanos(ctx)
	if err == nil {
		return nil
	}
	// Igual que con el resto de catalogos: una version de hace un rato es
	// mejor que tumbar los listados por un refresco fallido.
	if hayDatos {
		slog.Warn("recarga de tamanos fallida; se sirve la cache anterior", "error", err)
		return nil
	}
	return err
}

// recargarTamanos trae los tamanos de inmueble desde Empleo_API, que es quien
// los sirve, y reemplaza su indice.
func (c *Catalogo) recargarTamanos(ctx context.Context) error {
	c.mu.RLock()
	cli := c.empleo
	c.mu.RUnlock()
	if cli == nil {
		return fmt.Errorf("no hay cliente de empleo para cargar los tamanos")
	}

	var filas []filaUpstream
	if err := cli.Get(ctx, "tamano_inmueble", sinLimite(), &filas); err != nil {
		return fmt.Errorf("cargando tamanos de inmueble: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tamanos, c.tamanosR = indexar(filas, campoID)
	c.tamanosEn = time.Now()
	return nil
}

// sinLimite devuelve opciones con un limit alto para traer los catalogos
// completos (Beego pagina de 10 en 10 por defecto).
func sinLimite() *OpcionesUpstream {
	o := &OpcionesUpstream{Query: map[string][]string{}}
	// Los catalogos son pequenos, pero el default de Beego es limit=10 y nos
	// dejaria fuera la mitad de los estados.
	o.Query.Set("limit", "500")
	return o
}

// indexar arma los mapas nombre->id e id->nombre de un catalogo, saltando las
// filas sin id o sin nombre.
func indexar(filas []filaUpstream, clave string) (map[string]int, map[int]string) {
	porNombre := make(map[string]int, len(filas))
	porID := make(map[int]string, len(filas))
	for _, f := range filas {
		id, ok := entero(f[clave])
		if !ok {
			continue
		}
		nombre := strings.TrimSpace(texto(f["Nombre"]))
		if nombre == "" {
			continue
		}
		porNombre[nombre] = id
		porID[id] = nombre
	}
	return porNombre, porID
}

// indexarPorContexto agrupa estados y tipos, que son tablas polimorficas:
// la unicidad es (contexto, nombre), no solo nombre.
func indexarPorContexto(filas []filaUpstream, clave string) (map[string]map[string]int, map[int]string) {
	porCtx := map[string]map[string]int{}
	porID := map[int]string{}
	for _, f := range filas {
		id, ok := entero(f[clave])
		if !ok {
			continue
		}
		ctx := strings.TrimSpace(texto(f["Contexto"]))
		nombre := strings.TrimSpace(texto(f["Nombre"]))
		if ctx == "" || nombre == "" {
			continue
		}
		if porCtx[ctx] == nil {
			porCtx[ctx] = map[string]int{}
		}
		porCtx[ctx][nombre] = id
		porID[id] = nombre
	}
	return porCtx, porID
}

// entero convierte a int un valor numerico decodificado de JSON; el bool indica
// si el valor era un numero.
func entero(v any) (int, bool) {
	switch n := v.(type) {
	case float64: // encoding/json decodifica todo numero como float64
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

// texto devuelve el valor como string, o cadena vacia si no lo es.
func texto(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ---- Traduccion ----

// IDModalidad convierte la modalidad del front al id de catalogo.
func (c *Catalogo) IDModalidad(ctx context.Context, m models.Modalidad) (int, error) {
	if m == models.ModalidadPresencial {
		return 0, models.Validacion(
			"La modalidad 'presencial' no existe en el catalogo de la base. " +
				"Es una modalidad de lugar, no de jornada; hay que decidirla a nivel de producto.")
	}
	nombreBase, ok := modalidadFrontABase[m]
	if !ok {
		return 0, models.Validacion(fmt.Sprintf("Modalidad desconocida: %q.", m))
	}
	if err := c.asegurar(ctx); err != nil {
		return 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.modalidades[nombreBase]
	if !ok {
		return 0, models.Upstream("catalogo",
			fmt.Errorf("catalogo.modalidades_trabajo no tiene la fila %q; hay que sembrarla", nombreBase))
	}
	return id, nil
}

// ModalidadDesdeID hace el camino inverso para pintar un listado.
func (c *Catalogo) ModalidadDesdeID(ctx context.Context, id int) (models.Modalidad, error) {
	if err := c.asegurar(ctx); err != nil {
		return "", err
	}
	c.mu.RLock()
	nombre, ok := c.modalidadesR[id]
	c.mu.RUnlock()
	if !ok {
		return "", models.Upstream("catalogo", fmt.Errorf("id_modalidad_trabajo %d no esta en el catalogo", id))
	}
	m, ok := modalidadBaseAFront[nombre]
	if !ok {
		return "", models.Upstream("catalogo", fmt.Errorf("modalidad %q sin equivalente en el front", nombre))
	}
	return m, nil
}

// TamanoDesdeID devuelve el tamano y la gama por separado, deshaciendo el
// 'lujoso' de la base.
func (c *Catalogo) TamanoDesdeID(ctx context.Context, id int) (models.Tamano, bool, error) {
	if err := c.asegurarTamanos(ctx); err != nil {
		return "", false, err
	}
	c.mu.RLock()
	nombre, ok := c.tamanosR[id]
	c.mu.RUnlock()
	if !ok {
		return "", false, models.Upstream("empleo", fmt.Errorf("id_tamano_inmueble %d no esta en el catalogo", id))
	}
	par, ok := tamanoBaseAFront[nombre]
	if !ok {
		return "", false, models.Upstream("empleo", fmt.Errorf("tamano %q desconocido", nombre))
	}
	return par.Tamano, par.EsLujo, nil
}

// IDTamano convierte tamano + gama al id de la base. Si esLujo, gana 'lujoso',
// que es como la base representa hoy la gama alta.
func (c *Catalogo) IDTamano(ctx context.Context, t models.Tamano, esLujo bool) (int, error) {
	if err := c.asegurarTamanos(ctx); err != nil {
		return 0, err
	}
	nombre := string(t)
	if esLujo {
		nombre = "lujoso"
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.tamanos[nombre]
	if !ok {
		return 0, models.Upstream("empleo", fmt.Errorf("empleo.tamano_inmueble no tiene la fila %q", nombre))
	}
	return id, nil
}

// IDEstado resuelve un estado dentro de su contexto ('oferta', 'postulacion',
// 'contratacion', 'reporte'...).
func (c *Catalogo) IDEstado(ctx context.Context, contexto, nombre string) (int, error) {
	if err := c.asegurar(ctx); err != nil {
		return 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	porNombre, ok := c.estados[contexto]
	if !ok {
		return 0, models.Upstream("catalogo", fmt.Errorf("no hay estados con contexto %q", contexto))
	}
	id, ok := porNombre[nombre]
	if !ok {
		return 0, models.Upstream("catalogo", fmt.Errorf("estado %q/%q no existe", contexto, nombre))
	}
	return id, nil
}

// EstadoDesdeID devuelve el nombre del estado sin su contexto.
func (c *Catalogo) EstadoDesdeID(ctx context.Context, id int) (string, error) {
	if err := c.asegurar(ctx); err != nil {
		return "", err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	nombre, ok := c.estadosR[id]
	if !ok {
		return "", models.Upstream("catalogo", fmt.Errorf("id_estado %d no esta en el catalogo", id))
	}
	return nombre, nil
}

// IDTipo resuelve un tipo dentro de su contexto ('mascota', 'pago',
// 'producto_tienda', 'notificacion'...).
func (c *Catalogo) IDTipo(ctx context.Context, contexto, nombre string) (int, error) {
	if err := c.asegurar(ctx); err != nil {
		return 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	porNombre, ok := c.tipos[contexto]
	if !ok {
		return 0, models.Upstream("catalogo", fmt.Errorf("no hay tipos con contexto %q", contexto))
	}
	id, ok := porNombre[nombre]
	if !ok {
		return 0, models.Upstream("catalogo", fmt.Errorf("tipo %q/%q no existe", contexto, nombre))
	}
	return id, nil
}

// TipoDesdeID devuelve el nombre de un tipo dentro de su contexto.
func (c *Catalogo) TipoDesdeID(ctx context.Context, contexto string, id int) (string, error) {
	if err := c.asegurar(ctx); err != nil {
		return "", err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for nombre, n := range c.tipos[contexto] {
		if n == id {
			return nombre, nil
		}
	}
	return "", models.Upstream("catalogo", fmt.Errorf("tipo %d no existe en el contexto %q", id, contexto))
}

// IDDiaSemana resuelve el codigo de un dia ("lun", "mar"...) a su id.
func (c *Catalogo) IDDiaSemana(ctx context.Context, codigo string) (int, error) {
	if err := c.asegurar(ctx); err != nil {
		return 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.dias[strings.ToLower(strings.TrimSpace(codigo))]
	if !ok {
		return 0, models.Upstream("catalogo", fmt.Errorf("dia de semana desconocido: %q", codigo))
	}
	return id, nil
}

// DiaSemanaDesdeID hace el camino inverso.
func (c *Catalogo) DiaSemanaDesdeID(ctx context.Context, id int) (string, error) {
	if err := c.asegurar(ctx); err != nil {
		return "", err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	nombre, ok := c.diasR[id]
	if !ok {
		return "", models.Upstream("catalogo", fmt.Errorf("id_dia_semana %d no esta en el catalogo", id))
	}
	return nombre, nil
}
