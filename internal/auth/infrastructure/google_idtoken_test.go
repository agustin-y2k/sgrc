package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramiro/sgrc/internal/auth/application"
)

// Estos tests firman tokens de verdad con una clave RSA generada al vuelo y
// sirven el JWKS desde un httptest.Server, así que ejercitan el mismo camino
// que un ID token real de Google: firma, kid, cache de claves y todos los
// chequeos de claims.

const clientIDDePrueba = "123456789-abcdef.apps.googleusercontent.com"

// Generar una clave RSA de 2048 bits cuesta cerca de un segundo, y estos
// tests necesitan la misma clave una y otra vez: se genera una sola por
// proceso.
var (
	claveDePrueba  = sync.OnceValue(generarClave)
	claveImpostora = sync.OnceValue(generarClave)
)

func generarClave() *rsa.PrivateKey {
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("generando clave RSA de prueba: " + err.Error())
	}
	return clave
}

var ahoraDePrueba = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func relojDePrueba(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// jwksDePrueba levanta un servidor que publica la clave pública en formato
// JWKS, igual que https://www.googleapis.com/oauth2/v3/certs.
func jwksDePrueba(t *testing.T, kid string, clave *rsa.PublicKey, cacheControl string) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var pedidos atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos.Add(1)
		if cacheControl != "" {
			w.Header().Set("Cache-Control", cacheControl)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kid": kid,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(clave.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(clave.E)).Bytes()),
			}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &pedidos
}

// claimsValidos son los de un ID token recién emitido por Google para
// nuestra aplicación.
func claimsValidos() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":            "https://accounts.google.com",
		"aud":            clientIDDePrueba,
		"sub":            "112233445566",
		"email":          "ada@escuela.edu.ar",
		"email_verified": true,
		"given_name":     "Ada",
		"family_name":    "Lovelace",
		"iat":            ahoraDePrueba.Add(-time.Minute).Unix(),
		"exp":            ahoraDePrueba.Add(time.Hour).Unix(),
	}
}

func firmar(t *testing.T, privada *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	firmado, err := token.SignedString(privada)
	if err != nil {
		t.Fatalf("firmando el token de prueba: %v", err)
	}
	return firmado
}

// entornoDePrueba arma clave + servidor JWKS + verificador apuntando a él.
func entornoDePrueba(t *testing.T, dominios []string) (*rsa.PrivateKey, *VerificadorGoogle, *atomic.Int64) {
	t.Helper()

	privada := claveDePrueba()
	srv, pedidos := jwksDePrueba(t, "kid-1", &privada.PublicKey, "")

	v := NewVerificadorGoogle(clientIDDePrueba, dominios, relojDePrueba(ahoraDePrueba))
	v.urlCertificados = srv.URL

	return privada, v, pedidos
}

// ── El camino feliz ───────────────────────────────────────────────────

func TestVerificar_TokenValido(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	identidad, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claimsValidos()))
	if err != nil {
		t.Fatalf("un token válido no debería fallar: %v", err)
	}

	if identidad.Sub != "112233445566" {
		t.Errorf("sub inesperado: %q", identidad.Sub)
	}
	if identidad.Email != "ada@escuela.edu.ar" {
		t.Errorf("email inesperado: %q", identidad.Email)
	}
	if !identidad.EmailVerificado {
		t.Error("email_verified debería ser true")
	}
	if identidad.Nombre != "Ada" || identidad.Apellido != "Lovelace" {
		t.Errorf("nombre inesperado: %q %q", identidad.Nombre, identidad.Apellido)
	}
}

// Google usa las dos formas de iss indistintamente.
func TestVerificar_AceptaLosDosEmisoresDeGoogle(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	for _, emisor := range []string{"https://accounts.google.com", "accounts.google.com"} {
		claims := claimsValidos()
		claims["iss"] = emisor

		if _, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims)); err != nil {
			t.Errorf("el emisor %q debería aceptarse: %v", emisor, err)
		}
	}
}

