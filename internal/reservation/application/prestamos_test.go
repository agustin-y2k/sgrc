package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/domain"
)

// El reloj de nuevoServicioDeTest está fijado el lunes 2 de marzo de 2026 a
// las 12:00 UTC.
var mediodiaDeTest = time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)

// servicioConValidador deja tocar qué equipos están fuera del inventario, que
// es lo único que el servicio le pregunta a inventory antes de entregar.
func servicioConValidador(repo Repo, validador *fakeValidadorEquipo) *Service {
	svc := nuevoServicioDeTest(repo)
	svc.validadorEquipo = validador
	return svc
}

// reservaDeTest deja una reserva confirmada de 8 a 9 en el repo.
func reservaDeTest(t *testing.T, repo *fakeRepo, id, equipoID string) *domain.Reserva {
	t.Helper()
	docente := "Ada Lovelace"
	creadoPor := "docente1"
	r, err := domain.NuevaReservaNormal(id, "grupo1", equipoID, "materia1", docente, &creadoPor,
		fecha(2026, time.March, 2), 8*time.Hour, 9*time.Hour, mediodiaDeTest.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas[id] = r
	return r
}

// ── Entrega contra reserva ──────────────────────────────────────────────

func TestEntregarPorReserva_TomaLaHoraDeDevolucionDeLaReserva(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 1 {
		t.Fatalf("esperaba 1 entrega, obtuve %d", len(resultado.Entregadas))
	}
	p := resultado.Entregadas[0]
	// La hora de devolución no se pide: sale del fin de la reserva, que es
	// un dato que ya está y que el Admin no tiene por qué volver a tipear.
	esperado := time.Date(2026, 3, 2, 9, 0, 0, 0, mediodiaDeTest.Location())
	if p.DevolucionEstimada == nil || !p.DevolucionEstimada.Equal(esperado) {
		t.Errorf("devolucionEstimada = %v, esperaba las 9:00 del día de la reserva", p.DevolucionEstimada)
	}
	// Y el docente de la reserva queda como quien la tiene.
	if p.EntregadoANombre != "Ada Lovelace" {
		t.Errorf("nombre = %q, esperaba el docente de la reserva", p.EntregadoANombre)
	}
	if p.EntregadoAUsuarioID == nil || *p.EntregadoAUsuarioID != "docente1" {
		t.Errorf("usuario = %v, esperaba el de la reserva", p.EntregadoAUsuarioID)
	}
	if p.ReservaID == nil || *p.ReservaID != "res1" {
		t.Errorf("no quedó vinculado a la reserva: %v", p.ReservaID)
	}
}

// TestEntregarPorReserva_SeLasLlevoOtraPersona: el docente manda a un alumno
// o a un colega, que es lo habitual.
func TestEntregarPorReserva_SeLasLlevoOtraPersona(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, RetiradoPor: "Juan (alumno)", EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	p := resultado.Entregadas[0]
	if p.EntregadoANombre != "Ada Lovelace" {
		t.Errorf("responsable = %q, esperaba el docente de la reserva", p.EntregadoANombre)
	}
	if p.RetiradoPor != "Juan (alumno)" {
		t.Errorf("retiradoPor = %q, esperaba a quien vino a buscarlas", p.RetiradoPor)
	}
	// Y el aviso de "no volvieron" tiene que llegarle al docente, que es
	// quien responde — no a un alumno que probablemente no tenga cuenta.
	if p.EntregadoAUsuarioID == nil || *p.EntregadoAUsuarioID != "docente1" {
		t.Errorf("usuario = %v, esperaba el de la reserva", p.EntregadoAUsuarioID)
	}
}

// Anotar quién retira es OPCIONAL: a una institución le sirve para
// reconstruir quién pasó por el mostrador y a otra le sobra.
func TestEntregarPorReserva_SinAnotarQuienRetira(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	p := resultado.Entregadas[0]
	if p.EntregadoANombre != "Ada Lovelace" || p.RetiradoPor != "" {
		t.Errorf("esperaba responsable Ada y retiradoPor vacío, obtuve %q / %q",
			p.EntregadoANombre, p.RetiradoPor)
	}
}

// TestEntregarPorReserva_RetiroParcial: reservó cinco y se lleva tres.
// Entregar es máquina por máquina, no de a grupo.
func TestEntregarPorReserva_RetiroParcial(t *testing.T) {
	repo := nuevoFakeRepo()
	for i, equipo := range []string{"pc1", "pc2", "pc3", "pc4", "pc5"} {
		reservaDeTest(t, repo, string(rune('a'+i)), equipo)
	}
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"a", "b", "c"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 3 {
		t.Errorf("esperaba 3 entregas, obtuve %d", len(resultado.Entregadas))
	}
	if len(repo.prestamos) != 3 {
		t.Errorf("las otras dos máquinas no deberían haber salido: %d préstamos", len(repo.prestamos))
	}
}

func TestEntregarPorReserva_ReservaCancelada(t *testing.T) {
	repo := nuevoFakeRepo()
	r := reservaDeTest(t, repo, "res1", "pc1")
	r.Estado = domain.ReservaCancelada
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: se informa, no se rompe: %v", err)
	}
	if len(resultado.Entregadas) != 0 {
		t.Error("no debería entregar contra una reserva cancelada")
	}
	if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != NoEntregadaReservaCancelada {
		t.Errorf("esperaba la razón RESERVA_CANCELADA, obtuve %+v", resultado.NoEntregadas)
	}
}

