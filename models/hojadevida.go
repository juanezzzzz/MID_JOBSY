
package models

type Experiencia struct {
	Cargo               string `json:"cargo"`
	Empresa             string `json:"empresa"`
	Desde               string `json:"desde"`
	Hasta               string `json:"hasta,omitempty"`
	Descripcion         string `json:"descripcion"`
}

type Educacion struct {
	Titulo             string `json:"titulo"`
	Institucion        string `json:"institucion"`
	Ano                string `json:"ano"`
}

// HojaVida calca el tipo Resume.models.ts.
//
// Educacion y Habilidades no tiene tabla en la base: solo existen
// empleo.hojas_vida y empleo.hv_experiencias. Se exponen para no romper el
// contrato del frontend, pero hoy viajan siempre vacias.
type HojaVida struct {
	ID                            string          `json:"id"`
	UsuarioID                     string          `json:"userId"`
	ResumenProfesional            string          `json:"resumenProfesional"`
	Experiencias                  []Experiencia   `json:"experiencias"`
	Educacion                     []Educacion     `json:"educacion"`
	Habilidades                   []string        `json:"habilidaes"`
	Certificaciones               []string        `json:"certificaciones"`
	ArchivoURL                    string          `json:"ArchivoURL,omitempty"`
}
