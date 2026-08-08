package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramiro/sgrc/internal/inventory/domain"
)

// hoyDeTest es el día que devuelve el reloj de nuevoServicioDeTest.
var hoyDeTest = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func dia(anio int, mes time.Month, d int) time.Time {
	return time.Date(anio, mes, d, 0, 0, 0, 0, time.UTC)
}

// repoConCarroYPCs deja el inventario mínimo para poder cargar licencias.
func repoConCarroYPCs(cantidad int) *fakeRepo {
	repo := nuevoFakeRepo()
	repo.carros["carro-1"] = &domain.Carro{ID: "carro-1", Nombre: "Carro 1"}
	for i := 1; i <= cantidad; i++ {
		id := "equipo-" + string(rune('0'+i))
		repo.equipos[id] = &domain.Equipo{ID: id, CarroID: "carro-1", Identificador: i, Estado: domain.EstadoDisponible}
	}
	return repo
}

// ── Alta masiva ─────────────────────────────────────────────────────────

func TestCrearLicencias_UnaPorCadaEquipo(t *testing.T) {
	repo := repoConCarroYPCs(3)
	svc := servicioSimple(repo)

	resultado, err := svc.CrearLicencias(context.Background(), NuevaLicenciaParams{
		EquipoIDs:    []string{"equipo-1", "equipo-2", "equipo-3"},
		Nombre:       "AutoCAD 2027",
		DiasDuracion: 30,
		DiasAviso:    1,
		PorUsuario:   "admin-1",
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Creadas) != 3 {
		t.Fatalf("esperaba 3 licencias creadas, obtuve %d", len(resultado.Creadas))
	}
	// Sin vencimiento declarado, nacen sin fecha: es el arranque en frío,
	// cuando todavía no se miró la máquina.
	for _, l := range resultado.Creadas {
		if l.FechaVencimiento != nil {
			t.Errorf("licencia de %s: esperaba sin fecha, tiene %v", l.EquipoID, *l.FechaVencimiento)
		}
	}
}

func TestCrearLicencias_LasQueYaLaTenianSeSalteanYSeInforman(t *testing.T) {
	// El caso real: se agregaron dos PCs al carro y el Admin marca las
	// cuatro para no tener que acordarse de cuáles faltaban.
	repo := repoConCarroYPCs(4)
	svc := servicioSimple(repo)
	ctx := context.Background()

	if _, err := svc.CrearLicencias(ctx, NuevaLicenciaParams{
		EquipoIDs: []string{"equipo-1", "equipo-2"}, Nombre: "AutoCAD 2027", DiasDuracion: 30, DiasAviso: 1,
	}); err != nil {
		t.Fatalf("la primera tanda no debería fallar: %v", err)
	}

	resultado, err := svc.CrearLicencias(ctx, NuevaLicenciaParams{
		EquipoIDs: []string{"equipo-1", "equipo-2", "equipo-3", "equipo-4"}, Nombre: "AutoCAD 2027", DiasDuracion: 30, DiasAviso: 1,
	})

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Creadas) != 2 {
		t.Errorf("esperaba 2 creadas (equipo-3 y equipo-4), obtuve %d", len(resultado.Creadas))
	}
	if len(resultado.EquiposQueYaLaTenian) != 2 {
		t.Errorf("esperaba 2 salteadas, obtuve %v", resultado.EquiposQueYaLaTenian)
	}
}