// ── Lo que tiene que rechazar ─────────────────────────────────────────

// El chequeo más importante de todos: Google le firma ID tokens a cualquiera
// que tenga una aplicación registrada.
func TestVerificar_TokenParaOtraAplicacion_Rechaza(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	claims := claimsValidos()
	claims["aud"] = "otra-app.apps.googleusercontent.com"

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))

	if !errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("un token emitido para otra aplicación tiene que rechazarse: %v", err)
	}
}

func TestVerificar_FirmadoConOtraClave_Rechaza(t *testing.T) {
	_, v, _ := entornoDePrueba(t, nil)

	// Una clave que no es la que publica el JWKS, pero con el mismo kid: el
	// atacante controla el header, así que puede poner el kid que quiera.
	_, err := v.Verificar(context.Background(), firmar(t, claveImpostora(), "kid-1", claimsValidos()))

	if !errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("una firma que no valida tiene que rechazarse: %v", err)
	}
}

// La familia de bugs clásica de JWT: aceptar el algoritmo que declara el
// propio token.
func TestVerificar_AlgoritmoNoPermitido_Rechaza(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	casos := map[string]string{
		"alg none": func() string {
			token := jwt.NewWithClaims(jwt.SigningMethodNone, claimsValidos())
			token.Header["kid"] = "kid-1"
			firmado, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
			if err != nil {
				t.Fatalf("firmando con alg none: %v", err)
			}
			return firmado
		}(),
		"HS256 con la clave pública como secreto": func() string {
			publica := privada.PublicKey.N.Bytes()
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsValidos())
			token.Header["kid"] = "kid-1"
			firmado, err := token.SignedString(publica)
			if err != nil {
				t.Fatalf("firmando con HS256: %v", err)
			}
			return firmado
		}(),
	}

	for nombre, token := range casos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := v.Verificar(context.Background(), token); !errors.Is(err, application.ErrTokenGoogleInvalido) {
				t.Fatalf("tenía que rechazarse: %v", err)
			}
		})
	}
}

func TestVerificar_TokenVencido_Rechaza(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	claims := claimsValidos()
	claims["exp"] = ahoraDePrueba.Add(-time.Minute).Unix()

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))

	if !errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("un token vencido tiene que rechazarse: %v", err)
	}
}

func TestVerificar_SinExpiracion_Rechaza(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	claims := claimsValidos()
	delete(claims, "exp")

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))

	if !errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("un token sin exp no caduca nunca: tiene que rechazarse (%v)", err)
	}
}

func TestVerificar_EmisorQueNoEsGoogle_Rechaza(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	claims := claimsValidos()
	claims["iss"] = "https://accounts.google.com.attacker.example"

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))

	if !errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("un emisor parecido pero distinto tiene que rechazarse: %v", err)
	}
}

func TestVerificar_KidDesconocido_RechazaSinTratarloComoFallaNuestra(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-que-google-no-publica", claimsValidos()))

	if !errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("esperaba ErrTokenGoogleInvalido, hubo: %v", err)
	}
}

// email_verified viaja hasta application, que es quien decide qué hacer
// con un false. El verificador no lo filtra por su cuenta.
func TestVerificar_EmailNoVerificado_LoReporta(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	claims := claimsValidos()
	claims["email_verified"] = false

	identidad, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))
	if err != nil {
		t.Fatalf("no debería fallar la verificación en sí: %v", err)
	}
	if identidad.EmailVerificado {
		t.Error("email_verified false tiene que llegar como false")
	}
}

// Algunos endpoints de Google devuelven el flag como string. Si llegara
// así, un bool pelado fallaría el Unmarshal del token entero.
func TestVerificar_EmailVerificadoComoString(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	claims := claimsValidos()
	claims["email_verified"] = "true"

	identidad, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !identidad.EmailVerificado {
		t.Error(`"true" como string tiene que leerse como verdadero`)
	}
}

// ── Restricción por dominio ───────────────────────────────────────────

