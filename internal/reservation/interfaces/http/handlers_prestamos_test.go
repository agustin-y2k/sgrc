package http

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

func pedirPrestamos(t *testing.T, app *fiber.App, metodo, ruta string, cuerpo any, rol string) (int, []byte) {
	t.Helper()
	r := httptest.NewRequest(metodo, ruta, jsonBody(cuerpo))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+tokenPara("admin1", rol))

	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	defer resp.Body.Close()
	leido, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("no se pudo leer la respuesta: %v", err)
	}
	return resp.StatusCode, leido
}

func TestHTTP_EntregarSuelta(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos", entregarSueltaRequest{
		EquipoIDs: []string{"pc1"}, Nombre: "Marta (secretaría)", Motivo: "trámite",
	}, "ADMIN")

	if codigo != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}
	var resp resultadoEntregaResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Entregadas) != 1 {
		t.Fatalf("esperaba 1 entrega, obtuve %d", len(resp.Entregadas))
	}
	p := resp.Entregadas[0]
	if p.EntregadoANombre != "Marta (secretaría)" || !p.Abierto || p.Demorado {
		t.Errorf("datos de la entrega: %+v", p)
	}
	// Sin hora pactada no se le puede reclamar nada, y el campo no viaja.
	if p.DevolucionEstimada != nil {
		t.Errorf("no debería haber hora de devolución: %v", p.DevolucionEstimada)
	}
}

func TestHTTP_EntregarSuelta_SinNombre(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	codigo, _ := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos", entregarSueltaRequest{
		EquipoIDs: []string{"pc1"}, Nombre: "   ",
	}, "ADMIN")

	if codigo != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", codigo)
	}
}

// TestHTTP_EntregarSuelta_EquipoYaAfuera: 201 con el detalle en el cuerpo, no
// 409. El lote se procesó; un conflicto obligaría a la pantalla a deshacer
// las máquinas que sí se entregaron.
func TestHTTP_EntregarSuelta_EquipoYaAfuera(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)
	req := entregarSueltaRequest{EquipoIDs: []string{"pc1"}, Nombre: "Ada"}

	if codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos", req, "ADMIN"); codigo != fiber.StatusCreated {
		t.Fatalf("la primera: esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}

	codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos", req, "ADMIN")

	if codigo != fiber.StatusCreated {
		t.Fatalf("esperaba 201, obtuve %d: %s", codigo, cuerpo)
	}
	var resp resultadoEntregaResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Entregadas) != 0 || len(resp.NoEntregadas) != 1 {
		t.Fatalf("esperaba 0 entregadas y 1 informada: %+v", resp)
	}
	// El código permite que la pantalla ofrezca "ver quién la tiene" en vez
	// de un cartel genérico.
	if resp.NoEntregadas[0].Razon != "YA_ENTREGADA" {
		t.Errorf("razon = %q, esperaba YA_ENTREGADA", resp.NoEntregadas[0].Razon)
	}
}

func TestHTTP_EntregarPorReserva(t *testing.T) {
	repo := nuevoFakeRepo()
	docente := "Ada Lovelace"
	creadoPor := "docente1"
	r, err := domain.NuevaReservaNormal("res1", "grupo1", "pc1", "materia1", docente, &creadoPor,
		time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour,
		time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas["res1"] = r
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos/por-reserva",
		entregarPorReservaRequest{ReservaIDs: []string{"res1"}}, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var resp resultadoEntregaResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Entregadas) != 1 {
		t.Fatalf("esperaba 1 entrega, obtuve %d", len(resp.Entregadas))
	}
	// La hora de devolución sale del fin de la reserva, no se pide.
	if resp.Entregadas[0].DevolucionEstimada == nil {
		t.Error("una entrega contra reserva tiene que traer la hora de devolución")
	}
	if resp.Entregadas[0].EntregadoANombre != docente {
		t.Errorf("nombre = %q, esperaba el docente de la reserva", resp.Entregadas[0].EntregadoANombre)
	}
}

