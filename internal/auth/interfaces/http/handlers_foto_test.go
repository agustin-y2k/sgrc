package http

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/agustin-y2k/sgrc/internal/auth/domain"
)

func TestHTTP_FotoValidaYReemplazoInvalidoConservaLaAnterior(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)
	token := tokenPara("foto-u1", "DOCENTE")
	var valida bytes.Buffer
	if err := png.Encode(&valida, image.NewNRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	subir := func(contenido []byte, esperado int) {
		t.Helper()
		var cuerpo bytes.Buffer
		form := multipart.NewWriter(&cuerpo)
		archivo, err := form.CreateFormFile("foto", "archivo.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := archivo.Write(contenido); err != nil {
			t.Fatal(err)
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("PUT", "/api/mi-foto", &cuerpo)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != esperado {
			t.Fatalf("estado=%d esperado=%d", resp.StatusCode, esperado)
		}
	}
	subir(valida.Bytes(), 200)
	subir([]byte("\x89PNG\r\n\x1a\ncontenido inválido"), 400)
	subir(valida.Bytes()[:len(valida.Bytes())-8], 400)
	var excesiva bytes.Buffer
	if err := png.Encode(&excesiva, image.NewNRGBA(image.Rect(0, 0, domain.MaxDimensionFoto+1, 1))); err != nil {
		t.Fatal(err)
	}
	subir(excesiva.Bytes(), 400)
	subir(make([]byte, domain.MaxBytesFoto+1), 413)

	req := httptest.NewRequest("GET", "/api/auth/usuarios/foto-u1/foto", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	contenido, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || !bytes.Equal(contenido, valida.Bytes()) {
		t.Fatal("un reemplazo inválido alteró la foto válida o su tipo")
	}
}