// TestEntregarPorReserva_UnaFallaNoAbortaElLote es la decisión de diseño
// central de estas operaciones: el Admin tiene las otras máquinas en la mano.
func TestEntregarPorReserva_UnaFallaNoAbortaElLote(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	reservaDeTest(t, repo, "res2", "pc2")
	reservaDeTest(t, repo, "res3", "pc3")
	// La del medio ya está afuera, en manos de otro.
	yaAfuera, err := domain.NuevoPrestamo("pr-previo", domain.DatosDeEntrega{EquipoID: "pc2", Nombre: "Otro"}, mediodiaDeTest)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.prestamos[yaAfuera.ID] = yaAfuera
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1", "res2", "res3"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 2 {
		t.Errorf("esperaba que salieran las otras dos, salieron %d", len(resultado.Entregadas))
	}
	if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != NoEntregadaYaPrestada {
		t.Errorf("esperaba la razón YA_ENTREGADA para pc2, obtuve %+v", resultado.NoEntregadas)
	}
}

func TestEntregarPorReserva_EquipoDadaDeBaja(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := servicioConValidador(repo, &fakeValidadorEquipo{
		disponible:         true,
		fueraDelInventario: map[string]bool{"pc1": true},
	})

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != NoEntregadaFueraDelInventario {
		t.Errorf("esperaba FUERA_DEL_INVENTARIO, obtuve %+v", resultado.NoEntregadas)
	}
}

func TestEntregarPorReserva_SinReservas(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{EntregadoPor: "admin1"})

	if !errors.Is(err, ErrSinEquipos) {
		t.Errorf("esperaba ErrSinEquipos, obtuve %v", err)
	}
}

// Entregar contra una reserva LIBERADA es legítimo: el docente llegó tarde y
// las máquinas seguían ahí.
func TestEntregarPorReserva_LiberadaAvisaSiOtroLaReservo(t *testing.T) {
	repo := nuevoFakeRepo()
	liberada := reservaDeTest(t, repo, "res1", "pc1")
	liberada.Estado = domain.ReservaNoRetirada

	// La que agarró el hueco: de otro docente, todavía por venir.
	delOtro := reservaDeTest(t, repo, "res2", "pc1")
	delOtro.Fecha = fecha(2026, time.March, 3)
	otro := "Grace Hopper"
	delOtro.NombreDocenteSnapshot = &otro

	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 1 {
		t.Fatalf("el aviso no tiene que impedir la entrega: %+v", resultado.NoEntregadas)
	}
	if len(resultado.Avisos) != 1 {
		t.Fatalf("esperaba el aviso de que otro la reservó, obtuve %+v", resultado.Avisos)
	}
	if resultado.Avisos[0].Docente != otro {
		t.Errorf("el aviso tiene que decir de quién es la reserva: %+v", resultado.Avisos[0])
	}
}