func TestHTTP_RecibirYListarLoQueEstaAfuera(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)

	_, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos", entregarSueltaRequest{
		EquipoIDs: []string{"pc1", "pc2"}, Nombre: "Ada",
	}, "ADMIN")
	var entrega resultadoEntregaResponse
	if err := json.Unmarshal(cuerpo, &entrega); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}

	codigo, cuerpo := pedirPrestamos(t, app, "GET", "/api/reservation/prestamos", nil, "ADMIN")
	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var listado struct {
		Data []prestamoResponse `json:"data"`
	}
	if err := json.Unmarshal(cuerpo, &listado); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(listado.Data) != 2 {
		t.Fatalf("esperaba 2 máquinas afuera, obtuve %d", len(listado.Data))
	}
	// La ubicación viaja en el listado: un renglón que dice "entregada a
	// Ada" sin decir qué computadora no sirve para nada.
	if listado.Data[0].Identificador == 0 || listado.Data[0].CarroNombre == "" {
		t.Errorf("falta la ubicación: %+v", listado.Data[0])
	}

	// Se devuelve una sola: la otra sigue afuera.
	codigo, cuerpo = pedirPrestamos(t, app, "POST", "/api/reservation/prestamos/recibir", recibirRequest{
		PrestamoIDs: []string{entrega.Entregadas[0].ID}, Observaciones: "volvió sin el cargador",
	}, "ADMIN")
	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var devolucion resultadoDevolucionResponse
	if err := json.Unmarshal(cuerpo, &devolucion); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(devolucion.Recibidos) != 1 || devolucion.Recibidos[0].Observaciones != "volvió sin el cargador" {
		t.Errorf("devolución: %+v", devolucion)
	}
	if devolucion.Recibidos[0].Abierto {
		t.Error("una máquina devuelta no está abierta")
	}

	_, cuerpo = pedirPrestamos(t, app, "GET", "/api/reservation/prestamos", nil, "ADMIN")
	if err := json.Unmarshal(cuerpo, &listado); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(listado.Data) != 1 {
		t.Errorf("debería quedar 1 máquina afuera, quedaron %d", len(listado.Data))
	}
}

func TestHTTP_Recibir_DosVecesSeInforma(t *testing.T) {
	repo := nuevoFakeRepo()
	app := nuevaAppDeTest(repo)
	_, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos", entregarSueltaRequest{
		EquipoIDs: []string{"pc1"}, Nombre: "Ada",
	}, "ADMIN")
	var entrega resultadoEntregaResponse
	if err := json.Unmarshal(cuerpo, &entrega); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	req := recibirRequest{PrestamoIDs: []string{entrega.Entregadas[0].ID}}
	pedirPrestamos(t, app, "POST", "/api/reservation/prestamos/recibir", req, "ADMIN")

	codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos/recibir", req, "ADMIN")

	// 200: lo que el Admin quería —que la máquina figure adentro— ya pasó.
	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var resp resultadoDevolucionResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Recibidos) != 0 || len(resp.NoRecibidos) != 1 {
		t.Errorf("esperaba que se informara como ya devuelta: %+v", resp)
	}
}

func TestHTTP_Recibir_PrestamoInexistente(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	codigo, _ := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos/recibir", recibirRequest{
		PrestamoIDs: []string{"no-existe"},
	}, "ADMIN")

	if codigo != fiber.StatusNotFound {
		t.Fatalf("esperaba 404, obtuve %d", codigo)
	}
}

// TestHTTP_Prestamos_SoloAdmin: quien entrega y recibe es quien hoy escribe
// el papel. Un docente que pudiera marcarse la entrega a sí mismo
// convertiría el registro en una declaración en vez de en una constancia.
func TestHTTP_Prestamos_SoloAdmin(t *testing.T) {
	app := nuevaAppDeTest(nuevoFakeRepo())

	rutas := []struct{ metodo, ruta string }{
		{"GET", "/api/reservation/prestamos"},
		{"POST", "/api/reservation/prestamos"},
		{"POST", "/api/reservation/prestamos/por-reserva"},
		{"POST", "/api/reservation/prestamos/recibir"},
		{"GET", "/api/reservation/equipos/pc1/prestamos"},
	}

	for _, r := range rutas {
		codigo, _ := pedirPrestamos(t, app, r.metodo, r.ruta, nil, "DOCENTE")
		if codigo != fiber.StatusForbidden {
			t.Errorf("%s %s: esperaba 403 para un DOCENTE, obtuve %d", r.metodo, r.ruta, codigo)
		}
	}
}

