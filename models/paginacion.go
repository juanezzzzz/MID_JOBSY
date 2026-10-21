package models

// Pagina es el envelope de listados que espera el front (Paginated<T>).
//
// Ojo: los upstream devuelven un array pelado, sin total. Para poder informar
// Total y TotalPaginas el mid tiene que pedir limit+1 y estimar, o hacer una
// segunda lectura. Ver repositorio.ContarAproximado.
type Pagina[T any] struct {
	Items        []T `json:"items"`
	Pagina       int `json:"page"`
	TamanoPagina int `json:"pageSize"`
	Total        int `json:"total"`
	TotalPaginas int `json:"totalPages"`
	// HayMas es honesto cuando Total es una estimacion: dice si existe al
	// menos una pagina mas, que es lo unico que el upstream permite saber
	// con certeza.
	HayMas bool `json:"hasMore"`
}

// NuevaPagina arma una pagina de resultados calculando el total de paginas.
// Un items nil se devuelve como lista vacia para que el JSON sea [].
func NuevaPagina[T any](items []T, pagina, tamano, total int, hayMas bool) Pagina[T] {
	if items == nil {
		items = []T{}
	}
	totalPags := 0
	if tamano > 0 {
		totalPags = (total + tamano - 1) / tamano
	}
	return Pagina[T]{
		Items:        items,
		Pagina:       pagina,
		TamanoPagina: tamano,
		Total:        total,
		TotalPaginas: totalPags,
		HayMas:       hayMas,
	}
}