// Con la reserva todavía confirmada no hay nada que avisar: la única
// "próxima" que existe es ella misma, y nombrarla sería ruido en la operación
// de todos los días.
func TestEntregarPorReserva_ConfirmadaNoAvisaDeSiMisma(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Avisos) != 0 {
		t.Errorf("no debería avisar nada: %+v", resultado.Avisos)
	}
}

// ── Entrega espontánea ──────────────────────────────────────────────────

func TestEntregarSuelta_SinReservaSinCuentaSinHora(t *testing.T) {
	// El caso del trámite, con los tres campos opcionales vacíos a la vez.
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Marta (secretaría)",
		Motivo: "trámite", EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	p := resultado.Entregadas[0]
	if p.ReservaID != nil || p.EntregadoAUsuarioID != nil || p.DevolucionEstimada != nil {
		t.Errorf("los tres opcionales deberían quedar vacíos: %+v", p)
	}
	if p.EntregadoANombre != "Marta (secretaría)" || p.Motivo != "trámite" {
		t.Errorf("datos de la entrega: %+v", p)
	}
	// Sin hora pactada no se le puede reclamar nada.
	if p.Demorado(mediodiaDeTest.Add(12 * time.Hour)) {
		t.Error("un préstamo sin hora pactada no puede estar demorado")
	}
}

func TestEntregarSuelta_NombreVacio(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "   ", EntregadoPor: "admin1",
	})

	// Falla el lote entero, no una PC: el nombre es el mismo para todas.
	if !errors.Is(err, domain.ErrNombreDestinatarioVacio) {
		t.Errorf("esperaba ErrNombreDestinatarioVacio, obtuve %v", err)
	}
}

// TestEntregarSuelta_AvisaSiElEquipoTieneReservaEncima: no impide la entrega.
func TestEntregarSuelta_AvisaSiElEquipoTieneReservaEncima(t *testing.T) {
	repo := nuevoFakeRepo()
	r := reservaDeTest(t, repo, "res1", "pc1")
	// La reserva tiene que ser futura respecto del reloj del test.
	r.Fecha = fecha(2026, time.March, 3)
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Marta", EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 1 {
		t.Fatal("el aviso no tiene que impedir la entrega")
	}
	if len(resultado.Avisos) != 1 || resultado.Avisos[0].EquipoID != "pc1" {
		t.Fatalf("esperaba un aviso de reserva próxima, obtuve %+v", resultado.Avisos)
	}
	if resultado.Avisos[0].Docente != "Ada Lovelace" {
		t.Errorf("el aviso debería decir de quién es la reserva: %+v", resultado.Avisos[0])
	}
}

func TestEntregarSuelta_SinReservasEncimaNoAvisaNada(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Marta", EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Avisos) != 0 {
		t.Errorf("no debería avisar nada: %+v", resultado.Avisos)
	}
}

// ── Devolución ──────────────────────────────────────────────────────────

