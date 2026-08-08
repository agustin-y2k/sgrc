//go:build integration

package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

func diaDe(anio int, mes time.Month, dia int) time.Time {
	return time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)
}

func crearPCDeTest(t *testing.T, repo *PostgresRepo, carroID string, identificador int, serie string) *domain.PC {
	t.Helper()
	pc, err := domain.NuevaPC(NuevoID(), carroID, identificador, serie, false, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearPC(context.Background(), pc); err != nil {
		t.Fatalf("no se pudo crear la PC de prueba: %v", err)
	}
	return pc
}

func crearLicenciaDeTest(t *testing.T, repo *PostgresRepo, pcID, nombre string, diasDuracion, diasAviso int) *domain.LicenciaSoftware {
	t.Helper()
	l, err := domain.NuevaLicencia(NuevoID(), pcID, nombre, diasDuracion, diasAviso,
		time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearLicencia(context.Background(), l); err != nil {
		t.Fatalf("no se pudo crear la licencia de prueba: %v", err)
	}
	return l
}

func TestPostgresRepo_Licencia_CrearSinFechaYBuscar(t *testing.T) {
	// El caso del arranque en frío: la licencia se carga antes de saber
	// cuándo vence. La columna tiene que aceptar NULL y devolverlo como tal.
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 3, "SERIE-LIC-1")
	l := crearLicenciaDeTest(t, repo, pc.ID, "AutoCAD 2027", 30, 1)

	encontrada, err := repo.BuscarLicenciaPorID(ctx, l.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if encontrada.FechaVencimiento != nil {
		t.Errorf("esperaba vencimiento nulo, obtuve %v", *encontrada.FechaVencimiento)
	}
	if encontrada.Nombre != "AutoCAD 2027" || encontrada.DiasDuracion != 30 || encontrada.DiasAviso != 1 {
		t.Errorf("licencia encontrada no coincide: %+v", encontrada)
	}
}

func TestPostgresRepo_Licencia_GuardarYReleerLaFecha(t *testing.T) {
	// Confirma contra Postgres real que una fecha guardada vuelve idéntica.
	// Es lo que hace confiable al contador: si el DATE volviera corrido por
	// zona horaria, "faltan 30 días" pasaría a ser 29 o 31.
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 3, "SERIE-LIC-1")
	l := crearLicenciaDeTest(t, repo, pc.ID, "AutoCAD 2027", 30, 1)

	ahora := time.Now().UTC().Truncate(time.Microsecond)
	l.RenovadaEl(diaDe(2026, time.August, 4), "", ahora)
	if err := repo.GuardarLicencia(ctx, l); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	encontrada, err := repo.BuscarLicenciaPorID(ctx, l.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !encontrada.FechaVencimiento.Equal(diaDe(2026, time.September, 3)) {
		t.Errorf("vencimiento = %v, esperaba 2026-09-03", *encontrada.FechaVencimiento)
	}
	if !encontrada.UltimaRenovacion.Equal(diaDe(2026, time.August, 4)) {
		t.Errorf("ultimaRenovacion = %v, esperaba 2026-08-04", *encontrada.UltimaRenovacion)
	}
	if dias, _ := encontrada.DiasRestantes(diaDe(2026, time.August, 7)); dias != 27 {
		t.Errorf("DiasRestantes tras la ida y vuelta = %d, esperaba 27", dias)
	}
}

func TestPostgresRepo_Licencia_MismoSoftwareDosVecesEnLaMismaPC_Error(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 3, "SERIE-LIC-1")
	crearLicenciaDeTest(t, repo, pc.ID, "AutoCAD 2027", 30, 1)

	// Distinta capitalización: para el índice funcional es la misma.
	otra, err := domain.NuevaLicencia(NuevoID(), pc.ID, "autocad 2027", 30, 1, time.Now())
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	if err := repo.CrearLicencia(ctx, otra); err != application.ErrLicenciaDuplicada {
		t.Fatalf("esperaba ErrLicenciaDuplicada, obtuve %v", err)
	}
}

func TestPostgresRepo_Licencia_MismoSoftwareEnOtraPC_OK(t *testing.T) {
	// La regla de negocio del modelo elegido: una fila por (PC, software).
	// El mismo AutoCAD en las ocho PCs del carro son ocho filas.
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc1 := crearPCDeTest(t, repo, carro.ID, 1, "SERIE-LIC-1")
	pc2 := crearPCDeTest(t, repo, carro.ID, 2, "SERIE-LIC-2")

	crearLicenciaDeTest(t, repo, pc1.ID, "AutoCAD 2027", 30, 1)
	crearLicenciaDeTest(t, repo, pc2.ID, "AutoCAD 2027", 30, 1)
}

func TestPostgresRepo_Licencia_BorrarYListarPorPC(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 3, "SERIE-LIC-1")
	l1 := crearLicenciaDeTest(t, repo, pc.ID, "AutoCAD 2027", 30, 1)
	crearLicenciaDeTest(t, repo, pc.ID, "SolidWorks", 365, 7)

	licencias, err := repo.ListarLicenciasPorPC(ctx, pc.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(licencias) != 2 {
		t.Fatalf("esperaba 2 licencias, obtuve %d", len(licencias))
	}

	if err := repo.BorrarLicencia(ctx, l1.ID); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	licencias, err = repo.ListarLicenciasPorPC(ctx, pc.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(licencias) != 1 || licencias[0].Nombre != "SolidWorks" {
		t.Errorf("tras borrar esperaba solo SolidWorks, obtuve %+v", licencias)
	}

	if err := repo.BorrarLicencia(ctx, l1.ID); err != application.ErrLicenciaNoEncontrada {
		t.Errorf("borrar dos veces debería dar ErrLicenciaNoEncontrada, obtuve %v", err)
	}
}

func TestPostgresRepo_Licencia_ListarTraeLaUbicacionYOrdena(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 7, "SERIE-LIC-1")
	ahora := time.Now().UTC().Truncate(time.Microsecond)

	sinFecha := crearLicenciaDeTest(t, repo, pc.ID, "Sin fecha", 30, 1)

	vencida := crearLicenciaDeTest(t, repo, pc.ID, "Vencida", 30, 1)
	vencida.FijarVencimiento(diaDe(2026, time.August, 1), "", ahora)
	if err := repo.GuardarLicencia(ctx, vencida); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	lejana := crearLicenciaDeTest(t, repo, pc.ID, "Lejana", 30, 1)
	lejana.FijarVencimiento(diaDe(2026, time.December, 1), "", ahora)
	if err := repo.GuardarLicencia(ctx, lejana); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	licencias, err := repo.ListarLicencias(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(licencias) != 3 {
		t.Fatalf("esperaba 3 licencias, obtuve %d", len(licencias))
	}

	// Sin fecha primero (hay que ir a mirar la máquina), después de la más
	// vencida a la que más le falta.
	ordenEsperado := []string{sinFecha.ID, vencida.ID, lejana.ID}
	for i, esperado := range ordenEsperado {
		if licencias[i].Licencia.ID != esperado {
			t.Errorf("posición %d: %q, esperaba %q", i, licencias[i].Licencia.Nombre, esperado)
		}
	}

	if licencias[0].PCIdentificador != 7 || licencias[0].CarroNombre != "Carro 1" {
		t.Errorf("ubicación incompleta: PC %d del carro %q",
			licencias[0].PCIdentificador, licencias[0].CarroNombre)
	}
}

// TestPostgresRepo_Licencia_CandidatasAAviso cubre el filtro grueso del job
// contra Postgres real: la aritmética de DATE con dias_aviso y el
// IS DISTINCT FROM de las marcas. Es la consulta que decide si un mail sale
// o no sale, y la que no se puede verificar con un fake.
func TestPostgresRepo_Licencia_CandidatasAAviso(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	hoy := diaDe(2026, time.August, 7)

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 1, "SERIE-LIC-1")

	conVencimiento := func(nombre string, vencimiento time.Time, diasAviso int) *domain.LicenciaSoftware {
		l := crearLicenciaDeTest(t, repo, pc.ID, nombre, 30, diasAviso)
		l.FijarVencimiento(vencimiento, "", ahora)
		if err := repo.GuardarLicencia(ctx, l); err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
		return l
	}

	// Entran.
	venceManana := conVencimiento("Vence mañana", diaDe(2026, time.August, 8), 1)
	venceHoy := conVencimiento("Vence hoy", hoy, 1)
	vencida := conVencimiento("Vencida hace rato", diaDe(2026, time.July, 1), 1)
	conAvisoLargo := conVencimiento("Aviso de 7 días", diaDe(2026, time.August, 12), 7)

	// No entran.
	conVencimiento("Falta mucho", diaDe(2026, time.December, 1), 1)
	crearLicenciaDeTest(t, repo, pc.ID, "Sin fecha", 30, 1)

	candidatas, err := repo.ListarCandidatasAAviso(ctx, hoy)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	esperados := map[string]bool{
		venceManana.ID: true, venceHoy.ID: true, vencida.ID: true, conAvisoLargo.ID: true,
	}
	if len(candidatas) != len(esperados) {
		var nombres []string
		for _, c := range candidatas {
			nombres = append(nombres, c.Licencia.Nombre)
		}
		t.Fatalf("esperaba %d candidatas, obtuve %d: %v", len(esperados), len(candidatas), nombres)
	}
	for _, c := range candidatas {
		if !esperados[c.Licencia.ID] {
			t.Errorf("no esperaba a %q entre las candidatas", c.Licencia.Nombre)
		}
	}
}

// TestPostgresRepo_Licencia_MarcarAvisosSacaDeLasCandidatas es la mitad de
// la idempotencia que vive en la base: una vez marcada, la fila no vuelve a
// aparecer aunque el job corra cada hora.
func TestPostgresRepo_Licencia_MarcarAvisosSacaDeLasCandidatas(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	hoy := diaDe(2026, time.August, 7)

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 1, "SERIE-LIC-1")
	l := crearLicenciaDeTest(t, repo, pc.ID, "AutoCAD 2027", 30, 1)
	l.FijarVencimiento(diaDe(2026, time.August, 8), "", ahora)
	if err := repo.GuardarLicencia(ctx, l); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	candidatas, err := repo.ListarCandidatasAAviso(ctx, hoy)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(candidatas) != 1 {
		t.Fatalf("esperaba 1 candidata, obtuve %d", len(candidatas))
	}

	candidatas[0].Licencia.MarcarAvisoPrevioEnviado()
	if err := repo.MarcarAvisosEnviados(ctx, candidatas[0].Licencia); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Sigue siendo candidata, y está bien: mañana le toca el aviso del día
	// del vencimiento. Lo que no puede es volver a disparar el aviso previo
	// hoy — y eso lo decide el dominio, que es donde vive la regla fina.
	candidatas, err = repo.ListarCandidatasAAviso(ctx, hoy)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(candidatas) != 1 {
		t.Fatalf("esperaba que siguiera siendo candidata para el aviso de mañana, hay %d", len(candidatas))
	}
	if candidatas[0].Licencia.CorrespondeAvisoPrevio(hoy) {
		t.Error("el aviso previo volvió a corresponder el mismo día: el job mandaría dos mails")
	}

	// Al día siguiente sí le toca el otro aviso; una vez cerrado ese, la
	// fila deja de aparecer del todo.
	elDiaQueVence := diaDe(2026, time.August, 8)
	candidatas, err = repo.ListarCandidatasAAviso(ctx, elDiaQueVence)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(candidatas) != 1 || !candidatas[0].Licencia.CorrespondeAvisoDeVencimiento(elDiaQueVence) {
		t.Fatalf("el día del vencimiento debería corresponder el aviso de vencimiento")
	}
	candidatas[0].Licencia.MarcarAvisoDeVencimientoEnviado()
	if err := repo.MarcarAvisosEnviados(ctx, candidatas[0].Licencia); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	candidatas, err = repo.ListarCandidatasAAviso(ctx, elDiaQueVence)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(candidatas) != 0 {
		t.Errorf("con los dos avisos cerrados la fila no debería volver a leerse, quedaron %d", len(candidatas))
	}

	// Y al renovar, la marca vieja deja de coincidir con el vencimiento
	// nuevo y el ciclo se reabre solo, sin resetear nada a mano.
	renovada, err := repo.BuscarLicenciaPorID(ctx, l.ID)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := renovada.Renovar(hoy, "", ahora); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.GuardarLicencia(ctx, renovada); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// El día previo al vencimiento nuevo vuelve a ser candidata.
	candidatas, err = repo.ListarCandidatasAAviso(ctx, diaDe(2026, time.September, 5))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(candidatas) != 1 {
		t.Errorf("tras renovar, el ciclo nuevo debería volver a avisar: %d candidatas", len(candidatas))
	}
}