// TestHTTP_EntregarPorReserva_BloqueoSinDocente: el error de un bloqueo sin
// docente sale como razón por PC, no como un 400 que tumba el lote.
func TestHTTP_EntregarPorReserva_BloqueoSinDocente(t *testing.T) {
	repo := nuevoFakeRepo()
	bloqueo, err := domain.NuevaReservaBloqueo("bloq1", "pc1", nil,
		time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour,
		"Jornada docente", time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas["bloq1"] = bloqueo
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedirPrestamos(t, app, "POST", "/api/reservation/prestamos/por-reserva",
		entregarPorReservaRequest{ReservaIDs: []string{"bloq1"}}, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	var resp resultadoEntregaResponse
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.NoEntregadas) != 1 || resp.NoEntregadas[0].Razon != "SIN_DESTINATARIO" {
		t.Errorf("esperaba SIN_DESTINATARIO, obtuve %+v", resp.NoEntregadas)
	}

	// Con un nombre a mano sí sale.
	codigo, cuerpo = pedirPrestamos(t, app, "POST", "/api/reservation/prestamos/por-reserva",
		entregarPorReservaRequest{ReservaIDs: []string{"bloq1"}, RetiradoPor: "Mesa de examen"}, "ADMIN")
	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if len(resp.Entregadas) != 1 {
		t.Errorf("con nombre tiene que entregarse: %+v", resp)
	}
}

// ── Cambiar una PC de una reserva (RF-08.14) ────────────────────────────

func reservaEnRepo(t *testing.T, repo *fakeRepo, id, equipoID, creadoPor string) {
	t.Helper()
	docente := "Ada Lovelace"
	r, err := domain.NuevaReservaNormal(id, "grupo1", equipoID, "materia1", docente, &creadoPor,
		time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), 8*time.Hour, 9*time.Hour,
		time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas[id] = r
}

func TestHTTP_CambiarEquipoDeReserva(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaEnRepo(t, repo, "res1", "pc1", "admin1")
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedirPrestamos(t, app, "PATCH", "/api/reservation/reservas/res1/equipo",
		cambiarEquipoRequest{EquipoID: "pc9"}, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	// La misma reserva, otra máquina: no se creó un grupo nuevo.
	if repo.reservas["res1"].EquipoID != "pc9" {
		t.Errorf("la PC quedó en %q", repo.reservas["res1"].EquipoID)
	}
	if repo.reservas["res1"].ReservaGrupoID == nil || *repo.reservas["res1"].ReservaGrupoID != "grupo1" {
		t.Error("la clase no puede quedar partida en dos grupos")
	}
}

func TestHTTP_CambiarEquipoDeReserva_Ajena(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaEnRepo(t, repo, "res1", "pc1", "otro-docente")
	app := nuevaAppDeTest(repo)

	codigo, _ := pedirPrestamos(t, app, "PATCH", "/api/reservation/reservas/res1/equipo",
		cambiarEquipoRequest{EquipoID: "pc9"}, "DOCENTE")

	if codigo != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", codigo)
	}
}

func TestHTTP_CambiarEquipoDeReserva_SinEquipo(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaEnRepo(t, repo, "res1", "pc1", "admin1")
	app := nuevaAppDeTest(repo)

	codigo, _ := pedirPrestamos(t, app, "PATCH", "/api/reservation/reservas/res1/equipo",
		cambiarEquipoRequest{EquipoID: "  "}, "ADMIN")

	if codigo != fiber.StatusBadRequest {
		t.Fatalf("esperaba 400, obtuve %d", codigo)
	}
}

// Una reserva liberada por no retiro ya no reserva nada: cambiarle la
// máquina no significaría nada.
func TestHTTP_CambiarEquipoDeReserva_YaLiberada(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaEnRepo(t, repo, "res1", "pc1", "admin1")
	if err := repo.reservas["res1"].Liberar(); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	app := nuevaAppDeTest(repo)

	codigo, _ := pedirPrestamos(t, app, "PATCH", "/api/reservation/reservas/res1/equipo",
		cambiarEquipoRequest{EquipoID: "pc9"}, "ADMIN")

	if codigo != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", codigo)
	}
}

