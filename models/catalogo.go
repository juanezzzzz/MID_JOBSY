package models

// ItemCatalogo es una fila de cualquiera de las tablas del esquema catalogo
// (estados, tipos, modalidades_trabajo, dias_semana, generos...).
//
// Catalogo_api no tiene tags json en sus modelos, asi que los campos llegan
// como se llaman en Go (Id, Nombre, Contexto...). encoding/json compara sin
// importar mayusculas, por eso este mismo struct sirve para leer del CRUD y
// para responderle al front en minuscula.
type ItemCatalogo struct {
	Id             int    `json:"id"`
	Contexto       string `json:"contexto,omitempty"`
	Nombre         string `json:"nombre"`
	NombreCompleto string `json:"nombreCompleto,omitempty"`
	Descripcion    string `json:"descripcion,omitempty"`
	Orden          int    `json:"orden,omitempty"`
	Activo         bool   `json:"activo"`
}
