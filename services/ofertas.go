package services

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"api_MID/models"
)

// Ofertas resuelve la vista de oferta de empleo que pinta el frontend.
//
// Una oferta del front es plana; en la base son cuatro tablas mas dos
// catalogos:
//
//	empleo.ofertas                                       el grueso de los campos
//	empleo.tamano_inmueble                               tamano + gama (esLujo)
//	catalogo.modalidades_trabajo                         modalidad
//	empleo.TR_oferta_servicios + empleo.especialidades   las etiquetas
//	empleo.oferta_fotos                                  la imagen principal
type Ofertas struct {
	empleo   *ClienteUpstream
	catalogo *Catalogo
}

// NuevoOfertas crea el servicio de ofertas sobre Empleo_API y el catalogo.
func NuevoOfertas(empleo *ClienteUpstream, cat *Catalogo) *Ofertas {
	return &Ofertas{empleo: empleo, catalogo: cat}
}

// filaOferta refleja empleo.ofertas tal como lo serializa Beego.
//
// Los campos declarados con rel(fk) llegan como objeto anidado, no como
// entero; los declarados como int plano llegan como numero. La mezcla no sigue
// ningun criterio, asi que cada campo se tipa segun lo que declara el modelo
// Go del upstream.
type filaOferta struct {
	Id                 int         `json:"Id"`
	IdEmpleador        int         `json:"IdEmpleador"`
	Titulo             string      `json:"Titulo"`
	Descripcion        string      `json:"Descripcion"`
	IdTipoInmueble     *refAnidada `json:"IdTipoInmueble"`
	IdTamanoInmueble   *refAnidada `json:"IdTamanoInmueble"`
	Barrio             string      `json:"Barrio"`
	Direccion          string      `json:"Direccion"`
	Ciudad             string      `json:"Ciudad"`
	TarifaHora         float64     `json:"TarifaHora"`
	HorarioDescripcion string      `json:"HorarioDescripcion"`
	IdModalidadTrabajo int         `json:"IdModalidadTrabajo"`
	IdEstado           int         `json:"IdEstado"`
	Destacada          bool        `json:"Destacada"`
	Nueva              bool        `json:"Nueva"`
	Activo             bool        `json:"Activo"`
	CreadoEn           string      `json:"CreadoEn"`
}

type filaOfertaServicio struct {
	IdOfertas        refAnidada `json:"IdOfertas"`
	IdEspecialidades refAnidada `json:"IdEspecialidades"`
}

type filaOfertaFoto struct {
	Id          int        `json:"Id"`
	IdOfertas   refAnidada `json:"IdOfertas"`
	UrlFoto     string     `json:"UrlFoto"`
	Orden       int        `json:"Orden"`
	EsPrincipal bool       `json:"EsPrincipal"`
	Activo      bool       `json:"Activo"`
}

// aDominio traduce una fila cruda al tipo que espera el frontend.
//
// Aqui se deshace la mezcla de tamano y gama: la base guarda 'lujoso' como un
// cuarto tamano, y el front separa magnitud (3 valores) de gama (un booleano).
func (s *Ofertas) aDominio(ctx context.Context, f filaOferta) (models.Oferta, error) {
	o := models.Oferta{
		ID:               strconv.Itoa(f.Id),
		Titulo:           f.Titulo,
		Descripcion:      f.Descripcion,
		PrecioHora:       f.TarifaHora,
		Moneda:           "COP",
		Horario:          f.HorarioDescripcion,
		Direccion:        f.Direccion,
		Ciudad:           f.Ciudad,
		Etiquetas:        []string{},
		EsNuevo:          f.Nueva,
		EmpleadorID:      strconv.Itoa(f.IdEmpleador),
		FechaPublicacion: f.CreadoEn,
	}

	if f.IdTamanoInmueble != nil {
		tam, esLujo, err := s.catalogo.TamanoDesdeID(ctx, f.IdTamanoInmueble.Id)
		if err != nil {
			return o, err
		}
		o.Tamano, o.EsLujo = tam, esLujo
	}

	mod, err := s.catalogo.ModalidadDesdeID(ctx, f.IdModalidadTrabajo)
	if err != nil {
		return o, err
	}
	o.Modalidad = mod

	if est, err := s.catalogo.EstadoDesdeID(ctx, f.IdEstado); err == nil {
		o.Estado = est
	}
	return o, nil
}

