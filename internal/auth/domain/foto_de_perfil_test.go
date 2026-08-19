package domain

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

var ahoraFoto = time.Date(2026, time.August, 18, 10, 0, 0, 0, time.UTC)

// Firmas reales de cada formato, que es lo que mira el dominio.
var (
	png  = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 64)...)
	webp = append(append([]byte("RIFF"), []byte{0, 0, 0, 0}...), append([]byte("WEBP"), bytes.Repeat([]byte{0}, 64)...)...)
)

func TestNuevaFoto_AceptaLosTresFormatos(t *testing.T) {
	casos := map[string]struct {
		contenido []byte
		tipo      string
	}{
		"png":  {png, "image/png"},
		"jpeg": {jpeg, "image/jpeg"},
		"webp": {webp, "image/webp"},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			f, err := NuevaFotoDePerfil("u1", c.contenido, ahoraFoto)
			if err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if f.Tipo != c.tipo {
				t.Errorf("esperaba %q, obtuve %q", c.tipo, f.Tipo)
			}
		})
	}
}

// El tipo se deduce de los bytes y NO de lo que declare quien sube: un SVG
// trae JavaScript adentro y se serviría desde nuestro propio dominio, o sea
// con acceso a la sesión de quien lo mire.
func TestNuevaFoto_RechazaLoQueNoEsImagen(t *testing.T) {
	casos := map[string][]byte{
		"un SVG renombrado":    []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"texto plano":          []byte("no soy una imagen"),
		"un PDF":               []byte("%PDF-1.7\n..."),
		"casi un PNG":          []byte("\x89PNG\r\n\x1a"),
		"un WEBP sin la marca": append([]byte("RIFF"), bytes.Repeat([]byte{0}, 32)...),
	}
	for nombre, contenido := range casos {
		t.Run(nombre, func(t *testing.T) {
			if _, err := NuevaFotoDePerfil("u1", contenido, ahoraFoto); !errors.Is(err, ErrFotoTipo) {
				t.Errorf("esperaba ErrFotoTipo, obtuve %v", err)
			}
		})
	}
}

func TestNuevaFoto_Vacia(t *testing.T) {
	if _, err := NuevaFotoDePerfil("u1", nil, ahoraFoto); !errors.Is(err, ErrFotoVacia) {
		t.Errorf("esperaba ErrFotoVacia, obtuve %v", err)
	}
}

// El tope existe contra lo que no pasó por el recorte del navegador: alguien
// llamando a la API directo, o un navegador donde ese paso falló.
func TestNuevaFoto_TopeDeTamanio(t *testing.T) {
	grande := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, MaxBytesFoto)...)
	if _, err := NuevaFotoDePerfil("u1", grande, ahoraFoto); !errors.Is(err, ErrFotoGrande) {
		t.Errorf("esperaba ErrFotoGrande, obtuve %v", err)
	}
}

func TestEsTipoValido(t *testing.T) {
	if !EsTipoValido("image/png") {
		t.Error("image/png tiene que ser válido")
	}
	// El que motiva toda la validación por firma.
	if EsTipoValido("image/svg+xml") {
		t.Error("image/svg+xml NO puede ser válido: un SVG puede traer scripts")
	}
}