// TestCrearLicencias_ElLoteEsReintentable es la razón por la que un
// duplicado no aborta: si algo se rompe en el medio, volver a mandar el
// mismo request termina el trabajo sin duplicar nada.
func TestCrearLicencias_ElLoteEsReintentable(t *testing.T) {
	repo := repoConCarroYPCs(3)
	svc := servicioSimple(repo)
	ctx := context.Background()
	fallaDeRed := errors.New("se cayó la conexión")
	repo.errAlCrearLicenciaEnPC["equipo-3"] = fallaDeRed

	params := NuevaLicenciaParams{
		EquipoIDs: []string{"equipo-1", "equipo-2", "equipo-3"}, Nombre: "AutoCAD 2027", DiasDuracion: 30, DiasAviso: 1,
	}
	if _, err := svc.CrearLicencias(ctx, params); !errors.Is(err, fallaDeRed) {
		t.Fatalf("esperaba que el lote cortara con el error real, obtuve %v", err)
	}
	if len(repo.licencias) != 2 {
		t.Fatalf("esperaba que quedaran creadas las dos primeras, quedaron %d", len(repo.licencias))
	}

	// Se arregla lo que fallaba y se reintenta el MISMO request.
	delete(repo.errAlCrearLicenciaEnPC, "equipo-3")
	resultado, err := svc.CrearLicencias(ctx, params)

	if err != nil {
		t.Fatalf("el reintento no debería fallar: %v", err)
	}
	if len(resultado.Creadas) != 1 || len(resultado.EquiposQueYaLaTenian) != 2 {
		t.Errorf("el reintento debería crear solo la que faltaba: creadas=%d salteadas=%d",
			len(resultado.Creadas), len(resultado.EquiposQueYaLaTenian))
	}
	if len(repo.licencias) != 3 {
		t.Errorf("esperaba 3 licencias en total, hay %d", len(repo.licencias))
	}
}

func TestCrearLicencias_SinEquipos(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.CrearLicencias(context.Background(), NuevaLicenciaParams{
		EquipoIDs: nil, Nombre: "AutoCAD 2027", DiasDuracion: 30, DiasAviso: 1,
	})

	if !errors.Is(err, ErrSinPCs) {
		t.Errorf("esperaba ErrSinPCs, obtuve %v", err)
	}
}

func TestCrearLicencias_NombreInvalidoNoCreaNinguna(t *testing.T) {
	repo := repoConCarroYPCs(3)
	svc := servicioSimple(repo)

	_, err := svc.CrearLicencias(context.Background(), NuevaLicenciaParams{
		EquipoIDs: []string{"equipo-1", "equipo-2", "equipo-3"}, Nombre: "   ", DiasDuracion: 30, DiasAviso: 1,
	})

	if !errors.Is(err, domain.ErrNombreLicenciaVacio) {
		t.Fatalf("esperaba ErrNombreLicenciaVacio, obtuve %v", err)
	}
	if len(repo.licencias) != 0 {
		t.Errorf("una validación que falla para todas no debería crear ninguna, creó %d", len(repo.licencias))
	}
}

// ── Las tres formas de declarar el vencimiento ──────────────────────────

func TestCrearLicencias_VencimientoDeclarado(t *testing.T) {
	quedan := 12
	renovadaEl := dia(2025, time.December, 20)
	venceEl := dia(2026, time.March, 15)

	casos := []struct {
		nombre      string
		declarado   VencimientoDeclarado
		esperado    time.Time
		esperaRenov *time.Time
	}{
		{
			nombre:      "la renové el 20 de diciembre",
			declarado:   VencimientoDeclarado{RenovadaEl: &renovadaEl},
			esperado:    dia(2026, time.January, 19), // + 30 días de duración
			esperaRenov: &renovadaEl,
		},
		{
			nombre:    "quedan 12 días (lo que dice la máquina)",
			declarado: VencimientoDeclarado{QuedanDias: &quedan},
			esperado:  dia(2026, time.January, 13), // hoy + 12
		},
		{
			nombre:    "vence el 15 de marzo",
			declarado: VencimientoDeclarado{VenceEl: &venceEl},
			esperado:  venceEl,
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := repoConCarroYPCs(1)
			svc := servicioSimple(repo)

			resultado, err := svc.CrearLicencias(context.Background(), NuevaLicenciaParams{
				EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30, DiasAviso: 1,
				Vencimiento: c.declarado, PorUsuario: "admin-1",
			})
			if err != nil {
				t.Fatalf("no debería fallar: %v", err)
			}

			l := resultado.Creadas[0]
			if !l.FechaVencimiento.Equal(c.esperado) {
				t.Errorf("vencimiento = %v, esperaba %v", *l.FechaVencimiento, c.esperado)
			}
			if c.esperaRenov == nil && l.UltimaRenovacion != nil {
				t.Errorf("no debería quedar fecha de renovación, quedó %v", *l.UltimaRenovacion)
			}
			if c.esperaRenov != nil && !l.UltimaRenovacion.Equal(*c.esperaRenov) {
				t.Errorf("ultimaRenovacion = %v, esperaba %v", l.UltimaRenovacion, *c.esperaRenov)
			}
			if l.VencimientoFijadoPor == nil || *l.VencimientoFijadoPor != "admin-1" {
				t.Errorf("debería quedar registrado quién lo cargó, quedó %v", l.VencimientoFijadoPor)
			}
		})
	}
}