// TestEntregaYDevolucion_ElCaminoEsElMismoConReservaYSinElla recorre el ciclo
// completo por los dos caminos y verifica que terminen en el mismo lugar:
// entregar, figurar afuera, marcar la devolución, dejar de figurar.
func TestEntregaYDevolucion_ElCaminoEsElMismoConReservaYSinElla(t *testing.T) {
	ctx := context.Background()
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := nuevoServicioDeTest(repo)

	porReserva, err := svc.EntregarPorReserva(ctx, EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	suelta, err := svc.EntregarSuelta(ctx, EntregaSueltaParams{
		EquipoIDs: []string{"pc2"}, Nombre: "Secretaría", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Las dos salen en la MISMA lista, que es la que dibuja "Afuera del
	// laboratorio" con su botón de recibir.
	abiertos, err := svc.ListarPrestamosAbiertos(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(abiertos) != 2 {
		t.Fatalf("esperaba las dos afuera, obtuve %d", len(abiertos))
	}

	// Y las dos se reciben con la misma operación, en un solo lote: quien
	// las recibe no distingue de dónde vino cada una.
	resultado, err := svc.RecibirEquipos(ctx,
		[]string{porReserva.Entregadas[0].ID, suelta.Entregadas[0].ID}, "admin2", "")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Recibidos) != 2 {
		t.Fatalf("esperaba 2 devoluciones, obtuve %d", len(resultado.Recibidos))
	}

	abiertos, err = svc.ListarPrestamosAbiertos(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(abiertos) != 0 {
		t.Errorf("no debería quedar nada afuera, quedaron %d", len(abiertos))
	}
}

// La única diferencia entre los dos caminos está en lo que se sabe de
// antemano, y conviene fijarla: contra una reserva la hora de devolución sale
// del fin de la clase y no se pide, mientras que una entrega espontánea puede
// no tener ninguna — "vengo en un rato" es la respuesta honesta, y una hora
// inventada solo generaría reclamos falsos.
func TestEntregaYDevolucion_SoloLaDeReservaTraeHoraSinPedirla(t *testing.T) {
	ctx := context.Background()
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := nuevoServicioDeTest(repo)

	porReserva, err := svc.EntregarPorReserva(ctx, EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	suelta, err := svc.EntregarSuelta(ctx, EntregaSueltaParams{
		EquipoIDs: []string{"pc2"}, Nombre: "Secretaría", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if porReserva.Entregadas[0].DevolucionEstimada == nil {
		t.Error("la entrega contra reserva debería traer la hora del fin de la clase")
	}
	if suelta.Entregadas[0].DevolucionEstimada != nil {
		t.Error("una entrega espontánea sin hora pactada no debería inventar una")
	}
}

func TestRecibirEquipos_VariasDeUnaVez(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	entrega, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1", "pc2", "pc3"}, Nombre: "Ada", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	ids := []string{entrega.Entregadas[0].ID, entrega.Entregadas[1].ID, entrega.Entregadas[2].ID}

	resultado, err := svc.RecibirEquipos(context.Background(), ids, "admin2", "")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Recibidos) != 3 {
		t.Fatalf("esperaba 3 devoluciones, obtuve %d", len(resultado.Recibidos))
	}
	abiertos, err := svc.ListarPrestamosAbiertos(context.Background())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(abiertos) != 0 {
		t.Errorf("no debería quedar nada afuera, quedaron %d", len(abiertos))
	}
}

// TestRecibirEquipos_FaltoUno: el caso que planteó la escuela. Devolver tres de
// cuatro no necesita nada especial — la cuarta simplemente sigue abierta.
func TestRecibirEquipos_FaltoUna(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	entrega, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1", "pc2", "pc3", "pc4"}, Nombre: "Ada", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if _, err := svc.RecibirEquipos(context.Background(),
		[]string{entrega.Entregadas[0].ID, entrega.Entregadas[1].ID, entrega.Entregadas[2].ID},
		"admin2", ""); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	abiertos, err := svc.ListarPrestamosAbiertos(context.Background())
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(abiertos) != 1 || abiertos[0].Prestamo.EquipoID != "pc4" {
		t.Errorf("la que falta debería seguir figurando afuera: %+v", abiertos)
	}
}

func TestRecibirEquipos_ObservacionesYQuienRecibio(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	entrega, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Ada", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	resultado, err := svc.RecibirEquipos(context.Background(),
		[]string{entrega.Entregadas[0].ID}, "admin2", "volvió sin el cargador")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	p := resultado.Recibidos[0]
	if p.Observaciones != "volvió sin el cargador" {
		t.Errorf("observaciones = %q", p.Observaciones)
	}
	if p.RecibidoPor == nil || *p.RecibidoPor != "admin2" {
		t.Errorf("no quedó registrado quién la recibió: %v", p.RecibidoPor)
	}
	if p.DevueltoEn == nil || !p.DevueltoEn.Equal(mediodiaDeTest) {
		t.Errorf("devueltoEn = %v", p.DevueltoEn)
	}
}

// TestRecibirEquipos_DosVecesSeInforma: dos Admin en el mostrador o un doble
// clic.
func TestRecibirEquipos_DosVecesSeInforma(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	entrega, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Ada", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	id := entrega.Entregadas[0].ID
	if _, err := svc.RecibirEquipos(context.Background(), []string{id}, "admin2", ""); err != nil {
		t.Fatalf("la primera no debería fallar: %v", err)
	}

	resultado, err := svc.RecibirEquipos(context.Background(), []string{id}, "admin3", "")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Recibidos) != 0 || len(resultado.NoRecibidos) != 1 {
		t.Errorf("esperaba que se informara como ya devuelta: %+v", resultado)
	}
}

// TestEntregarDevolverYEntregarDeNuevo: el ciclo completo de una máquina en
// un día, que es lo que hoy hace el papel.
func TestEntregarDevolverYEntregarDeNuevo(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := nuevoServicioDeTest(repo)
	ctx := context.Background()

	primera, err := svc.EntregarSuelta(ctx, EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Ada", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Mientras está afuera, no se puede volver a entregar.
	segunda, err := svc.EntregarSuelta(ctx, EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Marta", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(segunda.Entregadas) != 0 || len(segunda.NoEntregadas) != 1 {
		t.Fatalf("una máquina afuera no se puede entregar de nuevo: %+v", segunda)
	}

	if _, err := svc.RecibirEquipos(ctx, []string{primera.Entregadas[0].ID}, "admin1", ""); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	tercera, err := svc.EntregarSuelta(ctx, EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Marta", EntregadoPor: "admin1",
	})
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(tercera.Entregadas) != 1 {
		t.Errorf("tras volver, la máquina se puede entregar de nuevo: %+v", tercera)
	}

	historial, err := svc.HistorialDeEquipo(ctx, "pc1")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(historial) != 2 {
		t.Errorf("el historial debería tener las dos salidas: %d", len(historial))
	}
}

// ── Regresiones encontradas al revisar ──────────────────────────────────

// TestEntregarPorReserva_BloqueoSinDocenteNoRompeElLote Un bloqueo
// administrativo (RF-04.7) no tiene docente: NuevaReservaBloqueo no recibe
// nombre, así que NombreDocenteSnapshot es nil.
func TestEntregarPorReserva_BloqueoSinDocenteNoRompeElLote(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	bloqueo, err := domain.NuevaReservaBloqueo("bloq1", "pc2", nil,
		fecha(2026, time.March, 2), 8*time.Hour, 9*time.Hour, "Jornada docente", mediodiaDeTest.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas["bloq1"] = bloqueo
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1", "bloq1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("un bloqueo sin docente no puede tumbar el lote entero: %v", err)
	}
	if len(resultado.Entregadas) != 1 || resultado.Entregadas[0].EquipoID != "pc1" {
		t.Errorf("la reserva con docente tenía que salir igual: %+v", resultado.Entregadas)
	}
	if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].EquipoID != "pc2" {
		t.Errorf("el bloqueo tenía que informarse, no romper: %+v", resultado.NoEntregadas)
	}
}

// TestEntregarPorReserva_BloqueoConNombreSiSeEntrega: con un nombre escrito a
// mano sí se puede — alguien tiene que retirar las máquinas de una mesa de
// examen.
func TestEntregarPorReserva_BloqueoConNombreSiSeEntrega(t *testing.T) {
	repo := nuevoFakeRepo()
	bloqueo, err := domain.NuevaReservaBloqueo("bloq1", "pc1", nil,
		fecha(2026, time.March, 2), 8*time.Hour, 9*time.Hour, "Jornada docente", mediodiaDeTest.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.reservas["bloq1"] = bloqueo
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"bloq1"}, RetiradoPor: "Mesa de examen", EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 1 {
		t.Fatalf("con nombre tiene que poder entregarse: %+v", resultado)
	}
	// Un bloqueo no tiene docente, así que no hay a quién hacer responsable:
	// el nombre escrito a mano ES el responsable, y no queda nadie "al lado".
	p := resultado.Entregadas[0]
	if p.EntregadoANombre != "Mesa de examen" || p.RetiradoPor != "" {
		t.Errorf("esperaba responsable \"Mesa de examen\" sin retiradoPor, obtuve %q / %q",
			p.EntregadoANombre, p.RetiradoPor)
	}
}

// TestEntregarSuelta_AvisaDeLaReservaMasProxima reservaProximaDe tomaba
// futuras[0] dando por sentado que la consulta venía ordenada, y no lo
// estaba.
func TestEntregarSuelta_AvisaDeLaReservaMasProxima(t *testing.T) {
	repo := nuevoFakeRepo()
	lejana := reservaDeTest(t, repo, "lejana", "pc1")
	lejana.Fecha = fecha(2026, time.March, 20)
	proxima := reservaDeTest(t, repo, "proxima", "pc1")
	proxima.Fecha = fecha(2026, time.March, 3)
	svc := nuevoServicioDeTest(repo)

	resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Marta", EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Avisos) != 1 {
		t.Fatalf("esperaba un aviso, obtuve %+v", resultado.Avisos)
	}
	if !resultado.Avisos[0].Fecha.Equal(fecha(2026, time.March, 3)) {
		t.Errorf("el aviso nombró la reserva del %v; tiene que ser la más próxima (3 de marzo)",
			resultado.Avisos[0].Fecha)
	}
}

// ── Lo que no está en circulación no sale (RF-08.17) ────────────────────

// Antes salía: el mostrador validaba que el equipo estuviera en el inventario
// y no miraba su estado, así que una máquina que un Admin acababa de marcar
// como rota se podía prestar igual.
func TestEntregarSuelta_LoQueNoEstaDisponibleNoSale(t *testing.T) {
	for _, caso := range []struct{ estado, esperado string }{
		{"EN_MANTENIMIENTO", "en mantenimiento"},
		{"FUERA_DE_SERVICIO", "fuera de servicio"},
	} {
		t.Run(caso.estado, func(t *testing.T) {
			repo := nuevoFakeRepo()
			svc := servicioConValidador(repo, &fakeValidadorEquipo{
				disponible: true,
				estados:    map[string]string{"pc1": caso.estado},
			})

			resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
				EquipoIDs: []string{"pc1"}, Nombre: "Marta (secretaría)", EntregadoPor: "admin1",
			})

			if err != nil {
				t.Fatalf("no debería fallar el lote entero: %v", err)
			}
			if len(resultado.Entregadas) != 0 {
				t.Fatalf("no debería haber salido nada: %+v", resultado.Entregadas)
			}
			if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != NoEntregadaFueraDeCirculacion {
				t.Fatalf("esperaba FUERA_DE_CIRCULACION, obtuve %+v", resultado.NoEntregadas)
			}
			// El detalle nombra el hecho, no el código: lo lee quien está en el
			// mostrador con alguien esperando enfrente.
			if !strings.Contains(resultado.NoEntregadas[0].Detalle, caso.esperado) {
				t.Errorf("detalle = %q, esperaba que dijera %q", resultado.NoEntregadas[0].Detalle, caso.esperado)
			}
		})
	}
}

