// Autenticacion: implementa la autenticacion que Jobsy_API no tiene.
//
// Los 7 servicios no emiten ni validan tokens, no hashean contrasenas y no
// consultan roles. core.credenciales tiene columnas password_hash y salt, pero
// ningun codigo Go las escribe. El endpoint POST /v1/sesiones es el CRUD
// generico: inserta la fila con el token que le mandes, sin firmar nada.
//
// Por eso el mid es la unica frontera de seguridad del sistema, y los 7
// servicios tienen que quedar en red privada. Si quedan expuestos,
// GET /v1/credenciales entrega todos los hashes y GET /v1/sesiones todos los
// tokens activos.

package services

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"api_MID/models"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// costeBcrypt por encima del default (10). Es un login, no un bucle caliente.
const costeBcrypt = 12

// Claims es lo que viaja firmado en el token.
//
// El rol va dentro del token, firmado. Hoy el front guarda el objeto User
// entero en localStorage y roleGuard lee el rol de ahi, asi que cualquiera
// puede abrir DevTools, cambiar "empleado" por "admin" y entrar al panel. El
// guard del front sigue siendo util para la navegacion, pero la decision real
// se toma aqui, contra el token firmado.
type Claims struct {
	UsuarioID string     `json:"uid"`
	Email     string     `json:"email"`
	Rol       models.Rol `json:"rol"`
	jwt.RegisteredClaims
}

// ServicioAuth emite y valida los JWT, y hashea/verifica contrasenas. Es la
// pieza de seguridad que los upstream no tienen.
type ServicioAuth struct {
	secreto  []byte
	duracion time.Duration
	emisor   string
}

// NuevoServicioAuth crea el servicio de autenticacion con el secreto de firma y
// la duracion de validez del token.
func NuevoServicioAuth(secreto []byte, duracion time.Duration) *ServicioAuth {
	return &ServicioAuth{secreto: secreto, duracion: duracion, emisor: "jobsy-mid"}
}

// Hash genera el hash de una contrasena nueva.
func (s *ServicioAuth) Hash(password string) (string, error) {
	if err := ValidarPassword(password); err != nil {
		return "", err
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), costeBcrypt)
	if err != nil {
		return "", models.Interno(fmt.Errorf("generando hash: %w", err))
	}
	return string(b), nil
}

// Verificar compara una contrasena contra su hash.
//
// Devuelve siempre el mismo error para "no existe el usuario" y "contrasena
// incorrecta": distinguirlos permite enumerar cuentas registradas.
func (s *ServicioAuth) Verificar(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return models.NoAutorizado("Correo o contrasena incorrectos.")
	}
	return nil
}

// HashFalso quema tiempo cuando el usuario no existe, para que el atacante no
// distinga por latencia una cuenta inexistente de una contrasena mala.
func (s *ServicioAuth) HashFalso(password string) {
	_, _ = bcrypt.GenerateFromPassword([]byte(password), costeBcrypt)
}

// Emitir firma un token para un usuario.
func (s *ServicioAuth) Emitir(u models.Usuario) (string, time.Time, error) {
	ahora := time.Now()
	expira := ahora.Add(s.duracion)

	claims := Claims{
		UsuarioID: u.ID,
		Email:     u.Email,
		Rol:       u.Rol,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			Issuer:    s.emisor,
			IssuedAt:  jwt.NewNumericDate(ahora),
			NotBefore: jwt.NewNumericDate(ahora),
			ExpiresAt: jwt.NewNumericDate(expira),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	firmado, err := token.SignedString(s.secreto)
	if err != nil {
		return "", time.Time{}, models.Interno(fmt.Errorf("firmando token: %w", err))
	}
	return firmado, expira, nil
}

// Validar verifica la firma y la vigencia del token.
func (s *ServicioAuth) Validar(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		// Sin este chequeo, un atacante puede cambiar alg a "none" o a RS256 y
		// colar un token que nosotros validariamos con la clave equivocada.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("algoritmo de firma inesperado: %v", t.Header["alg"])
		}
		return s.secreto, nil
	}, jwt.WithIssuer(s.emisor), jwt.WithValidMethods([]string{"HS256"}))

	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, models.NoAutorizado("Tu sesion expiro. Inicia sesion de nuevo.")
		default:
			return nil, models.NoAutorizado("Token invalido.")
		}
	}
	if !token.Valid {
		return nil, models.NoAutorizado("Token invalido.")
	}
	if claims.UsuarioID == "" {
		return nil, models.NoAutorizado("Token sin sujeto.")
	}
	return claims, nil
}

