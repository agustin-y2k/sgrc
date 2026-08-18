//go:build integration

package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/reservation/application"
	"github.com/ramiro/sgrc/internal/reservation/domain"
)

func diaDe(anio int, mes time.Month, dia int) time.Time {
	return time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)
}

func entregaDeTest(equipoID string) domain.DatosDeEntrega {
	return domain.DatosDeEntrega{EquipoID: equipoID, Nombre: "Ana Pérez"}
}

func crearPrestamoDeTest(t *testing.T, repo *PostgresRepo, d domain.DatosDeEntrega, ahora time.Time) *domain.Prestamo {
	t.Helper()
	p, err := domain.NuevoPrestamo(NuevoID(), d, ahora)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearPrestamo(context.Background(), p); err != nil {
		t.Fatalf("no se pudo registrar la entrega de prueba: %v", err)
	}
	return p
}

func TestPostgresRepo_Prestamo_EspontaneoIdaYVuelta(t *testing.T) {
	// El caso del trámite: sin reserva, sin cuenta, sin hora de devolución.
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	d := entregaDeTest(equipoID)
	d.Motivo = "trámite en secretaría"
	p := crearPrestamoDeTest(t, repo, d, ahora)

	encontrado, err := repo.BuscarPrestamoPorID(ctx, p.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrado.ReservaID != nil || encontrado.EntregadoAUsuarioID != nil || encontrado.DevolucionEstimada != nil {
		t.Errorf("los tres campos opcionales deberían volver nulos: %+v", encontrado)
	}
	if encontrado.EntregadoANombre != "Ana Pérez" || encontrado.Motivo != "trámite en secretaría" {
		t.Errorf("datos de la entrega no coinciden: %+v", encontrado)
	}
	if !encontrado.EstaAbierto() {
		t.Error("recién entregada, el préstamo tiene que estar abierto")
	}
}

// TestPostgresRepo_Prestamo_UnEquipoNoPuedeEstarEnDosManos verifica contra
// Postgres real la garantía que el papel no puede dar. Dos Admin anotando a
// la vez, o un doble clic, no pueden entregar dos veces la misma máquina.
func TestPostgresRepo_Prestamo_UnaEquipoNoPuedeEstarEnDosManos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	crearPrestamoDeTest(t, repo, entregaDeTest(equipoID), ahora)

	otro, err := domain.NuevoPrestamo(NuevoID(), entregaDeTest(equipoID), ahora)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearPrestamo(ctx, otro); err != application.ErrEquipoYaPrestado {
		t.Fatalf("esperaba ErrEquipoYaPrestado, obtuve %v", err)
	}
}