// TestPostgresRepo_Licencia_PCDadaDeBajaNoAvisa: renovarle la licencia a una
// máquina que ya no está en el inventario no le sirve a nadie.
func TestPostgresRepo_Licencia_PCDadaDeBajaNoAvisa(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	hoy := diaDe(2026, time.August, 7)

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 1, "SERIE-LIC-1")
	l := crearLicenciaDeTest(t, repo, pc.ID, "AutoCAD 2027", 30, 1)
	l.FijarVencimiento(diaDe(2026, time.August, 8), "", ahora)
	if err := repo.GuardarLicencia(ctx, l); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if err := pc.DarDeBaja(ahora); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := repo.GuardarPC(ctx, pc); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	candidatas, err := repo.ListarCandidatasAAviso(ctx, hoy)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(candidatas) != 0 {
		t.Errorf("una PC dada de baja no debería generar avisos, generó %d", len(candidatas))
	}
}

// TestPostgresRepo_Licencia_CandidatasNoDependenDeLaZonaDeLaSesion fija el
// borde exacto del aviso contra un cambio futuro de la consulta.
//
// Hoy es correcta y no por casualidad: Postgres infiere el tipo de `$1` como
// `date` a partir del operando izquierdo, así que pgx manda solo año/mes/día
// y la comparación es date contra date. Pero alcanza con que alguien toque
// esa expresión —comparar contra `now()`, castear a timestamp, agregar una
// hora— para que el parámetro pase a inferirse como timestamptz y Postgres
// tenga que convertir el DATE usando la zona de la SESIÓN. Ahí el borde se
// corre un día: con -03:00, el 2026-08-07 pasa a ser 2026-08-07 03:00 UTC y
// deja de ser <= las 00:00 de ese mismo día. La licencia que vence mañana se
// cae del resultado y el aviso NO SALE, sin ningún error.
//
// Es el peor modo de falla posible para una funcionalidad cuyo único trabajo
// es avisar, y no se vería en desarrollo: la base corre en UTC. Por eso el
// test fuerza la zona en la sesión en vez de confiar en la del servidor.
func TestPostgresRepo_Licencia_CandidatasNoDependenDeLaZonaDeLaSesion(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	hoy := diaDe(2026, time.August, 7)

	carro := crearCarroDeTest(t, repo, "Carro 1")
	pc := crearPCDeTest(t, repo, carro.ID, 1, "SERIE-LIC-TZ")
	l := crearLicenciaDeTest(t, repo, pc.ID, "AutoCAD 2027", 30, 1)
	// El borde exacto: vence mañana, avisa con un día de anticipación, así
	// que hoy es el primer día en que corresponde el aviso.
	l.FijarVencimiento(diaDe(2026, time.August, 8), "", ahora)
	if err := repo.GuardarLicencia(ctx, l); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	for _, zona := range []string{"UTC", "America/Argentina/Buenos_Aires", "Asia/Tokyo"} {
		t.Run(zona, func(t *testing.T) {
			if _, err := pool.Exec(ctx, "SET TIME ZONE '"+zona+"'"); err != nil {
				t.Fatalf("no se pudo cambiar la zona de la sesión: %v", err)
			}
			t.Cleanup(func() {
				if _, err := pool.Exec(ctx, "SET TIME ZONE 'UTC'"); err != nil {
					t.Logf("advertencia: no se pudo restaurar la zona: %v", err)
				}
			})

			candidatas, err := repo.ListarCandidatasAAviso(ctx, hoy)
			if err != nil {
				t.Fatalf("no debería fallar: %v", err)
			}
			if len(candidatas) != 1 {
				t.Fatalf("con la sesión en %s se perdió el aviso del borde: %d candidatas", zona, len(candidatas))
			}
			if !candidatas[0].Licencia.CorrespondeAvisoPrevio(hoy) {
				t.Errorf("con la sesión en %s, la licencia que vence mañana no dispara el aviso previo", zona)
			}
		})
	}
}

