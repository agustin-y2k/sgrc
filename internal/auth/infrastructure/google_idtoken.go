package infrastructure

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramiro/sgrc/internal/auth/application"
)

// VerificadorGoogle valida los ID token que emite Google cuando alguien
// aprieta "Iniciar sesión con Google" en el navegador.
type VerificadorGoogle struct {
	clientID           string
	dominiosPermitidos []string
	http               *http.Client
	urlCertificados    string
	ahora              func() time.Time

	// Las claves públicas de Google rotan cada pocas horas, así que se cachean
	// con el vencimiento que declara la propia respuesta en vez de pedirlas en
	// cada login.
	mu       sync.Mutex
	claves   map[string]*rsa.PublicKey
	vencenEn time.Time
}

// urlCertificadosGoogle es el JWKS con las claves públicas con las que
// Google firma los ID token.
const urlCertificadosGoogle = "https://www.googleapis.com/oauth2/v3/certs"

// emisoresGoogle: Google usa las dos formas indistintamente, y las dos son
// válidas.
var emisoresGoogle = []string{"https://accounts.google.com", "accounts.google.com"}

// errKidDesconocido: el token viene firmado con una clave que Google no
// publica (ni siquiera después de refrescar el cache).
var errKidDesconocido = errors.New("el token está firmado con una clave que Google no publica")

var _ application.VerificadorGoogle = (*VerificadorGoogle)(nil)

// NewVerificadorGoogle construye el verificador. dominiosPermitidos vacío
// significa "cualquier cuenta de Google" — ver DominiosPermitidos.
func NewVerificadorGoogle(clientID string, dominiosPermitidos []string, ahora func() time.Time) *VerificadorGoogle {
	return &VerificadorGoogle{
		clientID:           clientID,
		dominiosPermitidos: dominiosPermitidos,
		// Timeout explícito: sin él, http.Client espera para siempre y un problema
		// de red del lado de Google dejaría requests de login colgados hasta agotar
		// el pool de conexiones.
		http:            &http.Client{Timeout: 10 * time.Second},
		urlCertificados: urlCertificadosGoogle,
		ahora:           ahora,
		claves:          map[string]*rsa.PublicKey{},
	}
}

// DominiosPermitidos parsea la lista separada por comas de
// GOOGLE_DOMINIOS_PERMITIDOS ("tuinstitucion.edu.ar, otra.edu.ar").
func DominiosPermitidos(crudo string) []string {
	var dominios []string
	for _, d := range strings.Split(crudo, ",") {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, "@") // tolera que lo escriban "@escuela.edu.ar"
		if d != "" {
			dominios = append(dominios, d)
		}
	}
	return dominios
}

// claimsGoogle son los claims del ID token que nos interesan. Google manda
// bastantes más (picture, locale, nonce…) y se ignoran.
type claimsGoogle struct {
	jwt.RegisteredClaims
	Email           string       `json:"email"`
	EmailVerificado boolFlexible `json:"email_verified"`
	Nombre          string       `json:"given_name"`
	Apellido        string       `json:"family_name"`
}

// boolFlexible acepta tanto `true` como `"true"`.
type boolFlexible bool

func (b *boolFlexible) UnmarshalJSON(datos []byte) error {
	var comoBool bool
	if err := json.Unmarshal(datos, &comoBool); err == nil {
		*b = boolFlexible(comoBool)
		return nil
	}
	var comoString string
	if err := json.Unmarshal(datos, &comoString); err != nil {
		return fmt.Errorf("email_verified no es ni booleano ni string: %s", datos)
	}
	valor, err := strconv.ParseBool(comoString)
	if err != nil {
		return fmt.Errorf("email_verified %q no es un booleano: %w", comoString, err)
	}
	*b = boolFlexible(valor)
	return nil
}

// Verificar cumple application.VerificadorGoogle.
func (v *VerificadorGoogle) Verificar(ctx context.Context, idToken string) (*application.IdentidadGoogle, error) {
	// El error de la búsqueda de claves se guarda aparte porque golang-jwt lo
	// envuelve dentro de su propio error de parseo, y desde afuera no se puede
	// distinguir "no pudimos hablar con Google" (nuestro problema, un 500) de
	// "la firma no valida" (problema del token, un 401).
	var errClaves error

	var claims claimsGoogle
	_, err := jwt.ParseWithClaims(idToken, &claims,
		func(token *jwt.Token) (any, error) {
			kid, _ := token.Header["kid"].(string)
			clave, err := v.clavePorKid(ctx, kid)
			if err != nil {
				errClaves = err
				return nil, err
			}
			return clave, nil
		},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithAudience(v.clientID),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(v.ahora),
	)

	if errClaves != nil && !errors.Is(errClaves, errKidDesconocido) {
		return nil, fmt.Errorf("obteniendo las claves públicas de Google: %w", errClaves)
	}
	if err != nil {
		// El detalle va en el error envuelto (queda en los logs del servidor) pero
		// no en el mensaje que el handler le muestra al cliente:
		// ErrTokenGoogleInvalido tiene su propio texto genérico.
		return nil, fmt.Errorf("%w: %v", application.ErrTokenGoogleInvalido, err)
	}

	// El emisor se valida a mano porque son dos formas válidas y
	// jwt.WithIssuer solo acepta una.
	emisor, err := claims.GetIssuer()
	if err != nil || !contiene(emisoresGoogle, emisor) {
		return nil, fmt.Errorf("%w: emisor %q", application.ErrTokenGoogleInvalido, emisor)
	}

	if !v.dominioHabilitado(claims.Email) {
		return nil, application.ErrDominioNoPermitido
	}

	return &application.IdentidadGoogle{
		Sub:             claims.Subject,
		Email:           claims.Email,
		EmailVerificado: bool(claims.EmailVerificado),
		Nombre:          strings.TrimSpace(claims.Nombre),
		Apellido:        strings.TrimSpace(claims.Apellido),
	}, nil
}