// Una máquina del lote en mantenimiento no impide que salgan las demás:
// mismo criterio que el resto de los rechazos por equipo.
func TestEntregarSuelta_UnaFueraDeCirculacionNoAbortaElLote(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioConValidador(repo, &fakeValidadorEquipo{
		disponible: true,
		estados:    map[string]string{"pc2": "EN_MANTENIMIENTO"},
	})

	resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1", "pc2", "pc3"}, Nombre: "Marta", EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 2 {
		t.Errorf("esperaba que salieran las otras 2, obtuve %d", len(resultado.Entregadas))
	}
	if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].EquipoID != "pc2" {
		t.Errorf("esperaba solo pc2 rechazada, obtuve %+v", resultado.NoEntregadas)
	}
}

// La salida a reparación es el único camino para sacar del laboratorio algo
// que no está en condiciones de prestarse: sale, y queda la constancia.
func TestEntregarSuelta_SalidaAReparacionSacaLoQueNoCircula(t *testing.T) {
	repo := nuevoFakeRepo()
	svc := servicioConValidador(repo, &fakeValidadorEquipo{
		disponible: true,
		estados:    map[string]string{"pc1": "FUERA_DE_SERVICIO"},
	})

	resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Service Rossi", Motivo: "no enciende",
		SalidaAReparacion: true, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Entregadas) != 1 {
		t.Fatalf("esperaba que saliera: %+v", resultado.NoEntregadas)
	}
	if resultado.Entregadas[0].Motivo != "no enciende" {
		t.Errorf("motivo = %q, es la constancia de a dónde fue", resultado.Entregadas[0].Motivo)
	}
}

