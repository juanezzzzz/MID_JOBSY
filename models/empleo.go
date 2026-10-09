
package models

type Tamano string

const (
	TamanoPequeno  Tamano = "pequeno"
	TamanoMediano  Tamano = "mediano"
	TamanoGrande   Tamano = "grande"
)

type Modalidad string

const (
	ModalidadPresencial        Modalidad = "presencial"
	ModalidadPorHoras          Modalidad = "por_horas"
	ModalidadJornadaCompleta   Modalidad = "jornada_completa"
	ModalidadFinDeSemana       Modalidad = "fin_de_semana"
)

type Oferta struct {
	ID				      string `json:"id"`
	Titulo			      string `json:"titulo"`
	Descripcion		      string `json:"descripcion,omitempty"`
	Tamano			      Tamano `json:"tamano"`
	EsLujo			      bool   `json:"es_lujo"`
	PrecioHora		      float64 `json:"precio_hora"`
	Moneda			      string `json:"moneda"`
	Horario			      string `json:"horario"`
	Direccion		      string `json:"direccion"`
	Ciudad			      string `json:"ciudad"`
	Modalidad		      Modalidad `json:"modalidad"`
	Etiquetas		      []string `json:"etiquetas,omitempty"`
	ImagenURL		      string `json:"imagen_url,omitempty"`
	EsNuevo		          bool   `json:"es_nuevo"`
	EmpleadorID		      string `json:"empleador_id"`
	FechaPublicacion      string `json:"fecha_publicacion"`

}

type FiltrosOferta struct {
	Tamano            []Tamano
	Modalidad         []Modalidad
	PrecioMinimo      float64
	PrecioMaximo      float64
	Ciudad            string
	Busqueda          string
	Pagina            int
	TamanoPag         int
}


type EstadoPostulacion string

const (
	PostPendiente  EstadoPostulacion = "pendiente"
	PostEnRevision EstadoPostulacion = "en_revision"
	PostAceptada   EstadoPostulacion = "aceptada"
	PostRechazada  EstadoPostulacion = "rechazada"
)


// Postulacion incluye los campos de disponibilidad que el asistente del front
// ya captura pero hoy descarta (asistente-postulacion.ts los pide y
// application.service.ts solo manda el mensaje). La tabla empleo.postulaciones
// tiene columnas para todos ellos.


type Postulacion struct {
	ID               string            `json:"id"`
	OfertaID         string            `json:"jobId"`
	EmpleadoID       string            `json:"empleadoId"`
	Estado           EstadoPostulacion `json:"estado"`
	FechaPostulacion string            `json:"fechaPostulacion"`
	Mensaje          string            `json:"mensaje,omitempty"`
	DisponibleDesde  string            `json:"disponibleDesde,omitempty"`
	Dias             []string          `json:"dias,omitempty"`
	HoraInicio       string            `json:"horaInicio,omitempty"`
	HoraFin          string            `json:"horaFin,omitempty"`
	TarifaMin        float64           `json:"tarifaMin,omitempty"`
	TarifaMax        float64           `json:"tarifaMax,omitempty"`
}

// Empleado es la vista plana que pinta el front. En la base son tres tablas:
// core.usuarios + empleo.perfiles_empleado + el agregado de empleo.resenas.
// Este tipo es el ejemplo canonico de lo que pediste: el front pide una cosa,
// el mid lee varias y las une.
type Empleado struct {
	ID             string   `json:"id"`
	UsuarioID      string   `json:"usuarioId"`
	Nombre         string   `json:"nombre"`
	FotoURL        string   `json:"fotoUrl,omitempty"`
	Descripcion    string   `json:"descripcion"`
	Calificacion   float64  `json:"calificacion"`
	TotalServicios int      `json:"totalServicios"`
	Verificado     bool     `json:"verificado"`
	Especialidades []string `json:"especialidades"`
	Experiencia    string   `json:"experiencia"`
	Ciudad         string   `json:"ciudad"`
	Disponible     bool     `json:"disponible"`

	// TarifaHora es lo que esa persona cobra. La pantalla de contratacion la
	// necesita para proponer un precio; antes partia de una cifra fija
	// escrita en el codigo.
	TarifaHora float64 `json:"tarifaHora,omitempty"`
}

type Resena struct {
	ID        string  `json:"id"`
	Autor     string  `json:"autor"`
	Estrellas float64 `json:"estrellas"`
	Texto     string  `json:"texto"`
	Fecha     string  `json:"fecha"`
}

