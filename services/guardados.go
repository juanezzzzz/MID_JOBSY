package services

import (
	"context"
	"strconv"

	"api_MID/models"
)

// Guardados gestiona los perfiles de empleados que cada usuario guarda para
// verlos despues ("Guardar perfil"). La lista vive en Usuarios_Api
// (usuarios.perfiles_guardados); el detalle de cada empleado se arma con el
// servicio de empleados, asi la lista muestra los datos al dia.
type Guardados struct {
	usuarios  *ClienteUpstream
	empleados *Empleados
}

// NuevoGuardados crea el servicio sobre Usuarios_Api y el de empleados.
func NuevoGuardados(u *ClienteUpstream, e *Empleados) *Guardados {
	return &Guardados{usuarios: u, empleados: e}
}

type filaGuardado struct {
	Id         int `json:"Id"`
	IdUsuarios int `json:"IdUsuarios"`
	IdEmpleado int `json:"IdEmpleado"`
}

// filas devuelve lo guardado por el usuario, del mas reciente al mas antiguo.
func (s *Guardados) filas(ctx context.Context, idUsuario int) ([]filaGuardado, error) {
	var filas []filaGuardado
	o := &OpcionesUpstream{Query: map[string][]string{}}
	o.Query.Set("query", QueryBeego(map[string]string{"IdUsuarios": strconv.Itoa(idUsuario)}))
	// shortcut: tope de 200 guardados por usuario, paginar si alguien llega a tantos.
	o.Query.Set("limit", "200")
	o.Query.Set("sortby", "Id")
	o.Query.Set("order", "desc")
	if err := s.usuarios.Get(ctx, "perfiles_guardados", o, &filas); err != nil {
		return nil, err
	}
	return filas, nil
}

// Mios lista los empleados guardados por el usuario. Los que ya no existen o
// dejaron de ser empleados se omiten en vez de romper la lista.
func (s *Guardados) Mios(ctx context.Context, idUsuario string) ([]models.Empleado, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	filas, err := s.filas(ctx, id)
	if err != nil {
		return nil, err
	}
	res := make([]models.Empleado, len(filas))
	ok := make([]bool, len(filas))
	fns := make([]func() error, len(filas))
	for i, f := range filas {
		fns[i] = func() error {
			if e, err := s.empleados.PorID(ctx, strconv.Itoa(f.IdEmpleado)); err == nil {
				res[i], ok[i] = *e, true
			}
			return nil
		}
	}
	_ = EnParalelo(fns...)
	out := make([]models.Empleado, 0, len(filas))
	for i := range res {
		if ok[i] {
			out = append(out, res[i])
		}
	}
	return out, nil
}

// Guardar agrega el empleado a la lista del usuario. Guardarlo dos veces no es
// error: devuelve lo mismo.
func (s *Guardados) Guardar(ctx context.Context, idUsuario, idEmpleado string) (*models.Empleado, error) {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return nil, err
	}
	// Valida que sea un empleado real: la FK solo comprueba que sea un usuario.
	emp, err := s.empleados.PorID(ctx, idEmpleado)
	if err != nil {
		return nil, err
	}
	idEmp, _ := strconv.Atoi(emp.UsuarioID)
	if idEmp == id {
		return nil, models.Validacion("No puedes guardar tu propio perfil.")
	}
	filas, err := s.filas(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, f := range filas {
		if f.IdEmpleado == idEmp {
			return emp, nil
		}
	}
	if err := s.usuarios.Post(ctx, "perfiles_guardados", map[string]any{
		"IdUsuarios": id, "IdEmpleado": idEmp, "Activo": true,
	}, nil); err != nil {
		return nil, err
	}
	return emp, nil
}

// Quitar saca el empleado de la lista. Quitar uno que no estaba no es error.
func (s *Guardados) Quitar(ctx context.Context, idUsuario, idEmpleado string) error {
	id, err := IDNumerico(idUsuario)
	if err != nil {
		return err
	}
	idEmp, err := IDNumerico(idEmpleado)
	if err != nil {
		return err
	}
	filas, err := s.filas(ctx, id)
	if err != nil {
		return err
	}
	for _, f := range filas {
		if f.IdEmpleado == idEmp {
			if err := s.usuarios.Delete(ctx, "perfiles_guardados/"+strconv.Itoa(f.Id)); err != nil {
				return err
			}
		}
	}
	return nil
}
