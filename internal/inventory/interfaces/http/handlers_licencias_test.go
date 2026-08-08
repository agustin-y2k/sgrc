package http

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// El reloj de nuevaAppDeTest está fijado en el 1 de enero de 2026.
const hoyEnLaApp = "2026-01-01"

func repoConEquipo(t *testing.T) *fakeRepo {
	t.Helper()
	repo := nuevoFakeRepo()
	repo.carros["carro-1"] = &domain.Carro{ID: "carro-1", Nombre: "Carro 1"}
	repo.equipos["equipo-1"] = &domain.Equipo{ID: "equipo-1", CarroID: "carro-1", Identificador: 1, Estado: domain.EstadoDisponible}
	repo.equipos["equipo-2"] = &domain.Equipo{ID: "equipo-2", CarroID: "carro-1", Identificador: 2, Estado: domain.EstadoDisponible}
	return repo
}

func pedir(t *testing.T, app *fiber.App, metodo, ruta string, cuerpo any, rol string) (int, []byte) {
	t.Helper()

	r := httptest.NewRequest(metodo, ruta, jsonBody(cuerpo))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+tokenPara("usr-1", rol))

	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	defer resp.Body.Close()
	cuerpoResp, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	return resp.StatusCode, cuerpoResp
}

func TestHTTP_CrearLicencias_EnVariasEquipos(t *testing.T) {
	repo := repoConEquipo(t)
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias", crearLicenciasRequest{
		EquipoIDs: []string{"equipo-1", "equipo-2"}, Nombre: "AutoCAD 2027", DiasDuracion: 30,
	}, "ADMIN")

	if codigo != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}
	var resp altaMasivaResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Creadas) != 2 {
		t.Fatalf("esperaba 2 creadas, obtuve %d", len(resp.Creadas))
	}
	// Sin declarar vencimiento nacen "a verificar", y la respuesta lo dice
	// con el estado, no con un null que la pantalla tenga que interpretar.
	for _, l := range resp.Creadas {
		if l.FechaVencimiento != nil {
			t.Errorf("esperaba sin fecha, vino %q", *l.FechaVencimiento)
		}
		if l.Estado != string(domain.LicenciaSinFecha) {
			t.Errorf("estado = %q, esperaba %q", l.Estado, domain.LicenciaSinFecha)
		}
		if l.DiasAviso != diasAvisoPorDefecto {
			t.Errorf("diasAviso = %d, esperaba el default %d", l.DiasAviso, diasAvisoPorDefecto)
		}
	}
}

func TestHTTP_CrearLicencias_ConQuedanDias(t *testing.T) {
	app := nuevaAppDeTest(repoConEquipo(t))
	doce := 12

	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias", crearLicenciasRequest{
		EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30,
		vencimientoRequest: vencimientoRequest{QuedanDias: &doce},
	}, "ADMIN")

	if codigo != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}
	var resp altaMasivaResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	l := resp.Creadas[0]
	if l.FechaVencimiento == nil || *l.FechaVencimiento != "2026-01-13" {
		t.Errorf("fechaVencimiento = %v, esperaba 2026-01-13", l.FechaVencimiento)
	}
	if l.DiasRestantes == nil || *l.DiasRestantes != 12 {
		t.Errorf("diasRestantes = %v, esperaba 12", l.DiasRestantes)
	}
}

func TestHTTP_CrearLicencias_SegundaTandaInformaLasQueYaEstaban(t *testing.T) {
	repo := repoConEquipo(t)
	app := nuevaAppDeTest(repo)
	req := crearLicenciasRequest{EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30}

	if codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias", req, "ADMIN"); codigo != fiber.StatusCreated {
		t.Fatalf("la primera: esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}

	req.EquipoIDs = []string{"equipo-1", "equipo-2"}
	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias", req, "ADMIN")

	// 201 y no 409: el lote se procesó, y lo que pasó con cada PC está en
	// el cuerpo. Un conflicto obligaría a la pantalla a deshacer lo que sí
	// entró.
	if codigo != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}
	var resp altaMasivaResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Creadas) != 1 || len(resp.EquiposQueYaLaTenian) != 1 {
		t.Errorf("esperaba 1 creada y 1 salteada, obtuve %d y %d", len(resp.Creadas), len(resp.EquiposQueYaLaTenian))
	}
}

func TestHTTP_CrearLicencias_FechaMalFormada(t *testing.T) {
	app := nuevaAppDeTest(repoConEquipo(t))
	mal := "03/09/2026"

	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias", crearLicenciasRequest{
		EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30,
		vencimientoRequest: vencimientoRequest{VenceEl: &mal},
	}, "ADMIN")

	if codigo != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d: %s", codigo, cuerpo)
	}
}

