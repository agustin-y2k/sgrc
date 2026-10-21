package domain

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"time"

	_ "golang.org/x/image/webp"
)

// FotoDePerfil es la imagen que alguien elige para su cuenta.
type FotoDePerfil struct {
	UsuarioID     string
	Contenido     []byte
	Tipo          string
	ActualizadaEn time.Time
}

// MaxBytesFoto es el tope de lo que se acepta guardar.
const MaxBytesFoto = 200 * 1024

// Se comprueba antes de decodificar: un archivo pequeño puede expandirse a
// millones de píxeles. Como máximo una foto de 2048 x 2048.
const MaxDimensionFoto = 2048

var (
	ErrFotoVacia       = errors.New("no llegó ninguna imagen")
	ErrFotoGrande      = fmt.Errorf("la imagen no puede pasar de %d KB", MaxBytesFoto/1024)
	ErrFotoTipo        = errors.New("la imagen tiene que ser JPG, PNG o WEBP")
	ErrFotoNoExiste    = errors.New("esa persona no tiene foto")
	ErrFotoCorrupta    = errors.New("el archivo no es una imagen válida")
	ErrFotoDimensiones = fmt.Errorf("la foto no puede superar %d píxeles de ancho ni de alto", MaxDimensionFoto)
	tiposDeFotoValid   = map[string]bool{
		"image/webp": true,
		"image/jpeg": true,
		"image/png":  true,
	}
)

// NuevaFotoDePerfil valida y arma la foto.
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
	config, _, err := image.DecodeConfig(bytes.NewReader(contenido))
	if err != nil {
		return nil, ErrFotoCorrupta
	}
	if err := validarDimensionesFoto(config.Width, config.Height); err != nil {
		return nil, err
	}
	if tipo == "image/webp" {
		if err := validarFrameWebP(contenido, config); err != nil {
			return nil, err
		}
	}
	// DecodeConfig sólo verifica la cabecera. La decodificación completa
	// rechaza archivos truncados o con datos de imagen corruptos.
	foto, _, err := image.Decode(bytes.NewReader(contenido))
	if err != nil || foto.Bounds().Dx() != config.Width || foto.Bounds().Dy() != config.Height {
		return nil, ErrFotoCorrupta
	}

	return &FotoDePerfil{
		UsuarioID:     usuarioID,
		Contenido:     contenido,
		Tipo:          tipo,
		ActualizadaEn: ahora,
	}, nil
}

func validarDimensionesFoto(ancho, alto int) error {
	if ancho <= 0 || alto <= 0 {
		return ErrFotoCorrupta
	}
	if ancho > MaxDimensionFoto || alto > MaxDimensionFoto {
		return ErrFotoDimensiones
	}
	return nil
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
