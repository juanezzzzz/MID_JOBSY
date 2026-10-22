package models

type Rol string

const (
	RolEmpleador Rol = "empleador"
	RolEmpleado  Rol = "empleado"
	RolAdmin     Rol = "admin"
)

// Valido indica si el rol es uno de los que maneja la aplicacion.
func (r Rol) Valido() bool {
	return r == RolEmpleador || r == RolEmpleado || r == RolAdmin
}

type Usuario struct {
	ID            string `json:"id"`
	Nombre        string `json:"nombre"`
	Apellido      string `json:"apellido"`
	Email         string `json:"email"`
	Telefono      string `json:"telefono,omitempty"`
	Rol           Rol    `json:"rol"`
	FotoURL       string `json:"fotoUrl,omitempty"`
	Verificado    bool   `json:"verificado"`
	Ciudad        string `json:"ciudad,omitempty"`
	Departamento  string `json:"departamento,omitempty"`
	FechaRegistro string `json:"fechaRegistro"`
}

// PeticionLogin usa "usuario" y no "email" porque asi lo manda el front
// (LoginRequest en user.model.ts).
type PeticionLogin struct {
	Usuario  string `json:"usuario"`
	Password string `json:"password"`
}

type RespuestaLogin struct {
	Token   string  `json:"token"`
	Usuario Usuario `json:"user"`
}

type PeticionRegistro struct {
	Nombre   string `json:"nombre"`
	Apellido string `json:"apellido"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Rol      Rol    `json:"rol"`

	// Opcionales: lo que el asistente de registro del front pide en sus pasos.
	// Antes se descartaban y el usuario tenia que volver a escribirlos.
	Nacionalidad    string      `json:"nacionalidad,omitempty"`
	Genero          string      `json:"genero,omitempty"`
	FechaNacimiento string      `json:"fechaNacimiento,omitempty"` // YYYY-MM-DD
	Documento       string      `json:"documento,omitempty"`
	Ciudad          string      `json:"ciudad,omitempty"`
	Barrio          string      `json:"barrio,omitempty"`
	DistanciaKm     float64     `json:"distanciaKm,omitempty"`
	Modalidades     []Modalidad `json:"modalidades,omitempty"`
	Experiencia     string      `json:"experiencia,omitempty"`
	Especialidades  []string    `json:"especialidades,omitempty"`
}

// DocumentoUsuario es un documento subido para verificar la identidad.
type DocumentoUsuario struct {
	ID         string `json:"id"`
	Tipo       string `json:"tipo"`
	ArchivoURL string `json:"archivoUrl"`
	Estado     string `json:"estado"`
	Notas      string `json:"notas,omitempty"`
	Fecha      string `json:"fecha,omitempty"`
}