func TestHTTP_CrearLicencias_VencimientoAmbiguo(t *testing.T) {
	app := nuevaAppDeTest(repoConEquipo(t))
	doce := 12
	vence := "2026-03-15"

	codigo, _ := pedir(t, app, "POST", "/api/inventory/licencias", crearLicenciasRequest{
		EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30,
		vencimientoRequest: vencimientoRequest{QuedanDias: &doce, VenceEl: &vence},
	}, "ADMIN")

	if codigo != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", codigo)
	}
}

func TestHTTP_RenovarLicencias(t *testing.T) {
	repo := repoConEquipo(t)
	app := nuevaAppDeTest(repo)
	vence := "2026-01-05"
	_, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias", crearLicenciasRequest{
		EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30,
		vencimientoRequest: vencimientoRequest{VenceEl: &vence},
	}, "ADMIN")
	var alta altaMasivaResponse
	if err := json.Unmarshal(cuerpo, &alta); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}

	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias/renovar", renovarLicenciasRequest{
		LicenciaIDs: []string{alta.Creadas[0].ID},
	}, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var resp renovacionResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Renovadas) != 1 {
		t.Fatalf("esperaba 1 renovada, obtuve %d", len(resp.Renovadas))
	}
	// hoy + 30 días.
	if *resp.Renovadas[0].FechaVencimiento != "2026-01-31" {
		t.Errorf("fechaVencimiento = %q, esperaba 2026-01-31", *resp.Renovadas[0].FechaVencimiento)
	}
	if *resp.Renovadas[0].UltimaRenovacion != hoyEnLaApp {
		t.Errorf("ultimaRenovacion = %q, esperaba %q", *resp.Renovadas[0].UltimaRenovacion, hoyEnLaApp)
	}
}

// TestHTTP_RenovarLicencias_SinFechaPreviaSeInforma: el botón "Renovar" no
// puede ser un atajo para ponerle treinta días a una licencia que nadie
// verificó.
func TestHTTP_RenovarLicencias_SinFechaPreviaSeInforma(t *testing.T) {
	repo := repoConEquipo(t)
	app := nuevaAppDeTest(repo)
	_, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias", crearLicenciasRequest{
		EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30,
	}, "ADMIN")
	var alta altaMasivaResponse
	if err := json.Unmarshal(cuerpo, &alta); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	id := alta.Creadas[0].ID

	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/licencias/renovar", renovarLicenciasRequest{
		LicenciaIDs: []string{id},
	}, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var resp renovacionResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Renovadas) != 0 || len(resp.SinFechaPrevia) != 1 {
		t.Errorf("esperaba 0 renovadas y 1 informada, obtuve %d y %d", len(resp.Renovadas), len(resp.SinFechaPrevia))
	}
	if repo.licencias[id].FechaVencimiento != nil {
		t.Error("una licencia sin verificar no puede terminar con fecha inventada")
	}
}

