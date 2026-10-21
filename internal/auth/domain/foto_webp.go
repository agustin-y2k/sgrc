package domain

import (
	"bytes"
	"image"

	"golang.org/x/image/riff"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
)

// DecodeConfig puede devolver el tamaño VP8X sin leer el frame VP8/VP8L.
// Comprobamos también ese frame antes de que Decode reserve su bitmap.
func validarFrameWebP(contenido []byte, config image.Config) error {
	_, reader, err := riff.NewReader(bytes.NewReader(contenido))
	if err != nil {
		return ErrFotoCorrupta
	}
	for {
		chunk, longitud, datos, err := reader.Next()
		if err != nil {
			return ErrFotoCorrupta
		}
		var ancho, alto int
		switch chunk {
		case riff.FourCC{'V', 'P', '8', ' '}:
			decoder := vp8.NewDecoder()
			decoder.Init(datos, int(longitud))
			header, err := decoder.DecodeFrameHeader()
			if err != nil {
				return ErrFotoCorrupta
			}
			ancho, alto = header.Width, header.Height
		case riff.FourCC{'V', 'P', '8', 'L'}:
			header, err := vp8l.DecodeConfig(datos)
			if err != nil {
				return ErrFotoCorrupta
			}
			ancho, alto = header.Width, header.Height
		default:
			continue
		}
		if err := validarDimensionesFoto(ancho, alto); err != nil {
			return err
		}
		if ancho != config.Width || alto != config.Height {
			return ErrFotoCorrupta
		}
		return nil
	}
}