func contiene(lista []string, valor string) bool {
	for _, v := range lista {
		if v == valor {
			return true
		}
	}
	return false
}

// dominioHabilitado aplica GOOGLE_DOMINIOS_PERMITIDOS. Lista vacía =
// cualquier dominio.
func (v *VerificadorGoogle) dominioHabilitado(email string) bool {
	if len(v.dominiosPermitidos) == 0 {
		return true
	}
	arroba := strings.LastIndex(email, "@")
	if arroba < 0 {
		return false
	}
	return contiene(v.dominiosPermitidos, strings.ToLower(email[arroba+1:]))
}

// clavePorKid devuelve la clave pública con la que verificar la firma,
// refrescando el cache si el kid no está o si las claves vencieron.
func (v *VerificadorGoogle) clavePorKid(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if clave, ok := v.claves[kid]; ok && v.ahora().Before(v.vencenEn) {
		return clave, nil
	}

	if err := v.refrescarClaves(ctx); err != nil {
		return nil, err
	}

	clave, ok := v.claves[kid]
	if !ok {
		return nil, fmt.Errorf("%w (kid %q)", errKidDesconocido, kid)
	}
	return clave, nil
}

// refrescarClaves trae el JWKS de Google.
func (v *VerificadorGoogle) refrescarClaves(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.urlCertificados, nil)
	if err != nil {
		return fmt.Errorf("armando el pedido de certificados: %w", err)
	}

	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("pidiendo los certificados a Google: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // solo lectura

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Google respondió %d al pedido de certificados", resp.StatusCode)
	}

	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("leyendo los certificados de Google: %w", err)
	}

	claves := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		// Solo claves RSA para RS256: el JWKS puede traer otras (Google hoy no,
		// pero el formato lo permite) y aceptarlas sería aceptar algoritmos que no
		// dijimos soportar.
		if k.Kty != "RSA" || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		clave, err := claveRSADesdeJWK(k.N, k.E)
		if err != nil {
			// Una clave ilegible no invalida a las demás: si justo es la que firmó
			// este token, el kid no va a estar y el token se rechaza por ese camino.
			continue
		}
		claves[k.Kid] = clave
	}

	if len(claves) == 0 {
		return errors.New("Google no devolvió ninguna clave RSA utilizable")
	}

	v.claves = claves
	v.vencenEn = v.ahora().Add(vigenciaDe(resp.Header.Get("Cache-Control")))
	return nil
}

// claveRSADesdeJWK arma la clave pública a partir del módulo y el exponente
// en base64url, que es como viaja en un JWKS.
func claveRSADesdeJWK(nBase64, eBase64 string) (*rsa.PublicKey, error) {
	// RawURLEncoding: el JWKS usa base64url SIN padding (RFC 7517).
	n, err := base64.RawURLEncoding.DecodeString(nBase64)
	if err != nil {
		return nil, fmt.Errorf("módulo inválido: %w", err)
	}
	e, err := base64.RawURLEncoding.DecodeString(eBase64)
	if err != nil {
		return nil, fmt.Errorf("exponente inválido: %w", err)
	}
	if len(n) == 0 || len(e) == 0 || len(e) > 8 {
		return nil, errors.New("módulo o exponente fuera de rango")
	}

	var exponente int
	for _, b := range e {
		exponente = exponente<<8 | int(b)
	}
	if exponente <= 0 {
		return nil, errors.New("exponente no positivo")
	}

	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponente}, nil
}

// vigenciaDe saca el max-age del Cache-Control con el que Google sirve sus
// certificados.
func vigenciaDe(cacheControl string) time.Duration {
	const (
		porDefecto = time.Hour
		minima     = 5 * time.Minute
		maxima     = 12 * time.Hour
	)

	for _, parte := range strings.Split(cacheControl, ",") {
		parte = strings.TrimSpace(strings.ToLower(parte))
		if !strings.HasPrefix(parte, "max-age=") {
			continue
		}
		segundos, err := strconv.Atoi(strings.TrimPrefix(parte, "max-age="))
		if err != nil {
			break
		}
		return min(max(time.Duration(segundos)*time.Second, minima), maxima)
	}
	return porDefecto
}