// Sin motivo no hay constancia, y sin constancia la salida a reparación es
// un préstamo común mal cargado. Falla el lote entero: el motivo es uno solo
// para todas las máquinas.
func TestEntregarSuelta_SalidaAReparacionSinMotivo(t *testing.T) {
	svc := nuevoServicioDeTest(nuevoFakeRepo())

	_, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Service Rossi", Motivo: "  ",
		SalidaAReparacion: true, EntregadoPor: "admin1",
	})

	if !errors.Is(err, ErrSalidaAReparacionSinMotivo) {
		t.Errorf("esperaba ErrSalidaAReparacionSinMotivo, obtuve %v", err)
	}
}

// Ni siquiera la salida a reparación saca algo dado de baja: ya no es parte
// del parque, y lo que sale del laboratorio es lo que hay que esperar de
// vuelta.
func TestEntregarSuelta_SalidaAReparacionNoSacaLoDadoDeBaja(t *testing.T) {
	svc := servicioConValidador(nuevoFakeRepo(), &fakeValidadorEquipo{
		disponible:         true,
		fueraDelInventario: map[string]bool{"pc1": true},
	})

	resultado, err := svc.EntregarSuelta(context.Background(), EntregaSueltaParams{
		EquipoIDs: []string{"pc1"}, Nombre: "Service Rossi", Motivo: "no enciende",
		SalidaAReparacion: true, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != NoEntregadaFueraDelInventario {
		t.Errorf("esperaba FUERA_DEL_INVENTARIO, obtuve %+v", resultado.NoEntregadas)
	}
}

// Una reserva tampoco autoriza sacar un equipo roto: si dejó de estar
// disponible, sus reservas se cancelaron en cascada (RF-03.8) y esta entrega
// llega tarde.
func TestEntregarPorReserva_NoSaleLoQueEstaEnMantenimiento(t *testing.T) {
	repo := nuevoFakeRepo()
	reservaDeTest(t, repo, "res1", "pc1")
	svc := servicioConValidador(repo, &fakeValidadorEquipo{
		disponible: true,
		estados:    map[string]string{"pc1": "EN_MANTENIMIENTO"},
	})

	resultado, err := svc.EntregarPorReserva(context.Background(), EntregaPorReservaParams{
		ReservaIDs: []string{"res1"}, EntregadoPor: "admin1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.NoEntregadas) != 1 || resultado.NoEntregadas[0].Razon != NoEntregadaFueraDeCirculacion {
		t.Errorf("esperaba FUERA_DE_CIRCULACION, obtuve %+v", resultado.NoEntregadas)
	}
}
