package services

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const secreto = "clave-compartida-con-el-proveedor"

// Una firma hecha con la clave correcta sobre el mismo cuerpo se acepta.
func TestFirmaValidaSeAcepta(t *testing.T) {
	cuerpo := []byte(`{"pagoId":"12","estado":"aprobado"}`)
	ahora := time.Now()
	if err := VerificarFirma(Firmar(cuerpo, secreto, ahora), cuerpo, secreto, ahora); err != nil {
		t.Fatalf("no esperaba error: %v", err)
	}
}

// Sin clave configurada no se puede verificar nada, asi que la ruta no debe
// atender: es distinto de una firma incorrecta.
func TestSinSecretoNoSeVerifica(t *testing.T) {
	cuerpo := []byte("{}")
	err := VerificarFirma("t=1,v1=aa", cuerpo, "", time.Now())
	if !errors.Is(err, ErrSinSecreto) {
		t.Fatalf("err = %v, esperaba ErrSinSecreto", err)
	}
}

// Si el cuerpo cambia despues de firmarlo, la firma deja de valer.
func TestCuerpoAlteradoSeRechaza(t *testing.T) {
	ahora := time.Now()
	original := []byte(`{"pagoId":"12","estado":"rechazado"}`)
	cabecera := Firmar(original, secreto, ahora)

	// Lo que llega dice "aprobado" pero la firma es la del cuerpo original.
	alterado := []byte(`{"pagoId":"12","estado":"aprobado"}`)
	if err := VerificarFirma(cabecera, alterado, secreto, ahora); !errors.Is(err, ErrFirmaInvalida) {
		t.Fatalf("un cuerpo alterado no puede pasar: %v", err)
	}
}

// Una firma hecha con otra clave se rechaza.
func TestOtraClaveSeRechaza(t *testing.T) {
	cuerpo := []byte("{}")
	ahora := time.Now()
	cabecera := Firmar(cuerpo, "otra-clave-distinta", ahora)
	if err := VerificarFirma(cabecera, cuerpo, secreto, ahora); !errors.Is(err, ErrFirmaInvalida) {
		t.Fatalf("una firma de otra clave no puede pasar: %v", err)
	}
}

// Una notificacion valida capturada por alguien no debe servir mas tarde.
func TestFirmaCaducadaSeRechaza(t *testing.T) {
	cuerpo := []byte("{}")
	emitida := time.Now().Add(-ToleranciaFirma - time.Minute)
	cabecera := Firmar(cuerpo, secreto, emitida)
	if err := VerificarFirma(cabecera, cuerpo, secreto, time.Now()); !errors.Is(err, ErrFirmaInvalida) {
		t.Fatalf("una firma caducada no puede pasar: %v", err)
	}
}

// Una marca en el futuro tambien es sospechosa: no se acepta.
func TestFirmaDelFuturoSeRechaza(t *testing.T) {
	cuerpo := []byte("{}")
	futura := time.Now().Add(ToleranciaFirma + time.Minute)
	cabecera := Firmar(cuerpo, secreto, futura)
	if err := VerificarFirma(cabecera, cuerpo, secreto, time.Now()); !errors.Is(err, ErrFirmaInvalida) {
		t.Fatalf("una firma del futuro no puede pasar: %v", err)
	}
}

// Cabeceras vacias, incompletas o ilegibles se rechazan.
func TestCabecerasMalFormadasSeRechazan(t *testing.T) {
	cuerpo := []byte("{}")
	for _, cab := range []string{
		"",
		"cualquier cosa",
		"t=1700000000",                 // sin firma
		"v1=aabbcc",                    // sin marca
		"t=no-es-numero,v1=aabbcc",     // marca ilegible
		"t=1700000000,v1=no-es-hex-zz", // firma ilegible
	} {
		if err := VerificarFirma(cab, cuerpo, secreto, time.Unix(1700000000, 0)); err == nil {
			t.Fatalf("la cabecera %q no deberia aceptarse", cab)
		}
	}
}

// El mismo error para los tres motivos: al que llama no se le dice si acerto
// el formato, la antiguedad o la clave.
func TestElMotivoNoSeFiltra(t *testing.T) {
	cuerpo := []byte("{}")
	ahora := time.Now()
	casos := []string{
		Firmar(cuerpo, "otra-clave", ahora),
		Firmar(cuerpo, secreto, ahora.Add(-time.Hour)),
		"t=1,v1=zz",
	}
	for _, cab := range casos {
		err := VerificarFirma(cab, cuerpo, secreto, ahora)
		if err == nil || !strings.Contains(err.Error(), "no es valida") {
			t.Fatalf("mensaje inesperado para %q: %v", cab, err)
		}
	}
}