// Las dos consultas de licencias hacían INNER JOIN a carro. Desde la 015 eso
// es un agujero silencioso: una notebook suelta puede tener AutoCAD igual que
// las del carro, y su licencia no aparecía en la pantalla NI era candidata a
// aviso — se vencía sin que nadie se enterara, que es exactamente lo que esta
// funcionalidad existe para evitar.
func TestPostgresRepo_Licencia_DeUnEquipoSinCarro(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	repo := NewPostgresRepo(pool)
	ctx := context.Background()

	equipo := crearEquipoDeTest(t, repo, "NOTEBOOK", "Notebook chica", false)
	l := crearLicenciaDeTest(t, repo, equipo.ID, "AutoCAD 2027", 30, 1)

	// Vence mañana: entra en la ventana de aviso. Sin usuario, igual que los
	// demás tests de este archivo — la columna referencia a usuario y acá no
	// hace falta ninguno.
	ahora := time.Now().UTC().Truncate(time.Microsecond)
	hoy := diaDe(2026, time.August, 7)
	l.FijarVencimiento(diaDe(2026, time.August, 8), "", ahora)
	if err := repo.GuardarLicencia(ctx, l); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	listadas, err := repo.ListarLicencias(ctx)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(listadas) != 1 {
		t.Fatalf("la licencia de un equipo suelto tiene que listarse; obtuve %d", len(listadas))
	}
	if listadas[0].Etiqueta != "Notebook chica" {
		t.Errorf("esperaba la etiqueta del equipo, obtuve %q", listadas[0].Etiqueta)
	}
	if listadas[0].PCIdentificador != 0 || listadas[0].CarroNombre != "" {
		t.Errorf("esperaba identificador 0 y carro vacío, obtuve %d y %q",
			listadas[0].PCIdentificador, listadas[0].CarroNombre)
	}

	candidatas, err := repo.ListarCandidatasAAviso(ctx, hoy)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(candidatas) != 1 {
		t.Fatalf("tenía que ser candidata a aviso; obtuve %d", len(candidatas))
	}
	if candidatas[0].Etiqueta != "Notebook chica" {
		t.Errorf("el aviso mandaría a buscar %q", candidatas[0].Etiqueta)
	}
}