func TestVerificar_DominioPermitido(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, []string{"escuela.edu.ar"})

	if _, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claimsValidos())); err != nil {
		t.Fatalf("el dominio está en la lista: %v", err)
	}
}

func TestVerificar_DominioNoPermitido(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, []string{"escuela.edu.ar"})

	claims := claimsValidos()
	claims["email"] = "cualquiera@gmail.com"

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))

	if !errors.Is(err, application.ErrDominioNoPermitido) {
		t.Fatalf("esperaba ErrDominioNoPermitido, hubo: %v", err)
	}
}

// Un dominio que TERMINA en el permitido no es el permitido.
func TestVerificar_DominioParecido_NoAlcanza(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, []string{"escuela.edu.ar"})

	claims := claimsValidos()
	claims["email"] = "cualquiera@noesescuela.edu.ar"

	if _, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims)); !errors.Is(err, application.ErrDominioNoPermitido) {
		t.Fatalf("esperaba ErrDominioNoPermitido, hubo: %v", err)
	}
}

// ── Cache de claves ───────────────────────────────────────────────────

func TestVerificar_CacheaLasClaves(t *testing.T) {
	privada, v, pedidos := entornoDePrueba(t, nil)

	for i := 0; i < 5; i++ {
		if _, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claimsValidos())); err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
	}

	if n := pedidos.Load(); n != 1 {
		t.Errorf("esperaba un solo pedido de certificados a Google, hubo %d", n)
	}
}

// Con las claves vencidas hay que volver a pedirlas: si no, el día que Google
// rote sus claves todos los logins fallarían hasta reiniciar el proceso.
func TestVerificar_ClavesVencidas_LasVuelveAPedir(t *testing.T) {
	privada := claveDePrueba()
	srv, pedidos := jwksDePrueba(t, "kid-1", &privada.PublicKey, "public, max-age=600")

	ahora := ahoraDePrueba
	v := NewVerificadorGoogle(clientIDDePrueba, nil, func() time.Time { return ahora })
	v.urlCertificados = srv.URL

	claims := claimsValidos()
	claims["exp"] = ahora.Add(24 * time.Hour).Unix()

	if _, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	ahora = ahora.Add(11 * time.Minute) // pasado el max-age de 600s

	if _, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if n := pedidos.Load(); n != 2 {
		t.Errorf("esperaba que volviera a pedir los certificados al vencerse, hubo %d pedidos", n)
	}
}

// Un token basura con un kid inventado no puede provocar un pedido a Google
// por intento: sería un amplificador de tráfico gratuito contra un endpoint
// público.
func TestVerificar_KidDesconocido_RefrescaUnaSolaVez(t *testing.T) {
	privada, v, pedidos := entornoDePrueba(t, nil)

	for i := 0; i < 3; i++ {
		_, _ = v.Verificar(context.Background(), firmar(t, privada, "kid-inventado", claimsValidos()))
	}

	if n := pedidos.Load(); n != 3 {
		t.Errorf("esperaba un refresco por intento y ninguno de más, hubo %d pedidos para 3 intentos", n)
	}
}

// ── Fallas de infraestructura ─────────────────────────────────────────

// No poder hablar con Google no dice nada sobre el token.
func TestVerificar_SinPoderTraerLasClaves_NoEsTokenInvalido(t *testing.T) {
	privada := claveDePrueba()

	// Un servidor que ya está cerrado: la conexión falla.
	srv, _ := jwksDePrueba(t, "kid-1", &privada.PublicKey, "")
	urlMuerta := srv.URL
	srv.Close()

	v := NewVerificadorGoogle(clientIDDePrueba, nil, relojDePrueba(ahoraDePrueba))
	v.urlCertificados = urlMuerta

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claimsValidos()))

	if err == nil {
		t.Fatal("esperaba un error")
	}
	if errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("una falla de red no es un token inválido: %v", err)
	}
}

