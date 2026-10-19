package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Verificacion de la notificacion asincrona de una pasarela.
//
// Un proveedor real no confirma el cobro por la pantalla del usuario: llama a
// una URL nuestra cuando el dinero se mueve. Esa llamada no trae sesion, asi
// que lo unico que la distingue de la de un desconocido es la firma.
//
// Sin esto, cualquiera que supiera el identificador de un pago podia darlo por
// aprobado con una peticion. Por eso la ruta existia pero no se podia exponer.

// ErrFirmaInvalida cubre los tres motivos por los que se rechaza una
// notificacion. Se devuelve el mismo error en los tres casos a proposito: al
// que llama no se le dice si acerto el formato, la antiguedad o la clave.
var ErrFirmaInvalida = errors.New("la firma de la notificacion no es valida")

// ErrSinSecreto indica que no hay clave configurada, asi que no se puede
// verificar nada y la ruta no debe atender.
var ErrSinSecreto = errors.New("no hay secreto configurado para verificar notificaciones")

// ToleranciaFirma es la antiguedad maxima que se acepta.
//
// Sin ventana, una notificacion valida capturada por alguien podria reenviarse
// mas tarde. Cinco minutos es lo que usan los proveedores habituales y deja
// margen para un reloj algo desviado.
const ToleranciaFirma = 5 * time.Minute

// VerificarFirma comprueba la cabecera de firma de una notificacion.
//
// El formato es el mas extendido entre proveedores:
//
//	t=<unix>,v1=<hmac hex>
//
// y lo firmado es "<unix>.<cuerpo>" con HMAC-SHA256 y la clave compartida. Se
// firma el cuerpo EXACTO recibido, sin volver a serializarlo: cualquier
// reordenacion de claves daria otra firma.
func VerificarFirma(cabecera string, cuerpo []byte, secreto string, ahora time.Time) error {
	if strings.TrimSpace(secreto) == "" {
		return ErrSinSecreto
	}

	marca, firma, err := partirCabecera(cabecera)
	if err != nil {
		return err
	}

	// La ventana se comprueba en los dos sentidos: una marca en el futuro
	// tambien es sospechosa.
	edad := ahora.Sub(time.Unix(marca, 0))
	if edad < 0 {
		edad = -edad
	}
	if edad > ToleranciaFirma {
		return ErrFirmaInvalida
	}

	mac := hmac.New(sha256.New, []byte(secreto))
	fmt.Fprintf(mac, "%d.", marca)
	mac.Write(cuerpo)
	esperada := mac.Sum(nil)

	recibida, err := hex.DecodeString(firma)
	if err != nil {
		return ErrFirmaInvalida
	}
	// Comparacion en tiempo constante: con == se podria averiguar la firma
	// correcta byte a byte midiendo lo que tarda en responder.
	if !hmac.Equal(esperada, recibida) {
		return ErrFirmaInvalida
	}
	return nil
}

// partirCabecera extrae la marca de tiempo (t) y la firma (v1) de la cabecera
// "t=<unix>,v1=<hmac hex>"; devuelve ErrFirmaInvalida si falta alguna.
func partirCabecera(cabecera string) (int64, string, error) {
	var marca int64
	var firma string
	for _, parte := range strings.Split(cabecera, ",") {
		clave, valor, ok := strings.Cut(strings.TrimSpace(parte), "=")
		if !ok {
			continue
		}
		switch clave {
		case "t":
			n, err := strconv.ParseInt(valor, 10, 64)
			if err != nil {
				return 0, "", ErrFirmaInvalida
			}
			marca = n
		case "v1":
			firma = valor
		}
	}
	if marca == 0 || firma == "" {
		return 0, "", ErrFirmaInvalida
	}
	return marca, firma, nil
}

// Firmar produce la cabecera que enviaria el proveedor.
//
// Existe para poder probar el camino completo sin depender de nadie, y para
// que quien configure un proveedor real pueda comprobar que su clave coincide.
func Firmar(cuerpo []byte, secreto string, ahora time.Time) string {
	marca := ahora.Unix()
	mac := hmac.New(sha256.New, []byte(secreto))
	fmt.Fprintf(mac, "%d.", marca)
	mac.Write(cuerpo)
	return fmt.Sprintf("t=%d,v1=%s", marca, hex.EncodeToString(mac.Sum(nil)))
}