func TestCrearLicencias_VencimientoAmbiguo(t *testing.T) {
	// Dos formas a la vez dan fechas distintas. Elegir una sería decidir
	// por el Admin cuál de las dos cosas que dijo es la verdadera.
	quedan := 12
	venceEl := dia(2026, time.March, 15)
	repo := repoConCarroYPCs(1)
	svc := servicioSimple(repo)

	_, err := svc.CrearLicencias(context.Background(), NuevaLicenciaParams{
		EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30, DiasAviso: 1,
		Vencimiento: VencimientoDeclarado{QuedanDias: &quedan, VenceEl: &venceEl},
	})

	if !errors.Is(err, ErrVencimientoAmbiguo) {
		t.Fatalf("esperaba ErrVencimientoAmbiguo, obtuve %v", err)
	}
	if len(repo.licencias) != 0 {
		t.Errorf("no debería haber creado nada, creó %d", len(repo.licencias))
	}
}

func TestCrearLicencias_QuedanDiasNegativo(t *testing.T) {
	quedan := -5
	svc := servicioSimple(repoConCarroYPCs(1))

	_, err := svc.CrearLicencias(context.Background(), NuevaLicenciaParams{
		EquipoIDs: []string{"equipo-1"}, Nombre: "AutoCAD 2027", DiasDuracion: 30, DiasAviso: 1,
		Vencimiento: VencimientoDeclarado{QuedanDias: &quedan},
	})

	if !errors.Is(err, domain.ErrDiasRestantesInvalido) {
		t.Errorf("esperaba ErrDiasRestantesInvalido, obtuve %v", err)
	}
}

// ── Renovación masiva ───────────────────────────────────────────────────

func licenciaCargada(t *testing.T, repo *fakeRepo, id, equipoID string, vencimiento time.Time) *domain.LicenciaSoftware {
	t.Helper()
	l, err := domain.NuevaLicencia(id, equipoID, "AutoCAD 2027", 30, 1, hoyDeTest)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	l.FijarVencimiento(vencimiento, "admin-1", hoyDeTest)
	repo.licencias[id] = l
	return l
}

func TestRenovarLicencias_SinFechaDeRenovacionUsaHoy(t *testing.T) {
	repo := repoConCarroYPCs(2)
	licenciaCargada(t, repo, "lic-1", "equipo-1", dia(2026, time.January, 2))
	licenciaCargada(t, repo, "lic-2", "equipo-2", dia(2026, time.January, 2))
	svc := servicioSimple(repo)

	resultado, err := svc.RenovarLicencias(context.Background(), []string{"lic-1", "lic-2"}, nil, "admin-1")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(resultado.Renovadas) != 2 {
		t.Fatalf("esperaba 2 renovadas, obtuve %d", len(resultado.Renovadas))
	}
	for _, l := range resultado.Renovadas {
		if !l.FechaVencimiento.Equal(dia(2026, time.January, 31)) {
			t.Errorf("%s: vencimiento = %v, esperaba 2026-01-31 (hoy + 30)", l.ID, *l.FechaVencimiento)
		}
		if !l.UltimaRenovacion.Equal(hoyDeTest) {
			t.Errorf("%s: ultimaRenovacion = %v, esperaba hoy", l.ID, l.UltimaRenovacion)
		}
	}
}

