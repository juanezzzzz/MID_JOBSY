package services

import "testing"

// Una carpeta con tipos permitidos pero sin limite rechazaba todo ("maximo 0 MB").
func TestCadaCarpetaTieneLimite(t *testing.T) {
	for c := range tiposPermitidos {
		if limites[c] <= 0 {
			t.Errorf("la carpeta %q no tiene limite de tamano", c)
		}
	}
}
