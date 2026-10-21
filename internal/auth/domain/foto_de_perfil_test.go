package domain

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
	"time"
)

var ahoraFoto = time.Date(2026, time.August, 18, 10, 0, 0, 0, time.UTC)

func fotoDePrueba(t *testing.T, formato string, ancho, alto int) []byte {
	t.Helper()
	if formato == "webp-lossless" || formato == "webp-lossy" {
		contenido, err := os.ReadFile("testdata/foto-" + formato[5:] + ".webp")
		if err != nil {
			t.Fatal(err)
		}
		return contenido
	}
	var buf bytes.Buffer
	foto := image.NewNRGBA(image.Rect(0, 0, ancho, alto))
	var err error
	if formato == "png" {
		err = png.Encode(&buf, foto)
	} else {
		err = jpeg.Encode(&buf, foto, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNuevaFoto_AceptaLosTresFormatos(t *testing.T) {
	casos := map[string]string{
		"png": "image/png", "jpeg": "image/jpeg",
		"webp-lossless": "image/webp", "webp-lossy": "image/webp",
	}
	for nombre, tipo := range casos {
		t.Run(nombre, func(t *testing.T) {
			contenido := fotoDePrueba(t, nombre, 16, 16)
			f, err := NuevaFotoDePerfil("u1", contenido, ahoraFoto)
			if err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if f.Tipo != tipo || !bytes.Equal(f.Contenido, contenido) {
				t.Errorf("no conservó el tipo o los bytes de la foto: %s", f.Tipo)
			}
		})
	}
}

func TestNuevaFoto_RechazaFirmaValidaSinImagen(t *testing.T) {
	for nombre, contenido := range map[string][]byte{
		"png":  append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...),
		"jpeg": append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64)...),
		"webp": append([]byte("RIFF\x00\x00\x00\x00WEBP"), make([]byte, 64)...),
	} {
		t.Run(nombre, func(t *testing.T) {
			if _, err := NuevaFotoDePerfil("u1", contenido, ahoraFoto); !errors.Is(err, ErrFotoCorrupta) {
				t.Fatalf("se aceptó una cabecera sin imagen: %v", err)
			}
		})
	}
}

func TestNuevaFoto_RechazaPayloadTruncadoConCabeceraValida(t *testing.T) {
	for _, formato := range []string{"png", "jpeg", "webp-lossless", "webp-lossy"} {
		t.Run(formato, func(t *testing.T) {
			contenido := fotoDePrueba(t, formato, 16, 16)
			contenido = contenido[:len(contenido)-8]
			if _, _, err := image.DecodeConfig(bytes.NewReader(contenido)); err != nil {
				t.Fatalf("la prueba debe conservar una cabecera decodificable: %v", err)
			}
			if _, err := NuevaFotoDePerfil("u1", contenido, ahoraFoto); !errors.Is(err, ErrFotoCorrupta) {
				t.Fatalf("se aceptó una imagen truncada: %v", err)
			}
		})
	}
}

func TestNuevaFoto_LimitaDimensionesAntesDeDecodificar(t *testing.T) {
	for _, dimensiones := range [][2]int{{MaxDimensionFoto, 1}, {1, MaxDimensionFoto}, {MaxDimensionFoto + 1, 1}, {1, MaxDimensionFoto + 1}} {
		contenido := fotoDePrueba(t, "png", dimensiones[0], dimensiones[1])
		_, err := NuevaFotoDePerfil("u1", contenido, ahoraFoto)
		if dimensiones[0] <= MaxDimensionFoto && dimensiones[1] <= MaxDimensionFoto {
			if err != nil {
				t.Fatalf("rechazó el límite exacto %v: %v", dimensiones, err)
			}
		} else if !errors.Is(err, ErrFotoDimensiones) {
			t.Fatalf("no rechazó %v: %v", dimensiones, err)
		}
	}
}

func TestNuevaFoto_WebPNoConfiaEnDimensionesDelContenedor(t *testing.T) {
	// Un VP8X pequeño seguido por un VP8L cuya cabecera pide 16384 x 16384.
	// DecodeConfig sólo mira VP8X: el frame debe rechazarse sin decodificarlo.
	contenido := fotoDePrueba(t, "webp-lossless", 4, 4)
	if string(contenido[12:16]) != "VP8L" {
		t.Fatal("la imagen de prueba debe tener un frame VP8L simple")
	}
	frame := bytes.Clone(contenido[12:])
	for _, caso := range []struct {
		ancho, alto int
		err         error
	}{
		{16384, 16384, ErrFotoDimensiones},
		{5, 4, ErrFotoCorrupta},
	} {
		binary.LittleEndian.PutUint32(frame[9:13], uint32(caso.ancho-1)|uint32(caso.alto-1)<<14)
		trampa := append([]byte("RIFF"), make([]byte, 4)...)
		trampa = append(trampa, []byte("WEBPVP8X")...)
		trampa = binary.LittleEndian.AppendUint32(trampa, 10)
		trampa = append(trampa, []byte{0, 0, 0, 0, 3, 0, 0, 3, 0, 0}...)
		trampa = append(trampa, frame...)
		binary.LittleEndian.PutUint32(trampa[4:8], uint32(len(trampa)-8))
		config, _, err := image.DecodeConfig(bytes.NewReader(trampa))
		if err != nil || config.Width != 4 || config.Height != 4 {
			t.Fatalf("la prueba debe anunciar un contenedor 4 x 4: %+v %v", config, err)
		}
		if _, err := NuevaFotoDePerfil("u1", trampa, ahoraFoto); !errors.Is(err, caso.err) {
			t.Fatalf("se confió en el contenedor: %v", err)
		}
	}
}

func TestNuevaFoto_WebPLossyValidaDimensionesDelFrame(t *testing.T) {
	for _, ancho := range []int{16383, 5} {
		contenido := fotoDePrueba(t, "webp-lossy", 4, 4)
		encontrado := false
		for offset := 12; offset+8 <= len(contenido); {
			longitud := int(binary.LittleEndian.Uint32(contenido[offset+4 : offset+8]))
			if string(contenido[offset:offset+4]) == "VP8 " {
				binary.LittleEndian.PutUint16(contenido[offset+14:offset+16], uint16(ancho))
				encontrado = true
				break
			}
			offset += 8 + longitud + longitud%2
		}
		if !encontrado {
			t.Fatal("la imagen de prueba debe tener un frame VP8")
		}
		config, _, err := image.DecodeConfig(bytes.NewReader(contenido))
		if err != nil || config.Width != 4 || config.Height != 4 {
			t.Fatalf("la prueba debe anunciar un contenedor 4 x 4: %+v %v", config, err)
		}
		esperado := ErrFotoCorrupta
		if ancho > MaxDimensionFoto {
			esperado = ErrFotoDimensiones
		}
		if _, err := NuevaFotoDePerfil("u1", contenido, ahoraFoto); !errors.Is(err, esperado) {
			t.Fatalf("no comprobó el frame VP8: %v", err)
		}
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