func TestRenovarLicencias_ConFechaPasada(t *testing.T) {
	// "Las renové el 28 de diciembre y recién hoy lo cargo."
	repo := repoConCarroYPCs(1)
	licenciaCargada(t, repo, "lic-1", "equipo-1", dia(2026, time.January, 2))
	svc := servicioSimple(repo)
	renovadaEl := dia(2025, time.December, 28)

	resultado, err := svc.RenovarLicencias(context.Background(), []string{"lic-1"}, &renovadaEl, "admin-2")

	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	l := resultado.Renovadas[0]
	if !l.FechaVencimiento.Equal(dia(2026, time.January, 27)) {
		t.Errorf("vencimiento = %v, esperaba 2026-01-27 (28 de diciembre + 30)", *l.FechaVencimiento)
	}
	// La fecha real de la renovación, no la de la carga: es la diferencia
	// que hace que el contador no quede tres días adelantado.
	if !l.UltimaRenovacion.Equal(renovadaEl) {
		t.Errorf("ultimaRenovacion = %v, esperaba %v", l.UltimaRenovacion, renovadaEl)
	}
}

// TestRenovarLicencias_LasSinFechaSeInformanNoSeInventan es la guarda que
// impide usar "Renovar" como atajo para sacarse de encima una licencia que
// nadie verificó.
func TestRenovarLicencias_LasSinFechaSeInformanNoSeInventan(t *testing.T) {
	repo := repoConCarroYPCs(2)
	licenciaCargada(t, repo, "lic-1", "equipo-1", dia(2026, time.January, 2))
	sinFecha, err := domain.NuevaLicencia("lic-2", "equipo-2", "AutoCAD 2027", 30, 1, hoyDeTest)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.licencias["lic-2"] = sinFecha
	svc := servicioSimple(repo)

	resultado, err := svc.RenovarLicencias(context.Background(), []string{"lic-1", "lic-2"}, nil, "admin-1")

	if err != nil {
		t.Fatalf("no debería fallar: la que no se puede renovar se informa, no corta el lote: %v", err)
	}
	if len(resultado.Renovadas) != 1 || resultado.Renovadas[0].ID != "lic-1" {
		t.Errorf("esperaba solo lic-1 renovada, obtuve %+v", resultado.Renovadas)
	}
	if len(resultado.SinFechaPrevia) != 1 || resultado.SinFechaPrevia[0] != "lic-2" {
		t.Errorf("esperaba lic-2 informada como sin fecha, obtuve %v", resultado.SinFechaPrevia)
	}
	if repo.licencias["lic-2"].FechaVencimiento != nil {
		t.Error("una licencia sin verificar no puede terminar con fecha inventada")
	}
}

func TestRenovarLicencias_SinIDs(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	_, err := svc.RenovarLicencias(context.Background(), nil, nil, "admin-1")

	if !errors.Is(err, ErrSinLicencias) {
		t.Errorf("esperaba ErrSinLicencias, obtuve %v", err)
	}
}

// ── Edición ─────────────────────────────────────────────────────────────

