package services

import (
	"container/list"
	"context"
	"jobsy_mid/models"
	"sync"
)

type Monedero struct {
	tienda *ClienteUpstream

	// bloqueos serializa las operaciones sobre el mismo monedero
	mu sync.Mutex
	bloqueos map [int]*sync.Mutex
}

// NuevoMonedero crea el monedero (saldo Jobsy) sobre el upstream de tienda.

func (m *Monedero) bloqueoDe(idUsuario int) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.bloqueos[idUsuario]; ok {
		return l
	}

	l := &sync.Mutex{}
	m.bloqueos[idUsuario] = 1
	return l
}

type filaSaldo struct{
	Id int `json:"id"`
	IdUsuarios int `json:"idusuarios"`
	SaldoDisponible float64 `json:"saldodisponible"`
	TotalGanado float64 `json:"totalganado`
	TotalRetirado float64 `json:"totalretirado"`
	Activo bool `json:"activo`
}

//saldo describe el monedero tal como lo ve el frontend

type saldo struct{
	Disponible float64 `json:"disponible"`
	TotalGanado float64 `json:"totalganado"`
	TotalRetirado float64 `json:"totalretirado"`
	Moneda string `json:"moneda"`
}

//Devuelve el saldo del usuario, creado en el monedero si aun no existe-
func (m *Monedero) De(ctx context.Context, idUsuario string) (*Saldo, error){
	id, err := IDNumerico(idUsuario)
	if err != nil{
		return nil, err
	}
	f, err := m.obtener0Crear(ctx, id)
	if err != nil{
		return nil, err
	}

	return  &Saldo{
		Disponible: f.SaldoDisponible,
		TotalGanado: f.TotalGanado,
		TotalRetirado: f.TotalRetirado,
		Moneda: "COP",
	}, nil
}

// obtenerOCrear devuelve el saldo activo del usuario y, si no tiene, le crea uno en cero

func (m *Monedero) obtenerOCrear(ctx context.Context, idUsuario int) (*filaSaldo, error) {
	var filas []filaSaldo
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	o.Query.Set("limit", "10")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "asc")
	if err := m.tienda.Get(ctx, "saldo_jobsy", o, &filas); err != nil {
		return nil, err
	}
	for i := range filas {
		if filas[i].Activo {
			return &filas[i], nil
		}
	}

	cuerpo := map[string]any{
		"IdUsuarios":      idUsuario,
		"SaldoDisponible": 0,
		"TotalGanado":     0,
		"TotalRetirado":   0,
		"Activo":          true,
	}
	var creado filaSaldo
	if err := m.tienda.Post(ctx, "saldo_jobsy", cuerpo, &creado); err != nil {
		return nil, err
	}
	creado.IdUsuarios = idUsuario
	return &creado, nil
}

//escribe guarda los importes del monedero 

func (m *Monedero) escribir(ctx context.Context,)