// PorID devuelve una oferta con sus etiquetas y su foto principal.
func (s *Ofertas) PorID(ctx context.Context, idOferta string) (*models.Oferta, error) {
	id, err := strconv.Atoi(strings.TrimSpace(idOferta))
	if err != nil || id <= 0 {
		return nil, models.Validacion("Identificador de oferta invalido.")
	}

	var filas []filaOferta
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Id": strconv.Itoa(id)}))
	o.Query.Set("limit", "1")
	if err := s.empleo.Get(ctx, "ofertas", o, &filas); err != nil {
		return nil, err
	}
	if len(filas) == 0 || !filas[0].Activo {
		return nil, models.NoEncontrado("esa oferta")
	}

	oferta, err := s.aDominio(ctx, filas[0])
	if err != nil {
		return nil, err
	}

	// Etiquetas y foto son independientes: van en paralelo. Si alguna falla se
	// devuelve la oferta sin ese adorno en vez de un 502: son datos
	// secundarios y la ficha se pinta igual.
	var (
		etiquetas []string
		foto      string
	)
	_ = EnParalelo(
		func() error {
			if e, err := s.etiquetasDe(ctx, id); err == nil {
				etiquetas = e
			}
			return nil
		},
		func() error {
			if u, err := s.fotoPrincipalDe(ctx, id); err == nil {
				foto = u
			}
			return nil
		},
	)
	if etiquetas != nil {
		oferta.Etiquetas = etiquetas
	}
	oferta.ImagenURL = foto

	return &oferta, nil
}

// Listar devuelve ofertas paginadas aplicando los filtros del buscador.
func (s *Ofertas) Listar(ctx context.Context, f models.FiltrosOferta) (models.Pagina[models.Oferta], error) {
	var vacia models.Pagina[models.Oferta]

	pagina, tamanoPag := f.Pagina, f.TamanoPag
	if pagina < 1 {
		pagina = 1
	}
	if tamanoPag < 1 || tamanoPag > 100 {
		tamanoPag = 12
	}

	// El CRUD de Beego solo admite igualdad por campo: no hay rangos ni OR.
	// El precio, el texto libre y los filtros multivalor se aplican aqui.
	filtros := map[string]string{"Activo": "true"}
	if c := strings.TrimSpace(f.Ciudad); c != "" {
		filtros["Ciudad"] = c
	}
	// Solo las ofertas vigentes. El buscador filtraba unicamente por Activo,
	// asi que mostraba tambien las pausadas, las cerradas y las borradas: los
	// cuatro estados que el catalogo distingue no se usaban para nada.
	if idActiva, err := s.catalogo.IDEstado(ctx, "oferta", "activa"); err == nil {
		filtros["IdEstado"] = strconv.Itoa(idActiva)
	}

	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(filtros))
	o.Query.Set("limit", strconv.Itoa(tamanoPag+1))
	o.Query.Set("offset", strconv.Itoa((pagina-1)*tamanoPag))
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")

	var filas []filaOferta
	if err := s.empleo.Get(ctx, "ofertas", o, &filas); err != nil {
		return vacia, err
	}

	hayMas := len(filas) > tamanoPag
	if hayMas {
		filas = filas[:tamanoPag]
	}

	items := make([]models.Oferta, 0, len(filas))
	descartadas := 0
	for _, fila := range filas {
		of, err := s.aDominio(ctx, fila)
		if err != nil {
			// Una oferta con un id de catalogo que no se puede traducir no
			// debe tumbar el listado entero. Pero omitirla en silencio hace
			// que el total no cuadre y nadie sepa por que, asi que queda
			// registrada: casi siempre significa que al catalogo le falta una
			// fila, o que la cache es anterior a ella.
			slog.Warn("oferta descartada del listado",
				"oferta", fila.Id, "titulo", fila.Titulo, "motivo", err)
			descartadas++
			continue
		}
		if !coincide(of, f) {
			continue
		}
		items = append(items, of)
	}
	if descartadas > 0 {
		slog.Warn("el listado de ofertas omitio filas por catalogo incompleto",
			"descartadas", descartadas, "devueltas", len(items))
	}

	total := (pagina-1)*tamanoPag + len(items)
	return models.NuevaPagina(items, pagina, tamanoPag, total, hayMas), nil
}

// coincide aplica los filtros que el backend no sabe resolver.
func coincide(o models.Oferta, f models.FiltrosOferta) bool {
	if f.PrecioMin > 0 && o.PrecioHora < f.PrecioMin {
		return false
	}
	if f.PrecioMax > 0 && o.PrecioHora > f.PrecioMax {
		return false
	}
	if len(f.Tamano) > 0 && !contiene(f.Tamano, o.Tamano) {
		return false
	}
	if len(f.Modalidad) > 0 && !contiene(f.Modalidad, o.Modalidad) {
		return false
	}
	if b := strings.TrimSpace(strings.ToLower(f.Busqueda)); b != "" {
		if !strings.Contains(strings.ToLower(o.Titulo), b) &&
			!strings.Contains(strings.ToLower(o.Direccion), b) &&
			!strings.Contains(strings.ToLower(o.Ciudad), b) {
			return false
		}
	}
	return true
}