func TestEditarLicencia_CambiarDuracionNoMueveElVencimiento(t *testing.T) {
	repo := repoConCarroYPCs(1)
	l := licenciaCargada(t, repo, "lic-1", "equipo-1", dia(2026, time.January, 20))
	vencimientoOriginal := *l.FechaVencimiento
	svc := servicioSimple(repo)

	sesenta := 60
	if err := svc.EditarLicencia(context.Background(), "lic-1", EditarLicenciaParams{
		DiasDuracion: &sesenta, PorUsuario: "admin-1",
	}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	editada := repo.licencias["lic-1"]
	if editada.DiasDuracion != 60 {
		t.Errorf("diasDuracion = %d, esperaba 60", editada.DiasDuracion)
	}
	if !editada.FechaVencimiento.Equal(vencimientoOriginal) {
		t.Errorf("el vencimiento vigente no debería moverse: %v", *editada.FechaVencimiento)
	}
}

// TestEditarLicencia_DuracionYRecalculoEnElMismoRequest cubre el botón
// "recalcular": cambiar la duración y pedir explícitamente que el
// vencimiento se rehaga desde la última renovación conocida.
func TestEditarLicencia_DuracionYRecalculoEnElMismoRequest(t *testing.T) {
	repo := repoConCarroYPCs(1)
	l := licenciaCargada(t, repo, "lic-1", "equipo-1", dia(2026, time.January, 20))
	renovadaEl := dia(2025, time.December, 21)
	l.RenovadaEl(renovadaEl, "admin-1", hoyDeTest)
	svc := servicioSimple(repo)

	sesenta := 60
	if err := svc.EditarLicencia(context.Background(), "lic-1", EditarLicenciaParams{
		DiasDuracion: &sesenta,
		Vencimiento:  VencimientoDeclarado{RenovadaEl: &renovadaEl},
		PorUsuario:   "admin-1",
	}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// El vencimiento nuevo sale de la duración NUEVA: el orden importa.
	editada := repo.licencias["lic-1"]
	if !editada.FechaVencimiento.Equal(dia(2026, time.February, 19)) {
		t.Errorf("vencimiento = %v, esperaba 2026-02-19 (21 de diciembre + 60)", *editada.FechaVencimiento)
	}
}

func TestEditarLicencia_CargarLaFechaPorPrimeraVez(t *testing.T) {
	// El camino que sí puede darle fecha a una licencia sin verificar: hay
	// que decir CÓMO se sabe.
	repo := repoConCarroYPCs(1)
	sinFecha, err := domain.NuevaLicencia("lic-1", "equipo-1", "AutoCAD 2027", 30, 1, hoyDeTest)
	if err != nil {
		t.Fatalf("error de dominio inesperado: %v", err)
	}
	repo.licencias["lic-1"] = sinFecha
	svc := servicioSimple(repo)

	quedan := 12
	if err := svc.EditarLicencia(context.Background(), "lic-1", EditarLicenciaParams{
		Vencimiento: VencimientoDeclarado{QuedanDias: &quedan}, PorUsuario: "admin-1",
	}); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !repo.licencias["lic-1"].FechaVencimiento.Equal(dia(2026, time.January, 13)) {
		t.Errorf("vencimiento = %v, esperaba 2026-01-13", *repo.licencias["lic-1"].FechaVencimiento)
	}
}

func TestEditarLicencia_NoEncontrada(t *testing.T) {
	svc := servicioSimple(nuevoFakeRepo())

	err := svc.EditarLicencia(context.Background(), "no-existe", EditarLicenciaParams{})

	if !errors.Is(err, ErrLicenciaNoEncontrada) {
		t.Errorf("esperaba ErrLicenciaNoEncontrada, obtuve %v", err)
	}
}

func TestBorrarLicencia(t *testing.T) {
	repo := repoConCarroYPCs(1)
	licenciaCargada(t, repo, "lic-1", "equipo-1", dia(2026, time.January, 20))
	svc := servicioSimple(repo)
	ctx := context.Background()

	if err := svc.BorrarLicencia(ctx, "lic-1"); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if err := svc.BorrarLicencia(ctx, "lic-1"); !errors.Is(err, ErrLicenciaNoEncontrada) {
		t.Errorf("borrar dos veces debería dar ErrLicenciaNoEncontrada, obtuve %v", err)
	}
}
