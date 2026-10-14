package models

type Pagina[T any] struct {
	Items []T `json:"items"`
	Pagina int `json:"page"`
	TamanoPagina int `json:"pageSize"`
	Total int `json:"total"`
	TotalPaginas int `json:"totalPages"`
	HayMas bool `json:"hasMore"`
}

func NuevaPagina[T any](items []T, pagina, tamano, total int, hayMas bool) Pagina[T] {
	if items == nil {
		items = []T{}
	}
	totalPags := 0
	if tamano > 0 {
		totalPags = (total + tamano - 1) / tamano
	}
	return Pagina[T]{
		Items: items,
		Pagina: pagina,
		TamanoPagina: tamano,
		Total: total,
		TotalPaginas: totalPags,
		HayMas: hayMas,
	}
}