func TestHTTP_EditarLicencia_CorregirElVencimiento(t *testing.T) {
	repo := repoConEquipo(t)
	app := nuevaAppDeTest(repo)
	l, err := domain.NuevaLicencia("lic-1", "equipo-1", "AutoCAD 2027", 30, 1, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.licencias["lic-1"] = l
	nuevaFecha := "2026-02-10"

	codigo, cuerpo := pedir(t, app, "PATCH", "/api/inventory/licencias/lic-1", editarLicenciaRequest{
		vencimientoRequest: vencimientoRequest{VenceEl: &nuevaFecha},
	}, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	if repo.licencias["lic-1"].FechaVencimiento.Format(formatoFecha) != nuevaFecha {
		t.Errorf("no se guardó la fecha: %v", repo.licencias["lic-1"].FechaVencimiento)
	}
	// Queda registrado quién la tocó, que es lo que responde "¿por qué se
	// movió esto?" sin ir al audit_log.
	if repo.licencias["lic-1"].VencimientoFijadoPor == nil {
		t.Error("no quedó registrado quién fijó el vencimiento")
	}
}

func TestHTTP_EditarLicencia_NoEncontrada(t *testing.T) {
	app := nuevaAppDeTest(repoConEquipo(t))

	codigo, _ := pedir(t, app, "PATCH", "/api/inventory/licencias/no-existe", editarLicenciaRequest{}, "ADMIN")

	if codigo != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", codigo)
	}
}

func TestHTTP_BorrarLicencia(t *testing.T) {
	repo := repoConEquipo(t)
	app := nuevaAppDeTest(repo)
	l, err := domain.NuevaLicencia("lic-1", "equipo-1", "AutoCAD 2027", 30, 1, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.licencias["lic-1"] = l

	codigo, _ := pedir(t, app, "DELETE", "/api/inventory/licencias/lic-1", nil, "ADMIN")

	if codigo != fiber.StatusNoContent {
		t.Fatalf("esperaba 204, obtuve %d", codigo)
	}
	if _, sigue := repo.licencias["lic-1"]; sigue {
		t.Error("la licencia no se borró")
	}
}

func TestHTTP_ListarLicencias_TraeLaUbicacion(t *testing.T) {
	repo := repoConEquipo(t)
	app := nuevaAppDeTest(repo)
	l, err := domain.NuevaLicencia("lic-1", "equipo-1", "AutoCAD 2027", 30, 1, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	l.FijarVencimiento(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC), "admin-1", time.Now())
	repo.licencias["lic-1"] = l

	codigo, cuerpo := pedir(t, app, "GET", "/api/inventory/licencias", nil, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var resp struct {
		Data []licenciaResponse `json:"data"`
	}
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("esperaba 1 licencia, obtuve %d", len(resp.Data))
	}
	if resp.Data[0].Identificador != 1 || resp.Data[0].CarroNombre != "Carro 1" {
		t.Errorf("falta la ubicación: %+v", resp.Data[0])
	}
	// Vence mañana con dias_aviso 1: la pantalla lo tiene que poder pintar
	// sin recalcular nada.
	if resp.Data[0].Estado != string(domain.LicenciaPorVencer) {
		t.Errorf("estado = %q, esperaba %q", resp.Data[0].Estado, domain.LicenciaPorVencer)
	}
}

// TestHTTP_Licencias_SoloAdmin: un docente no tiene nada que hacer acá. La
// información que sí le sirve para elegir PC es software_instalado, que ve
// en la pantalla de reserva.
func TestHTTP_Licencias_SoloAdmin(t *testing.T) {
	app := nuevaAppDeTest(repoConEquipo(t))

	rutas := []struct{ metodo, ruta string }{
		{"GET", "/api/inventory/licencias"},
		{"POST", "/api/inventory/licencias"},
		{"POST", "/api/inventory/licencias/renovar"},
		{"PATCH", "/api/inventory/licencias/lic-1"},
		{"DELETE", "/api/inventory/licencias/lic-1"},
		{"GET", "/api/inventory/equipos/equipo-1/licencias"},
	}

	for _, r := range rutas {
		codigo, _ := pedir(t, app, r.metodo, r.ruta, nil, "DOCENTE")
		if codigo != fiber.StatusForbidden {
			t.Errorf("%s %s: esperaba 403 para un DOCENTE, obtuve %d", r.metodo, r.ruta, codigo)
		}
	}
}

// ── Equipos que no están en ningún carro (RF-03.15) ─────────────────────

func TestHTTP_CrearEquipo_ProyectorReservable(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/equipos/sueltos", crearEquipoSueltoRequest{
		Tipo: "PROYECTOR", Nombre: "Proyector Epson", Reservable: true,
	}, "ADMIN")

	if codigo != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}
	var resp equipoResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	// Sin carro, sin número y sin serie: los tres campos ni siquiera viajan.
	if resp.CarroID != "" || resp.Identificador != 0 || resp.NumeroSerie != "" {
		t.Errorf("un proyector no tiene carro ni número ni serie: %+v", resp)
	}
	// Y se nombra por su nombre, no "PC 0".
	if resp.Etiqueta != "Proyector Epson" || !resp.Reservable {
		t.Errorf("etiqueta=%q reservable=%v", resp.Etiqueta, resp.Reservable)
	}
}

func TestHTTP_CrearEquipo_CargadorNoReservable(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedir(t, app, "POST", "/api/inventory/equipos/sueltos", crearEquipoSueltoRequest{
		Tipo: "CARGADOR", Nombre: "Cargador 1", Reservable: false,
	}, "ADMIN")

	if codigo != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}
	var resp equipoResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if resp.Reservable {
		t.Error("un cargador se presta en el momento; nadie planifica con él")
	}
}

func TestHTTP_CrearEquipo_SinNombre(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	codigo, _ := pedir(t, app, "POST", "/api/inventory/equipos/sueltos", crearEquipoSueltoRequest{
		Tipo: "CARGADOR", Nombre: "   ",
	}, "ADMIN")

	// Sin nombre no hay forma de señalarlo en la lista de entregas, que es
	// justo donde hay que elegir cuál se está prestando.
	if codigo != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", codigo)
	}
}

func TestHTTP_ListarEquiposSueltos(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)
	pedir(t, app, "POST", "/api/inventory/equipos/sueltos", crearEquipoSueltoRequest{
		Tipo: "PROYECTOR", Nombre: "Proyector Epson", Reservable: true,
	}, "ADMIN")

	// Un docente también los puede ver: necesita saber que existe un
	// proyector antes de pedirlo (RF-03.7).
	codigo, cuerpo := pedir(t, app, "GET", "/api/inventory/equipos/sueltos", nil, "DOCENTE")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var resp struct {
		Data []equipoResponse `json:"data"`
	}
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Nombre != "Proyector Epson" {
		t.Errorf("listado: %+v", resp.Data)
	}
}

func TestHTTP_CrearEquipo_SoloAdmin(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	codigo, _ := pedir(t, app, "POST", "/api/inventory/equipos/sueltos", crearEquipoSueltoRequest{
		Tipo: "CARGADOR", Nombre: "Cargador 1",
	}, "DOCENTE")

	if codigo != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", codigo)
	}
}