// contiene indica si x esta en xs.
func contiene[T comparable](xs []T, x T) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// Destacadas alimenta el carrusel de la portada.
func (s *Ofertas) Destacadas(ctx context.Context, limite int) ([]models.Oferta, error) {
	if limite < 1 || limite > 50 {
		limite = 6
	}

	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"Activo": "true", "Destacada": "true"}))
	o.Query.Set("limit", strconv.Itoa(limite))
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")

	var filas []filaOferta
	if err := s.empleo.Get(ctx, "ofertas", o, &filas); err != nil {
		return nil, err
	}

	items := make([]models.Oferta, 0, len(filas))
	for _, fila := range filas {
		if of, err := s.aDominio(ctx, fila); err == nil {
			items = append(items, of)
		}
	}
	return items, nil
}

// etiquetasDe resuelve la relacion N:M entre oferta y especialidades.
func (s *Ofertas) etiquetasDe(ctx context.Context, idOferta int) ([]string, error) {
	var enlaces []filaOfertaServicio
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdOfertas": strconv.Itoa(idOferta)}))
	o.Query.Set("limit", "50")
	if err := s.empleo.Get(ctx, "tr_oferta_servicios", o, &enlaces); err != nil {
		return nil, err
	}
	if len(enlaces) == 0 {
		return []string{}, nil
	}

	// El catalogo de especialidades es pequeno: una lectura y se cruza en
	// memoria. Pedir una por id seria N+1.
	var todas []filaEspecialidad
	oc := &OpcionesUpstream{Query: map[string][]string{}}
	oc.Query.Set("limit", "500")
	if err := s.empleo.Get(ctx, "especialidades", oc, &todas); err != nil {
		return nil, err
	}

	porID := make(map[int]string, len(todas))
	for _, e := range todas {
		porID[e.Id] = e.Nombre
	}

	out := make([]string, 0, len(enlaces))
	for _, e := range enlaces {
		if n, ok := porID[e.IdEspecialidades.Id]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}

// fotoPrincipalDe devuelve la url de la foto marcada como principal, o la
// primera que haya.
func (s *Ofertas) fotoPrincipalDe(ctx context.Context, idOferta int) (string, error) {
	var fotos []filaOfertaFoto
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdOfertas": strconv.Itoa(idOferta)}))
	o.Query.Set("limit", "20")
	if err := s.empleo.Get(ctx, "oferta_fotos", o, &fotos); err != nil {
		return "", err
	}
	for _, f := range fotos {
		if f.EsPrincipal {
			return f.UrlFoto, nil
		}
	}
	if len(fotos) > 0 {
		return fotos[0].UrlFoto, nil
	}
	return "", nil
}

// Crear publica una oferta de empleo.
//
// El frontend captura la descripcion y hoy la descarta; aqui si se persiste.
// El estado inicial y la expiracion los fija el middleware: el backend no
// tiene ningun concepto de oferta vigente.
func (s *Ofertas) Crear(ctx context.Context, idEmpleador string, o models.Oferta) (*models.Oferta, error) {
	empleador, err := strconv.Atoi(strings.TrimSpace(idEmpleador))
	if err != nil || empleador <= 0 {
		return nil, models.Validacion("Identificador de empleador invalido.")
	}
	if strings.TrimSpace(o.Titulo) == "" {
		return nil, models.Validacion("El titulo es obligatorio.").ConCampo("titulo", "Obligatorio.")
	}
	if strings.TrimSpace(o.Direccion) == "" {
		return nil, models.Validacion("La direccion es obligatoria.").ConCampo("direccion", "Obligatoria.")
	}
	if strings.TrimSpace(o.Ciudad) == "" {
		return nil, models.Validacion("La ciudad es obligatoria.").ConCampo("ciudad", "Obligatoria.")
	}
	if o.PrecioHora <= 0 {
		return nil, models.Validacion("La tarifa por hora debe ser mayor que cero.").
			ConCampo("precioHora", "Mayor que cero.")
	}

	idTamano, err := s.catalogo.IDTamano(ctx, o.Tamano, o.EsLujo)
	if err != nil {
		return nil, err
	}
	idModalidad, err := s.catalogo.IDModalidad(ctx, o.Modalidad)
	if err != nil {
		return nil, err
	}
	idEstado, err := s.catalogo.IDEstado(ctx, "oferta", "activa")
	if err != nil {
		return nil, err
	}

	// tipo_inmueble y tipo_mascota son obligatorios en la tabla pero el
	// formulario del frontend no los pide: se toma el primero disponible.
	idTipoInmueble, err := s.primerTipoInmueble(ctx)
	if err != nil {
		return nil, err
	}
	idMascota, err := s.catalogo.IDTipo(ctx, "mascota", "ninguna")
	if err != nil {
		return nil, err
	}

	cuerpo := map[string]any{
		"IdEmpleador":        empleador,
		"Titulo":             strings.TrimSpace(o.Titulo),
		"Descripcion":        strings.TrimSpace(o.Descripcion),
		"IdTipoInmueble":     map[string]any{"Id": idTipoInmueble},
		"IdTamanoInmueble":   map[string]any{"Id": idTamano},
		"NumHabitaciones":    0,
		"NumBanos":           0,
		"Barrio":             "",
		"Direccion":          strings.TrimSpace(o.Direccion),
		"Ciudad":             strings.TrimSpace(o.Ciudad),
		"TarifaHora":         o.PrecioHora,
		"TarifaJornada":      0,
		"HorarioDescripcion": strings.TrimSpace(o.Horario),
		"IdModalidadTrabajo": idModalidad,
		"InsumosCliente":     true,
		"Vigilancia":         false,
		"IdTipoMascota":      idMascota,
		"CondicionesExtra":   "",
		"IdEstado":           idEstado,
		"Destacada":          false,
		"Nueva":              true,
		"Vistas":             0,
		"Activo":             true,
		"ExpiraEn":           time.Now().AddDate(0, 1, 0).UTC().Format(time.RFC3339),
	}

	var creada filaOferta
	if err := s.empleo.Post(ctx, "ofertas", cuerpo, &creada); err != nil {
		return nil, err
	}
	if creada.Id == 0 {
		return nil, models.Upstream("empleo", errAltaSinID)
	}

	// Las tareas que marca el empleador ("Limpieza", "Cocina"...) se enlazan
	// con el catalogo de especialidades. Antes llegaban y se descartaban, y la
	// ficha de la oferta salia sin tareas.
	etiquetas := s.enlazarEtiquetas(ctx, creada.Id, o.Etiquetas)

	res, err := s.aDominio(ctx, creada)
	if err != nil {
		// El alta funciono; si la traduccion falla se devuelve lo enviado.
		o.ID = strconv.Itoa(creada.Id)
		o.EmpleadorID = idEmpleador
		return &o, nil
	}
	res.Etiquetas = etiquetas
	return &res, nil
}

// enlazarEtiquetas crea los enlaces oferta-especialidad y devuelve las que
// quedaron. Las que no estan en el catalogo se registran y se omiten: la
// oferta ya esta publicada y no debe fallar por una etiqueta.
func (s *Ofertas) enlazarEtiquetas(ctx context.Context, idOferta int, nombres []string) []string {
	enlazadas := []string{}
	if len(nombres) == 0 {
		return enlazadas
	}
	var catalogo []filaNombre
	if err := s.empleo.Get(ctx, "especialidades", sinLimite(), &catalogo); err != nil {
		slog.Warn("no se pudieron leer las especialidades", "oferta", idOferta, "error", err)
		return enlazadas
	}
	nombrePorID := make(map[int]string, len(catalogo))
	for _, c := range catalogo {
		nombrePorID[c.Id] = c.Nombre
	}
	vistas := map[int]bool{}
	for _, n := range nombres {
		id := idEspecialidad(catalogo, n)
		if id == 0 || vistas[id] {
			if id == 0 {
				slog.Warn("etiqueta de oferta sin especialidad", "oferta", idOferta, "etiqueta", n)
			}
			continue
		}
		vistas[id] = true
		var creado map[string]any
		if err := s.empleo.Post(ctx, "tr_oferta_servicios", map[string]any{
			"IdOfertas":        map[string]any{"Id": idOferta},
			"IdEspecialidades": map[string]any{"Id": id},
			"Activo":           true,
		}, &creado); err != nil {
			slog.Warn("no se pudo enlazar la etiqueta", "oferta", idOferta, "etiqueta", n, "error", err)
			continue
		}
		enlazadas = append(enlazadas, nombrePorID[id])
	}
	return enlazadas
}

// primerTipoInmueble devuelve el id del primer tipo de inmueble activo, que se
// usa porque la tabla lo exige y el formulario del front no lo pide.
func (s *Ofertas) primerTipoInmueble(ctx context.Context) (int, error) {
	var filas []struct {
		Id     int    `json:"Id"`
		Activo bool   `json:"Activo"`
		Nombre string `json:"Nombre"`
	}
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("limit", "50")
	if err := s.empleo.Get(ctx, "tipo_inmueble", o, &filas); err != nil {
		return 0, err
	}
	for _, f := range filas {
		if f.Activo {
			return f.Id, nil
		}
	}
	return 0, models.Upstream("empleo", errSinTipoInmueble)
}

type erroresOferta string

// Error implementa la interfaz error.
func (e erroresOferta) Error() string { return string(e) }

const (
	errAltaSinID       = erroresOferta("el alta de la oferta no devolvio id")
	errSinTipoInmueble = erroresOferta("empleo.tipo_inmueble esta vacia")
)