// El cuerpo sin `soloEsta` tiene que cambiar UNA reserva y no la serie entera
// hasta fin de año: ante la duda, el cambio más chico.
func TestHTTP_CambiarEquipoDeReserva_SinAlcanceEsSoloEsta(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaEnRepo(t, repo, "res1", "pc1", "admin1")
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedirPrestamos(t, app, "PATCH", "/api/reservation/reservas/res1/equipo",
		map[string]string{"equipoId": "pc9"}, "ADMIN")

	if codigo != fiber.StatusOK {
		t.Fatalf("esperaba 200, obtuve %d: %s", codigo, cuerpo)
	}
	if repo.reservas["res1"].EquipoID != "pc9" {
		t.Errorf("la PC quedó en %q", repo.reservas["res1"].EquipoID)
	}
}

// ── Pedirle a otro que libere (RF-04.12) ────────────────────────────────

func datosDePedido(dueno string) *application.ReservaParaPedido {
	duenoID := dueno
	return &application.ReservaParaPedido{
		Estado:        domain.ReservaConfirmada,
		DuenoID:       &duenoID,
		DuenoNombre:   "Ada Lovelace",
		DuenoEmail:    "ada@escuela.edu.ar",
		Etiqueta:      "PC 3",
		MateriaNombre: "Matemáticas",
		// Pasado mañana respecto del reloj fijo de los tests: la franja
		// todavía no empezó.
		Fecha:      time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
		HoraInicio: 10 * time.Hour,
		HoraFin:    12 * time.Hour,
	}
}

func TestHTTP_PedirLiberacion(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.datosDelPedido = datosDePedido("otro-docente")
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedirPrestamos(t, app, "POST",
		"/api/reservation/reservas/res1/pedido-de-liberacion",
		pedirLiberacionRequest{Mensaje: "La necesito para una evaluación"}, "DOCENTE")

	if codigo != fiber.StatusAccepted {
		t.Fatalf("esperaba 202, obtuve %d: %s", codigo, cuerpo)
	}
}

// Pedirse a uno mismo no tiene sentido: no hay a quién avisarle.
func TestHTTP_PedirLiberacion_ReservaPropia(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.datosDelPedido = datosDePedido("admin1") // el mismo que manda el pedido
	app := nuevaAppDeTest(repo)

	codigo, _ := pedirPrestamos(t, app, "POST",
		"/api/reservation/reservas/res1/pedido-de-liberacion",
		pedirLiberacionRequest{}, "DOCENTE")

	if codigo != fiber.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", codigo)
	}
}

// El segundo correo idéntico es presión, no aviso.
func TestHTTP_PedirLiberacion_RepetidoElMismoDia(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.datosDelPedido = datosDePedido("otro-docente")
	repo.yaPidio = true
	app := nuevaAppDeTest(repo)

	codigo, _ := pedirPrestamos(t, app, "POST",
		"/api/reservation/reservas/res1/pedido-de-liberacion",
		pedirLiberacionRequest{}, "DOCENTE")

	if codigo != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", codigo)
	}
}

// A un bloqueo administrativo no se le pide nada: no tiene docente detrás.
func TestHTTP_PedirLiberacion_SobreUnBloqueo(t *testing.T) {
	repo := nuevoFakeRepo()
	datos := datosDePedido("otro-docente")
	datos.EsBloqueo = true
	datos.DuenoID = nil
	repo.datosDelPedido = datos
	app := nuevaAppDeTest(repo)

	codigo, _ := pedirPrestamos(t, app, "POST",
		"/api/reservation/reservas/res1/pedido-de-liberacion",
		pedirLiberacionRequest{}, "DOCENTE")

	if codigo != fiber.StatusConflict {
		t.Fatalf("esperaba 409, obtuve %d", codigo)
	}
}

// Pedir sin escribir nada es válido: el texto libre es opcional.
func TestHTTP_PedirLiberacion_SinCuerpo(t *testing.T) {
	repo := nuevoFakeRepo()
	repo.datosDelPedido = datosDePedido("otro-docente")
	app := nuevaAppDeTest(repo)

	codigo, cuerpo := pedirPrestamos(t, app, "POST",
		"/api/reservation/reservas/res1/pedido-de-liberacion", nil, "DOCENTE")

	if codigo != fiber.StatusAccepted {
		t.Fatalf("esperaba 202, obtuve %d: %s", codigo, cuerpo)
	}
}