// TokenDesdeCabecera extrae el token de "Authorization: Bearer xxx".
func TokenDesdeCabecera(cabecera string) (string, error) {
	if cabecera == "" {
		return "", models.NoAutorizado("")
	}
	partes := strings.SplitN(cabecera, " ", 2)
	if len(partes) != 2 || !strings.EqualFold(partes[0], "Bearer") {
		return "", models.NoAutorizado("Cabecera Authorization mal formada.")
	}
	token := strings.TrimSpace(partes[1])
	if token == "" {
		return "", models.NoAutorizado("Cabecera Authorization sin token.")
	}
	return token, nil
}

// ---- Validaciones ----

// ValidarPassword aplica una politica minima. Los upstream no validan nada:
// aceptan un usuario con correo vacio.
func ValidarPassword(p string) error {
	if len(p) < 8 {
		return models.Validacion("La contrasena debe tener al menos 8 caracteres.").
			ConCampo("password", "Minimo 8 caracteres.")
	}
	if len(p) > 72 {
		// bcrypt trunca silenciosamente a 72 bytes: mas alla, dos contrasenas
		// distintas validan igual. Preferimos rechazar a mentir.
		return models.Validacion("La contrasena no puede superar 72 caracteres.").
			ConCampo("password", "Maximo 72 caracteres.")
	}
	var tieneLetra, tieneNumero bool
	for _, r := range p {
		switch {
		case r >= '0' && r <= '9':
			tieneNumero = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			tieneLetra = true
		}
	}
	if !tieneLetra || !tieneNumero {
		return models.Validacion("La contrasena debe combinar letras y numeros.").
			ConCampo("password", "Usa al menos una letra y un numero.")
	}
	return nil
}

// ValidarEmail hace una comprobacion estructural, no de existencia.
func ValidarEmail(e string) error {
	e = strings.TrimSpace(e)
	if e == "" {
		return models.Validacion("El correo es obligatorio.").ConCampo("email", "Obligatorio.")
	}
	arroba := strings.IndexByte(e, '@')
	if arroba <= 0 || arroba == len(e)-1 {
		return models.Validacion("El correo no tiene un formato valido.").ConCampo("email", "Formato invalido.")
	}
	host := e[arroba+1:]
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return models.Validacion("El correo no tiene un formato valido.").ConCampo("email", "Formato invalido.")
	}
	if strings.Contains(e, " ") {
		return models.Validacion("El correo no puede contener espacios.").ConCampo("email", "Sin espacios.")
	}
	return nil
}

// ---- Contexto ----

type claveCtx struct{}

// ConClaims guarda los claims del token en el contexto de la peticion, para que
// los controladores los recuperen con ClaimsDesdeCtx.
func ConClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claveCtx{}, c)
}

// ClaimsDesdeCtx recupera los claims que el middleware dejo en el contexto.
func ClaimsDesdeCtx(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claveCtx{}).(*Claims)
	return c, ok
}

// ExigirPropietario comprueba que el usuario autenticado sea el dueno del
// recurso, o un admin.
//
// Esto es imprescindible: los upstream obedecen cualquier id_usuarios que se
// les pase, asi que sin esta comprobacion cualquiera lee la hoja de vida o los
// pedidos de otro con solo cambiar el id en la URL.
func ExigirPropietario(ctx context.Context, idRecurso string) error {
	c, ok := ClaimsDesdeCtx(ctx)
	if !ok {
		return models.NoAutorizado("")
	}
	if c.Rol == models.RolAdmin {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(c.UsuarioID), []byte(idRecurso)) != 1 {
		return models.Prohibido("")
	}
	return nil
}

// ExigirRol comprueba que el usuario tenga alguno de los roles dados.
func ExigirRol(ctx context.Context, roles ...models.Rol) error {
	c, ok := ClaimsDesdeCtx(ctx)
	if !ok {
		return models.NoAutorizado("")
	}
	for _, r := range roles {
		if c.Rol == r {
			return nil
		}
	}
	return models.Prohibido("")
}

// IDNumerico convierte el id string del front al entero que usa PostgreSQL.
func IDNumerico(id string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil || n <= 0 {
		return 0, models.Validacion("Identificador invalido.")
	}
	return n, nil
}