// TestPostgresRepo_Prestamo_TrasDevolverSePuedeVolverAEntregar: el índice
// único es PARCIAL, así que la misma PC puede prestarse cien veces mientras
// no haya dos abiertas.
func TestPostgresRepo_Prestamo_TrasDevolverSePuedeVolverAEntregar(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	primero := crearPrestamoDeTest(t, repo, entregaDeTest(equipoID), ahora)
	if err := primero.Devolver("", "", ahora.Add(time.Hour)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.GuardarPrestamo(ctx, primero); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	crearPrestamoDeTest(t, repo, entregaDeTest(equipoID), ahora.Add(2*time.Hour))

	historial, err := repo.ListarPrestamosDeEquipo(ctx, equipoID, 10)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(historial) != 2 {
		t.Fatalf("esperaba 2 entregas en el historial, obtuve %d", len(historial))
	}
	// De lo más reciente a lo más viejo.
	if !historial[0].Prestamo.EstaAbierto() || historial[1].Prestamo.EstaAbierto() {
		t.Error("el historial debería venir de la más reciente (abierta) a la más vieja (devuelta)")
	}
}

func TestPostgresRepo_Prestamo_BuscarAbiertoDeEquipo(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// Con la máquina en el laboratorio, no hay préstamo abierto.
	if _, err := repo.BuscarPrestamoAbiertoDeEquipo(ctx, equipoID); err != application.ErrPrestamoNoEncontrado {
		t.Fatalf("esperaba ErrPrestamoNoEncontrado, obtuve %v", err)
	}

	p := crearPrestamoDeTest(t, repo, entregaDeTest(equipoID), ahora)

	abierto, err := repo.BuscarPrestamoAbiertoDeEquipo(ctx, equipoID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if abierto.ID != p.ID {
		t.Errorf("devolvió otro préstamo: %s", abierto.ID)
	}

	// Y al devolverla vuelve a no haber ninguno abierto: el estado se
	// deriva de la tabla, no de una columna que haya que acordarse de
	// actualizar.
	if err := abierto.Devolver("", "", ahora.Add(time.Hour)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.GuardarPrestamo(ctx, abierto); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if _, err := repo.BuscarPrestamoAbiertoDeEquipo(ctx, equipoID); err != application.ErrPrestamoNoEncontrado {
		t.Errorf("tras devolver no debería quedar ningún préstamo abierto, obtuve %v", err)
	}
}

func TestPostgresRepo_Prestamo_GuardarRegistraLaDevolucion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	adminID := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	p := crearPrestamoDeTest(t, repo, entregaDeTest(equipoID), ahora)
	devuelta := ahora.Add(90 * time.Minute)
	if err := p.Devolver(adminID, "volvió sin el cargador", devuelta); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.GuardarPrestamo(ctx, p); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrado, err := repo.BuscarPrestamoPorID(ctx, p.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrado.DevueltoEn == nil || !encontrado.DevueltoEn.Equal(devuelta) {
		t.Errorf("devueltoEn = %v, esperaba %v", encontrado.DevueltoEn, devuelta)
	}
	if encontrado.RecibidoPor == nil || *encontrado.RecibidoPor != adminID {
		t.Errorf("no quedó registrado quién la recibió: %v", encontrado.RecibidoPor)
	}
	if encontrado.Observaciones != "volvió sin el cargador" {
		t.Errorf("observaciones = %q", encontrado.Observaciones)
	}
}

// TestPostgresRepo_Prestamo_AbiertosTraenUbicacionYMateria: el listado que
// reemplaza al papel. Un renglón que dice "entregada a Ana Pérez" sin decir
// qué computadora no sirve para nada.
func TestPostgresRepo_Prestamo_AbiertosTraenUbicacionYMateria(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	materiaID := crearMateriaDeTest(t, pool)
	equipoConReserva := crearEquipoDeCarroDeTest(t, pool)
	equipoSuelto := crearEquipoDeCarroDeTest(t, pool)

	grupo := nuevoReservaGrupoDeTest(materiaID, ahora, 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, grupo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	reserva, err := domain.NuevaReservaNormal(NuevoID(), grupo.ID, equipoConReserva, materiaID,
		"Ada Lovelace", nil, ahora, 8*time.Hour, 9*time.Hour, ahora.Add(-time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, reserva); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// La de la reserva vence antes, así que tiene que salir primera.
	vence := ahora.Add(time.Hour)
	dConReserva := entregaDeTest(equipoConReserva)
	dConReserva.ReservaID = &reserva.ID
	dConReserva.DevolucionEstimada = &vence
	crearPrestamoDeTest(t, repo, dConReserva, ahora)

	// La suelta no tiene hora pactada: va al final.
	crearPrestamoDeTest(t, repo, entregaDeTest(equipoSuelto), ahora)

	abiertos, err := repo.ListarPrestamosAbiertos(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(abiertos) != 2 {
		t.Fatalf("esperaba 2 préstamos abiertos, obtuve %d", len(abiertos))
	}

	if abiertos[0].Prestamo.EquipoID != equipoConReserva {
		t.Error("primero debería ir lo que tiene hora de devolución, no lo que no la tiene")
	}
	if abiertos[0].MateriaNombre == nil || *abiertos[0].MateriaNombre != "Matemáticas" {
		t.Errorf("un préstamo contra reserva debería traer la materia: %v", abiertos[0].MateriaNombre)
	}
	if abiertos[0].Identificador != 1 || abiertos[0].CarroNombre == "" {
		t.Errorf("falta la ubicación: PC %d del carro %q", abiertos[0].Identificador, abiertos[0].CarroNombre)
	}
	// Un préstamo espontáneo no tiene materia, y eso no es un dato faltante.
	if abiertos[1].MateriaNombre != nil {
		t.Errorf("un préstamo espontáneo no debería traer materia: %v", *abiertos[1].MateriaNombre)
	}
}

// TestPostgresRepo_Prestamo_SoloListaLosAbiertos: lo devuelto sale del
// listado del mostrador, pero sigue en el historial de la PC.
func TestPostgresRepo_Prestamo_SoloListaLosAbiertos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	p := crearPrestamoDeTest(t, repo, entregaDeTest(equipoID), ahora)
	if err := p.Devolver("", "", ahora.Add(time.Hour)); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.GuardarPrestamo(ctx, p); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	abiertos, err := repo.ListarPrestamosAbiertos(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(abiertos) != 0 {
		t.Errorf("una máquina devuelta no debería seguir figurando afuera: %d", len(abiertos))
	}

	historial, err := repo.ListarPrestamosDeEquipo(ctx, equipoID, 10)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(historial) != 1 {
		t.Errorf("el historial de la PC debería conservarla: %d", len(historial))
	}
}

// TestPostgresRepo_Prestamo_SobreviveALaReserva: al archivar un ciclo se
// borran físicamente sus reservas (RF-02.4). El registro de que alguien se
// llevó una máquina vale por sí mismo, así que el préstamo tiene que quedar
// con reserva_id en NULL en vez de irse con ella.
func TestPostgresRepo_Prestamo_SobreviveALaReserva(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	grupo := nuevoReservaGrupoDeTest(materiaID, ahora, 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, grupo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	reserva, err := domain.NuevaReservaNormal(NuevoID(), grupo.ID, equipoID, materiaID,
		"Ada Lovelace", nil, ahora, 8*time.Hour, 9*time.Hour, ahora.Add(-time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, reserva); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	d := entregaDeTest(equipoID)
	d.ReservaID = &reserva.ID
	p := crearPrestamoDeTest(t, repo, d, ahora)

	if _, err := pool.Exec(ctx, `DELETE FROM reserva WHERE id = $1`, reserva.ID); err != nil {
		t.Fatalf("borrar la reserva no debería fallar por la FK del préstamo: %v", err)
	}

	encontrado, err := repo.BuscarPrestamoPorID(ctx, p.ID)
	if err != nil {
		t.Fatalf("el préstamo debería seguir existiendo: %v", err)
	}
	if encontrado.ReservaID != nil {
		t.Errorf("reservaID debería haber quedado en NULL, quedó %q", *encontrado.ReservaID)
	}
	if encontrado.EntregadoANombre != "Ana Pérez" {
		t.Error("el registro tiene que seguir diciendo quién se llevó la máquina")
	}
}

// TestPostgresRepo_Prestamo_DentroDeUnaTransaccion: entregar varios equipos de
// una reserva es una sola operación, así que el repo tiene que funcionar
// atado a una transacción igual que el resto del paquete.
func TestPostgresRepo_Prestamo_DentroDeUnaTransaccion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	pc1 := crearEquipoDeCarroDeTest(t, pool)
	pc2 := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	// La segunda PC ya está afuera: el lote entero tiene que volver atrás.
	crearPrestamoDeTest(t, repo, entregaDeTest(pc2), ahora)

	err := repo.EnTransaccion(ctx, func(tx application.Repo) error {
		for _, equipoID := range []string{pc1, pc2} {
			p, err := domain.NuevoPrestamo(NuevoID(), entregaDeTest(equipoID), ahora)
			if err != nil {
				return err
			}
			if err := tx.CrearPrestamo(ctx, p); err != nil {
				return err
			}
		}
		return nil
	})

	if err != application.ErrEquipoYaPrestado {
		t.Fatalf("esperaba ErrEquipoYaPrestado, obtuve %v", err)
	}
	if _, err := repo.BuscarPrestamoAbiertoDeEquipo(ctx, pc1); err != application.ErrPrestamoNoEncontrado {
		t.Error("la primera entrega del lote debería haberse deshecho")
	}
}

// TestPostgresRepo_ReservasFuturasDeEquipo_VienenOrdenadas fija el contrato del
// que depende el aviso de "esta PC tiene una reserva encima" al entregarla
// suelta: quien llama toma la PRIMERA como la más próxima.
//
// La consulta no tenía ORDER BY, así que Postgres podía devolverlas en
// cualquier orden y el aviso nombraba la reserva de la semana siguiente en
// vez de la de dentro de una hora.
func TestPostgresRepo_ReservasFuturasDeEquipo_VienenOrdenadas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	materiaID := crearMateriaDeTest(t, pool)
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	creada := time.Now().UTC().Truncate(time.Microsecond)
	desde := diaDe(2026, time.March, 1)

	// Se insertan del futuro hacia el presente para que el orden de la tabla
	// sea el inverso al esperado.
	for _, dia := range []int{20, 10, 3} {
		fecha := diaDe(2026, time.March, dia)
		grupo := nuevoReservaGrupoDeTest(materiaID, fecha, 8*time.Hour, 9*time.Hour)
		if err := repo.CrearReservaGrupo(ctx, grupo); err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
		reserva, err := domain.NuevaReservaNormal(NuevoID(), grupo.ID, equipoID, materiaID,
			"Ada Lovelace", nil, fecha, 8*time.Hour, 9*time.Hour, creada)
		if err != nil {
			t.Fatalf("error de dominio inesperado: %v", err)
		}
		if err := repo.CrearReserva(ctx, reserva); err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
	}

	futuras, err := repo.ListarReservasFuturasDeEquipo(ctx, equipoID, desde)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(futuras) != 3 {
		t.Fatalf("esperaba 3 reservas futuras, obtuve %d", len(futuras))
	}
	if !futuras[0].Fecha.Equal(diaDe(2026, time.March, 3)) {
		t.Errorf("la primera es del %v; tiene que ser la más próxima (3 de marzo)", futuras[0].Fecha)
	}
}

// TestPostgresRepo_Prestamo_QuienRetiraSobreviveALaVueltaDeLaBase
//
// Quien responde y quien vino a buscar el equipo son dos columnas distintas,
// y las dos tienen que volver de la base como se guardaron. Es el dato que
// permite reclamarle al docente aunque las máquinas hayan salido en manos de
// un alumno.
func TestPostgresRepo_Prestamo_QuienRetiraSobreviveALaVueltaDeLaBase(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	datos := entregaDeTest(equipoID)
	datos.RetiradoPor = "Juan (alumno de 5°A)"
	p := crearPrestamoDeTest(t, repo, datos, ahora)

	vuelto, err := repo.BuscarPrestamoPorID(ctx, p.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if vuelto.EntregadoANombre != "Ana Pérez" {
		t.Errorf("responsable = %q, esperaba a quien responde", vuelto.EntregadoANombre)
	}
	if vuelto.RetiradoPor != "Juan (alumno de 5°A)" {
		t.Errorf("retiradoPor = %q, esperaba a quien vino a buscarlo", vuelto.RetiradoPor)
	}

	// Y el listado del mostrador tiene que traerlo también: es donde se lee.
	abiertos, err := repo.ListarPrestamosAbiertos(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(abiertos) != 1 || abiertos[0].Prestamo.RetiradoPor != "Juan (alumno de 5°A)" {
		t.Errorf("el listado de afuera debería traer quién retiró: %+v", abiertos)
	}
}

// No anotar a quién retira es el caso normal, no un dato que falte: vacío
// significa que lo retiró la misma persona que responde por él.
func TestPostgresRepo_Prestamo_SinAnotarQuienRetira(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	equipoID := crearEquipoDeCarroDeTest(t, pool)
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	p := crearPrestamoDeTest(t, repo, entregaDeTest(equipoID), ahora)

	vuelto, err := repo.BuscarPrestamoPorID(ctx, p.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if vuelto.RetiradoPor != "" {
		t.Errorf("retiradoPor = %q, esperaba vacío", vuelto.RetiradoPor)
	}
}

// PrestamosAVigilar es la consulta que alimenta el barrido de fondo: el
// reclamo por devolución demorada y el aviso de cierre de jornada.
//
// Tiene test propio, y contra Postgres real, por cómo se rompió: comparte
// `columnasPrestamoDetallado` con las consultas del mostrador, alguien le
// agregó `retirado_por` a esa constante, actualizó el Scan de un consumidor y
// no el del otro. pgx cortó con "number of field descriptions must equal
// number of destinations, got 20 and 19" y el barrido murió en cada pasada
// —cada cinco minutos, durante meses— sin que nadie recibiera un aviso de una
// computadora que no volvió.
//
// Ningún test con un repo falso podía verlo: el error lo produce Postgres al
// contar las columnas. Por eso este mira los campos que vienen DE la consulta,
// y no solo que no falle: si mañana se agrega otra columna a la constante y se
// olvida acá, el Scan vuelve a desalinearse y esto lo dice.
func TestPostgresRepo_PrestamosAVigilar_TraeTodasLasColumnas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	materiaID := crearMateriaDeTest(t, pool)
	equipo := crearEquipoDeCarroDeTest(t, pool)

	grupo := nuevoReservaGrupoDeTest(materiaID, ahora, 8*time.Hour, 9*time.Hour)
	if err := repo.CrearReservaGrupo(ctx, grupo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	reserva, err := domain.NuevaReservaNormal(NuevoID(), grupo.ID, equipo, materiaID,
		"Ada Lovelace", nil, ahora, 8*time.Hour, 9*time.Hour, ahora.Add(-time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, reserva); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	vence := ahora.Add(time.Hour)
	d := entregaDeTest(equipo)
	d.ReservaID = &reserva.ID
	d.DevolucionEstimada = &vence
	// El campo que faltaba en el Scan. Va con valor para que una fila que lo
	// trae vacío no oculte el desalineamiento.
	d.RetiradoPor = "Ayudante de laboratorio"
	creado := crearPrestamoDeTest(t, repo, d, ahora)

	aVigilar, err := repo.PrestamosAVigilar(ctx)
	if err != nil {
		t.Fatalf("el barrido no puede fallar leyendo lo que está afuera: %v", err)
	}
	if len(aVigilar) != 1 {
		t.Fatalf("esperaba 1 préstamo abierto, obtuve %d", len(aVigilar))
	}

	p := aVigilar[0]
	if p.Prestamo.ID != creado.ID {
		t.Errorf("esperaba el préstamo %s, obtuve %s", creado.ID, p.Prestamo.ID)
	}
	// Cada uno de estos sale de una columna distinta: si el Scan se corre un
	// lugar, alguno queda con el valor del vecino.
	if p.Prestamo.RetiradoPor != "Ayudante de laboratorio" {
		t.Errorf("retirado_por llegó como %q", p.Prestamo.RetiradoPor)
	}
	if p.Prestamo.EntregadoANombre != "Ana Pérez" {
		t.Errorf("entregado_a_nombre llegó como %q", p.Prestamo.EntregadoANombre)
	}
	if p.Prestamo.DevolucionEstimada == nil || !p.Prestamo.DevolucionEstimada.Equal(vence) {
		t.Errorf("devolucion_estimada llegó como %v", p.Prestamo.DevolucionEstimada)
	}
	if p.Prestamo.DevueltoEn != nil {
		t.Errorf("un préstamo abierto no puede traer devuelto_en: %v", p.Prestamo.DevueltoEn)
	}
	if p.Identificador != 1 || p.Etiqueta == "" || p.CarroNombre == "" {
		t.Errorf("falta la ubicación: identificador %d, etiqueta %q, carro %q",
			p.Identificador, p.Etiqueta, p.CarroNombre)
	}
}

// La contracara: lo devuelto no se vigila. Sin esto, "traer todo" podría
// estar reclamándole a alguien una máquina que ya entregó.
func TestPostgresRepo_PrestamosAVigilar_IgnoraLoDevuelto(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	equipo := crearEquipoDeCarroDeTest(t, pool)
	adminID := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	p := crearPrestamoDeTest(t, repo, entregaDeTest(equipo), ahora)
	if err := p.Devolver(adminID, "", ahora.Add(time.Hour)); err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.GuardarPrestamo(ctx, p); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	aVigilar, err := repo.PrestamosAVigilar(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(aVigilar) != 0 {
		t.Fatalf("lo devuelto no se vigila, obtuve %d", len(aVigilar))
	}
}

// ReservasAVigilar es la PRIMERA de las dos consultas del barrido, así que
// cuando falla no corre nada: ni los recordatorios, ni el aviso de no retiro,
// ni la liberación, ni los reclamos de devolución.
//
// Falló durante meses de una forma que se esconde sola: hora_inicio y hora_fin
// son columnas TIME, pgx las entrega como time.Time, y el Scan las recibía
// sobre los time.Duration del struct. Con la tabla vacía —o sin reservas de
// hoy ni de mañana— el Scan no se ejecuta y la consulta parece sana; alcanza
// UNA reserva del día para que el barrido entero muera.
//
// De ahí que este test cree la reserva antes de mirar, y que compare las horas
// además de contar filas: es el par de campos que estaba mal convertido.
func TestPostgresRepo_ReservasAVigilar_ConvierteLasHoras(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	hoy := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, time.UTC)

	materiaID := crearMateriaDeTest(t, pool)
	equipo := crearEquipoDeCarroDeTest(t, pool)

	const inicio = 8*time.Hour + 30*time.Minute
	const fin = 10 * time.Hour

	grupo := nuevoReservaGrupoDeTest(materiaID, hoy, inicio, fin)
	if err := repo.CrearReservaGrupo(ctx, grupo); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	reserva, err := domain.NuevaReservaNormal(NuevoID(), grupo.ID, equipo, materiaID,
		"Ada Lovelace", nil, hoy, inicio, fin, ahora.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearReserva(ctx, reserva); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	aVigilar, err := repo.ReservasAVigilar(ctx, hoy)
	if err != nil {
		t.Fatalf("el barrido no puede fallar leyendo las reservas del día: %v", err)
	}
	if len(aVigilar) != 1 {
		t.Fatalf("esperaba la reserva de hoy, obtuve %d", len(aVigilar))
	}

	v := aVigilar[0]
	if v.HoraInicio != inicio {
		t.Errorf("hora_inicio llegó como %v, esperaba %v", v.HoraInicio, inicio)
	}
	if v.HoraFin != fin {
		t.Errorf("hora_fin llegó como %v, esperaba %v", v.HoraFin, fin)
	}
	if v.ReservaID != reserva.ID || v.EquipoID != equipo {
		t.Errorf("la fila no corresponde a la reserva creada: %+v", v)
	}
	// Sin docente ni etiqueta el aviso no se puede redactar.
	if v.DocenteNombre == "" || v.Etiqueta == "" {
		t.Errorf("falta con qué armar el aviso: docente %q, equipo %q", v.DocenteNombre, v.Etiqueta)
	}
}

// La otra mitad del modo de falla: sin reservas del día la consulta no
// escanea nada y no puede decir si la conversión está bien. Este test fija
// que ese caso siga siendo silencioso y vacío, no un error.
func TestPostgresRepo_ReservasAVigilar_SinReservasDelDia(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	hoy := time.Date(2027, 5, 3, 0, 0, 0, 0, time.UTC)

	aVigilar, err := repo.ReservasAVigilar(ctx, hoy)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(aVigilar) != 0 {
		t.Fatalf("esperaba ninguna, obtuve %d", len(aVigilar))
	}
}
