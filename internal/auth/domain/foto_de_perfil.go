package domain

import (
	"errors"
	"fmt"
	"time"
)

// FotoDePerfil es la imagen que alguien elige para su cuenta.
//
// Opcional siempre: sin foto, la interfaz muestra las iniciales, que es lo
// que hacía antes. Nadie tiene que subir una para poder trabajar.
type FotoDePerfil struct {
	UsuarioID     string
	Contenido     []byte
	Tipo          string
	ActualizadaEn time.Time
}

// MaxBytesFoto es el tope de lo que se acepta guardar.
//
// El navegador recorta y achica la imagen a 256×256 antes de mandarla, así
// que lo normal son unas decenas de kilobytes; este límite es contra lo que
// no pasó por ahí (alguien llamando a la API directo, o un navegador donde
// el recorte falló). Doscientos kilobytes dejan lugar de sobra para una foto
// de ese tamaño incluso mal comprimida, y ponen un techo a lo que una cuenta
// puede ocupar en la base.
const MaxBytesFoto = 200 * 1024

var (
	ErrFotoVacia     = errors.New("no llegó ninguna imagen")
	ErrFotoGrande    = fmt.Errorf("la imagen no puede pasar de %d KB", MaxBytesFoto/1024)
	ErrFotoTipo      = errors.New("la imagen tiene que ser JPG, PNG o WEBP")
	ErrFotoNoExiste  = errors.New("esa persona no tiene foto")
	ErrFotoCorrupta  = errors.New("el archivo no es una imagen válida")
	tiposDeFotoValid = map[string]bool{
		"image/webp": true,
		"image/jpeg": true,
		"image/png":  true,
	}
)

// NuevaFotoDePerfil valida y arma la foto.
//
// El tipo NO se toma de lo que diga quien sube —eso es un dato que manda el
// cliente y se puede escribir a mano—: se deduce de los primeros bytes del
// archivo. Un SVG renombrado a .png puede traer JavaScript adentro, y se
// serviría desde nuestro propio dominio, o sea con acceso a la sesión de
// quien lo mire. Por eso la lista es cerrada y por firma.
func NuevaFotoDePerfil(usuarioID string, contenido []byte, ahora time.Time) (*FotoDePerfil, error) {
	if len(contenido) == 0 {
		return nil, ErrFotoVacia
	}
	if len(contenido) > MaxBytesFoto {
		return nil, ErrFotoGrande
	}

	tipo := tipoDeImagen(contenido)
	if tipo == "" {
		return nil, ErrFotoTipo
	}

	return &FotoDePerfil{
		UsuarioID:     usuarioID,
		Contenido:     contenido,
		Tipo:          tipo,
		ActualizadaEn: ahora,
	}, nil
}

// tipoDeImagen mira la firma del archivo (los "números mágicos" del
// principio). Devuelve "" si no es ninguno de los tres formatos aceptados.
func tipoDeImagen(b []byte) string {
	switch {
	// PNG: \x89PNG\r\n\x1a\n
	case len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	// JPEG: FF D8 FF
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg"
	// WEBP: "RIFF" .... "WEBP"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	default:
		return ""
	}
}

// EsTipoValido lo usa infrastructure antes de guardar, para que un tipo que
// no esté en la lista no llegue nunca al CHECK de la base (que también lo
// rechaza, pero con un error que no sirve para mostrar).
func EsTipoValido(tipo string) bool { return tiposDeFotoValid[tipo] }