func TestVerificar_GoogleRespondeError_NoEsTokenInvalido(t *testing.T) {
	privada := claveDePrueba()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	v := NewVerificadorGoogle(clientIDDePrueba, nil, relojDePrueba(ahoraDePrueba))
	v.urlCertificados = srv.URL

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claimsValidos()))

	if err == nil || errors.Is(err, application.ErrTokenGoogleInvalido) {
		t.Fatalf("un 500 de Google no es un token inválido: %v", err)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

func TestDominiosPermitidos(t *testing.T) {
	casos := []struct {
		crudo    string
		esperado []string
	}{
		{"", nil},
		{"   ", nil},
		{"escuela.edu.ar", []string{"escuela.edu.ar"}},
		{"Escuela.Edu.Ar", []string{"escuela.edu.ar"}},
		{"@escuela.edu.ar", []string{"escuela.edu.ar"}},
		{"a.edu.ar, b.edu.ar", []string{"a.edu.ar", "b.edu.ar"}},
		{"a.edu.ar,,b.edu.ar,", []string{"a.edu.ar", "b.edu.ar"}},
	}

	for _, c := range casos {
		obtenido := DominiosPermitidos(c.crudo)
		if fmt.Sprint(obtenido) != fmt.Sprint(c.esperado) {
			t.Errorf("DominiosPermitidos(%q) = %v, esperaba %v", c.crudo, obtenido, c.esperado)
		}
	}
}

func TestVigenciaDe(t *testing.T) {
	casos := []struct {
		cacheControl string
		esperado     time.Duration
	}{
		{"", time.Hour},
		{"no-cache", time.Hour},
		{"public, max-age=3600", time.Hour},
		{"public, max-age=abc", time.Hour},
		// Por debajo del mínimo: un max-age de un segundo convertiría cada
		// login en un pedido a Google.
		{"public, max-age=1", 5 * time.Minute},
		// Por encima del máximo: el cache quedaría caliente después de que
		// Google rotara sus claves.
		{"public, max-age=86400", 12 * time.Hour},
	}

	for _, c := range casos {
		if obtenido := vigenciaDe(c.cacheControl); obtenido != c.esperado {
			t.Errorf("vigenciaDe(%q) = %v, esperaba %v", c.cacheControl, obtenido, c.esperado)
		}
	}
}

func TestClaveRSADesdeJWK_EntradaInvalida(t *testing.T) {
	casos := map[string][2]string{
		"módulo no es base64url": {"no-es-base64!!", "AQAB"},
		"exponente vacío":        {base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3}), ""},
		"módulo vacío":           {"", "AQAB"},
	}

	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := claveRSADesdeJWK(c[0], c[1]); err == nil {
				t.Fatal("esperaba un error")
			}
		})
	}
}

// El JWKS usa base64url SIN padding: si se decodificara con el alfabeto
// estándar, las claves de Google no se podrían leer.
func TestClaveRSADesdeJWK_ReconstruyeLaClave(t *testing.T) {
	privada := claveDePrueba()

	clave, err := claveRSADesdeJWK(
		base64.RawURLEncoding.EncodeToString(privada.PublicKey.N.Bytes()),
		base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privada.PublicKey.E)).Bytes()),
	)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if clave.N.Cmp(privada.PublicKey.N) != 0 || clave.E != privada.PublicKey.E {
		t.Error("la clave reconstruida no coincide con la original")
	}
}

// El mensaje que ve el cliente no dice qué chequeo falló, pero el error
// envuelto sí — es lo que queda en el log del servidor cuando hay que
// entender por qué un login no funciona.
func TestVerificar_ElDetalleQuedaEnElErrorEnvuelto(t *testing.T) {
	privada, v, _ := entornoDePrueba(t, nil)

	claims := claimsValidos()
	claims["aud"] = "otra-app"

	_, err := v.Verificar(context.Background(), firmar(t, privada, "kid-1", claims))

	if err == nil || !strings.Contains(err.Error(), "aud") {
		t.Errorf("el error debería mencionar el claim que falló, fue: %v", err)
	}
}
