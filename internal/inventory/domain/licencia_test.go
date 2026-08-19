package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fecha arma una fecha calendario en la misma forma canónica que usa el
// dominio (medianoche UTC), para poder comparar con Equal sin sorpresas.
func fecha(anio int, mes time.Month, dia int) time.Time {
	return time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)
}

// enBuenosAires devuelve el mismo día pero a una hora de la tarde y con el
// offset de la escuela.
func enBuenosAires(t *testing.T, anio int, mes time.Month, dia, hora int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("cargando zona horaria: %v", err)
	}
	return time.Date(anio, mes, dia, hora, 30, 0, 0, loc)
}

func licenciaDePrueba(t *testing.T, diasDuracion, diasAviso int) *LicenciaSoftware {
	t.Helper()
	l, err := NuevaLicencia("lic-1", "equipo-1", "AutoCAD 2027", diasDuracion, diasAviso, fecha(2026, time.August, 1))
	if err != nil {
		t.Fatalf("NuevaLicencia no debería fallar: %v", err)
	}
	return l
}

// ── Alta ────────────────────────────────────────────────────────────────

func TestNuevaLicencia_NaceSinFecha(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)

	if l.FechaVencimiento != nil {
		t.Errorf("una licencia recién creada no debería tener vencimiento, tiene %v", *l.FechaVencimiento)
	}
	if _, tiene := l.DiasRestantes(fecha(2026, time.August, 1)); tiene {
		t.Error("DiasRestantes debería devolver false mientras no haya fecha")
	}
	if got := l.Estado(fecha(2026, time.August, 1)); got != LicenciaSinFecha {
		t.Errorf("Estado = %q, esperaba %q", got, LicenciaSinFecha)
	}
}

func TestNuevaLicencia_NormalizaElNombre(t *testing.T) {
	l, err := NuevaLicencia("lic-1", "equipo-1", "  AutoCAD 2027  ", 30, 1, fecha(2026, time.August, 1))
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	// Recorta los bordes pero NO cambia mayúsculas: el nombre se muestra.
	if l.Nombre != "AutoCAD 2027" {
		t.Errorf("Nombre = %q, esperaba %q", l.Nombre, "AutoCAD 2027")
	}
}

func TestNuevaLicencia_Invalidas(t *testing.T) {
	casos := []struct {
		nombre       string
		queSeCarga   string
		diasDuracion int
		diasAviso    int
		esperado     error
	}{
		{"nombre vacío", "", 30, 1, ErrNombreLicenciaVacio},
		{"nombre de puros espacios", "   ", 30, 1, ErrNombreLicenciaVacio},
		{"nombre demasiado largo", strings.Repeat("a", MaxLargoNombreLicencia+1), 30, 1, ErrNombreLicenciaLargo},
		{"duración cero", "AutoCAD", 0, 1, ErrDiasDuracionInvalido},
		{"duración negativa", "AutoCAD", -30, 1, ErrDiasDuracionInvalido},
		{"duración fuera de tope", "AutoCAD", MaxDiasDuracion + 1, 1, ErrDiasDuracionInvalido},
		{"aviso negativo", "AutoCAD", 30, -1, ErrDiasAvisoInvalido},
		{"aviso fuera de tope", "AutoCAD", 30, MaxDiasAviso + 1, ErrDiasAvisoInvalido},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := NuevaLicencia("lic-1", "equipo-1", c.queSeCarga, c.diasDuracion, c.diasAviso, fecha(2026, time.August, 1))
			if !errors.Is(err, c.esperado) {
				t.Errorf("esperaba %v, obtuve %v", c.esperado, err)
			}
		})
	}
}

func TestNuevaLicencia_NombreEnElLimiteEntra(t *testing.T) {
	_, err := NuevaLicencia("lic-1", "equipo-1", strings.Repeat("a", MaxLargoNombreLicencia), 30, 1, fecha(2026, time.August, 1))
	if err != nil {
		t.Errorf("un nombre de exactamente %d caracteres debería entrar: %v", MaxLargoNombreLicencia, err)
	}
}

// ── Las tres formas de fijar el vencimiento ─────────────────────────────

func TestRenovadaEl_CorreElVencimientoDesdeLaRenovacion(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	ahora := fecha(2026, time.August, 7)

	// El caso que motivó todo esto: se renovó el martes y se carga el jueves.
	l.RenovadaEl(fecha(2026, time.August, 4), "admin-1", ahora)

	if !l.FechaVencimiento.Equal(fecha(2026, time.September, 3)) {
		t.Errorf("vencimiento = %v, esperaba 2026-09-03 (4 de agosto + 30 días)", *l.FechaVencimiento)
	}
	if !l.UltimaRenovacion.Equal(fecha(2026, time.August, 4)) {
		t.Errorf("ultimaRenovacion = %v, esperaba el día que se renovó y no el que se cargó", *l.UltimaRenovacion)
	}
	if l.VencimientoFijadoPor == nil || *l.VencimientoFijadoPor != "admin-1" {
		t.Errorf("vencimientoFijadoPor = %v, esperaba admin-1", l.VencimientoFijadoPor)
	}
	if l.VencimientoFijadoEn == nil || !l.VencimientoFijadoEn.Equal(ahora) {
		t.Errorf("vencimientoFijadoEn = %v, esperaba el instante de la carga (%v)", l.VencimientoFijadoEn, ahora)
	}
}

func TestVenceEnDias_BorraLaUltimaRenovacion(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	ahora := fecha(2026, time.August, 7)
	l.RenovadaEl(fecha(2026, time.August, 4), "admin-1", ahora)

	// Ahora alguien se sienta delante de la máquina y lee "quedan 12 días".
	l.VenceEnDias(12, fecha(2026, time.August, 7), "admin-2", ahora)

	if !l.FechaVencimiento.Equal(fecha(2026, time.August, 19)) {
		t.Errorf("vencimiento = %v, esperaba 2026-08-19", *l.FechaVencimiento)
	}
	// Lo importante: la renovación anterior ya no explica este vencimiento,
	// así que no puede quedar mostrándose al lado.
	if l.UltimaRenovacion != nil {
		t.Errorf("ultimaRenovacion debería quedar vacía, quedó %v", *l.UltimaRenovacion)
	}
}

func TestFijarVencimiento_BorraLaUltimaRenovacion(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	ahora := fecha(2026, time.August, 7)
	l.RenovadaEl(fecha(2026, time.August, 4), "admin-1", ahora)

	l.FijarVencimiento(fecha(2026, time.September, 10), "admin-2", ahora)

	if !l.FechaVencimiento.Equal(fecha(2026, time.September, 10)) {
		t.Errorf("vencimiento = %v, esperaba 2026-09-10", *l.FechaVencimiento)
	}
	if l.UltimaRenovacion != nil {
		t.Errorf("ultimaRenovacion debería quedar vacía, quedó %v", *l.UltimaRenovacion)
	}
}

func TestRenovar_SinFechaPreviaFalla(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)

	err := l.Renovar(fecha(2026, time.August, 7), "admin-1", fecha(2026, time.August, 7))

	if !errors.Is(err, ErrSinFechaDeVencimiento) {
		t.Errorf("esperaba ErrSinFechaDeVencimiento, obtuve %v", err)
	}
	if l.FechaVencimiento != nil {
		t.Error("una renovación rechazada no debería dejar fecha cargada")
	}
}

func TestRenovar_CorreDesdeHoy(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	hoy := fecha(2026, time.August, 7)
	l.FijarVencimiento(fecha(2026, time.August, 8), "admin-1", hoy)

	if err := l.Renovar(hoy, "admin-1", hoy); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !l.FechaVencimiento.Equal(fecha(2026, time.September, 6)) {
		t.Errorf("vencimiento = %v, esperaba 2026-09-06 (hoy + 30)", *l.FechaVencimiento)
	}
	if !l.UltimaRenovacion.Equal(hoy) {
		t.Errorf("ultimaRenovacion = %v, esperaba hoy", *l.UltimaRenovacion)
	}
}

// ── El contador ─────────────────────────────────────────────────────────

func TestDiasRestantes_NoDependeDeLaHoraNiDeLaZona(t *testing.T) {
	// El escenario real: el vencimiento vuelve de Postgres como medianoche UTC y
	// "hoy" sale de la hora de la escuela (-03:00), a cualquier hora del día.
	l := licenciaDePrueba(t, 30, 1)
	l.FechaVencimiento = ptr(fecha(2026, time.September, 6))

	for _, hora := range []int{0, 3, 12, 23} {
		hoy := enBuenosAires(t, 2026, time.August, 7, hora)
		dias, tiene := l.DiasRestantes(hoy)
		if !tiene {
			t.Fatal("debería tener fecha")
		}
		if dias != 30 {
			t.Errorf("a las %02d h: DiasRestantes = %d, esperaba 30", hora, dias)
		}
	}
}

func TestDiasRestantes_Vencida(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	l.FechaVencimiento = ptr(fecha(2026, time.August, 4))

	dias, _ := l.DiasRestantes(fecha(2026, time.August, 7))

	if dias != -3 {
		t.Errorf("DiasRestantes = %d, esperaba -3 (venció hace tres días)", dias)
	}
}

func TestEstado(t *testing.T) {
	hoy := fecha(2026, time.August, 7)

	casos := []struct {
		nombre      string
		vencimiento *time.Time
		diasAviso   int
		esperado    EstadoLicencia
	}{
		{"sin fecha", nil, 1, LicenciaSinFecha},
		{"vence dentro de un mes", ptr(fecha(2026, time.September, 6)), 1, LicenciaVigente},
		{"vence mañana con aviso de 1 día", ptr(fecha(2026, time.August, 8)), 1, LicenciaPorVencer},
		{"vence en 5 días con aviso de 1", ptr(fecha(2026, time.August, 12)), 1, LicenciaVigente},
		{"vence en 5 días con aviso de 7", ptr(fecha(2026, time.August, 12)), 7, LicenciaPorVencer},
		{"vence hoy", ptr(hoy), 1, LicenciaVencida},
		{"venció ayer", ptr(fecha(2026, time.August, 6)), 1, LicenciaVencida},
		// dias_aviso = 0 es válido: "avisame recién el día que vence".
		{"vence mañana sin antelación", ptr(fecha(2026, time.August, 8)), 0, LicenciaVigente},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			l := licenciaDePrueba(t, 30, c.diasAviso)
			l.FechaVencimiento = c.vencimiento
			if got := l.Estado(hoy); got != c.esperado {
				t.Errorf("Estado = %q, esperaba %q", got, c.esperado)
			}
		})
	}
}

// ── Cambio de duración ──────────────────────────────────────────────────

func TestCambiarDuracion_NoMueveElVencimientoVigente(t *testing.T) {
	// De 30 a 60 días: la licencia que ya está instalada sigue venciendo cuando
	// vencía.
	l := licenciaDePrueba(t, 30, 1)
	ahora := fecha(2026, time.August, 7)
	l.RenovadaEl(fecha(2026, time.August, 4), "admin-1", ahora)
	vencimientoOriginal := *l.FechaVencimiento

	if err := l.CambiarDuracion(60); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	if !l.FechaVencimiento.Equal(vencimientoOriginal) {
		t.Errorf("vencimiento = %v, esperaba que siguiera en %v", *l.FechaVencimiento, vencimientoOriginal)
	}

	// Y la próxima renovación sí usa el valor nuevo.
	l.RenovadaEl(fecha(2026, time.August, 4), "admin-1", ahora)
	if !l.FechaVencimiento.Equal(fecha(2026, time.October, 3)) {
		t.Errorf("tras renovar: vencimiento = %v, esperaba 2026-10-03 (4 de agosto + 60)", *l.FechaVencimiento)
	}
}

func TestCambiarDuracion_Invalida(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	for _, dias := range []int{0, -1, MaxDiasDuracion + 1} {
		if err := l.CambiarDuracion(dias); !errors.Is(err, ErrDiasDuracionInvalido) {
			t.Errorf("CambiarDuracion(%d): esperaba ErrDiasDuracionInvalido, obtuve %v", dias, err)
		}
	}
	if l.DiasDuracion != 30 {
		t.Errorf("un cambio rechazado no debería tocar el valor: quedó en %d", l.DiasDuracion)
	}
}

// ── Los avisos ──────────────────────────────────────────────────────────

func TestCorrespondeAvisoPrevio_VentanaYAntelacion(t *testing.T) {
	hoy := fecha(2026, time.August, 7)

	casos := []struct {
		nombre      string
		vencimiento time.Time
		diasAviso   int
		esperado    bool
	}{
		{"todavía falta mucho", fecha(2026, time.September, 6), 1, false},
		{"vence mañana, aviso de 1 día", fecha(2026, time.August, 8), 1, true},
		{"vence en 7, aviso de 7", fecha(2026, time.August, 14), 7, true},
		{"vence en 8, aviso de 7", fecha(2026, time.August, 15), 7, false},
		// El día del vencimiento le toca al OTRO aviso, no a este: así
		// nunca salen dos avisos de la misma licencia el mismo día.
		{"vence hoy", hoy, 1, false},
		{"ya venció", fecha(2026, time.August, 6), 1, false},
		{"sin antelación configurada", fecha(2026, time.August, 8), 0, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			l := licenciaDePrueba(t, 30, c.diasAviso)
			l.FechaVencimiento = ptr(c.vencimiento)
			if got := l.CorrespondeAvisoPrevio(hoy); got != c.esperado {
				t.Errorf("CorrespondeAvisoPrevio = %v, esperaba %v", got, c.esperado)
			}
		})
	}
}

func TestCorrespondeAvisoPrevio_SinFechaNuncaAvisa(t *testing.T) {
	l := licenciaDePrueba(t, 30, 365)

	if l.CorrespondeAvisoPrevio(fecha(2026, time.August, 7)) {
		t.Error("una licencia sin fecha de vencimiento no puede disparar ningún aviso")
	}
	if l.CorrespondeAvisoDeVencimiento(fecha(2026, time.August, 7)) {
		t.Error("una licencia sin fecha de vencimiento no puede disparar ningún aviso")
	}
}

func TestCorrespondeAvisoDeVencimiento_DesdeElDiaQueVence(t *testing.T) {
	hoy := fecha(2026, time.August, 7)

	casos := []struct {
		nombre      string
		vencimiento time.Time
		esperado    bool
	}{
		{"vence mañana", fecha(2026, time.August, 8), false},
		{"vence hoy", hoy, true},
		// Con >= y no ==: si el proceso estuvo caído el día que vencía, el
		// aviso sale tarde en vez de perderse para siempre.
		{"venció hace tres días y nadie avisó", fecha(2026, time.August, 4), true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			l := licenciaDePrueba(t, 30, 1)
			l.FechaVencimiento = ptr(c.vencimiento)
			if got := l.CorrespondeAvisoDeVencimiento(hoy); got != c.esperado {
				t.Errorf("CorrespondeAvisoDeVencimiento = %v, esperaba %v", got, c.esperado)
			}
		})
	}
}

// TestAvisos_NoSeRepitenNiSePisan es la prueba de la idempotencia: el mismo
// día, corrido muchas veces, tiene que avisar una sola vez de cada cosa.
func TestAvisos_NoSeRepitenNiSePisan(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	l.FechaVencimiento = ptr(fecha(2026, time.August, 8))

	diaPrevio := fecha(2026, time.August, 7)

	if !l.CorrespondeAvisoPrevio(diaPrevio) {
		t.Fatal("el primer día debería corresponder el aviso previo")
	}
	l.MarcarAvisoPrevioEnviado()

	// Diez reinicios del contenedor en el mismo día: ni un mail más.
	for i := 0; i < 10; i++ {
		if l.CorrespondeAvisoPrevio(diaPrevio) {
			t.Fatalf("corrida %d: volvió a corresponder el aviso previo", i+1)
		}
	}

	// Al día siguiente vence: le toca el otro aviso, una sola vez.
	diaDelVencimiento := fecha(2026, time.August, 8)
	if !l.CorrespondeAvisoDeVencimiento(diaDelVencimiento) {
		t.Fatal("el día del vencimiento debería corresponder el aviso de vencimiento")
	}
	l.MarcarAvisoDeVencimientoEnviado()
	if l.CorrespondeAvisoDeVencimiento(diaDelVencimiento) {
		t.Error("el aviso de vencimiento salió dos veces")
	}

	// Y ya vencida, tampoco insiste los días siguientes.
	for dia := 9; dia <= 15; dia++ {
		if l.CorrespondeAvisoDeVencimiento(fecha(2026, time.August, dia)) {
			t.Errorf("el %d de agosto volvió a avisar de una licencia que ya avisó que venció", dia)
		}
	}
}

// TestAvisos_RenovarReabreElCiclo es la otra mitad de la idempotencia: las
// marcas no se resetean a mano en ningún lado, se vuelven obsoletas solas
// porque apuntan a la fecha de vencimiento vieja.
func TestAvisos_RenovarReabreElCiclo(t *testing.T) {
	l := licenciaDePrueba(t, 30, 1)
	l.FechaVencimiento = ptr(fecha(2026, time.August, 8))
	l.MarcarAvisoPrevioEnviado()

	hoy := fecha(2026, time.August, 7)
	if err := l.Renovar(hoy, "admin-1", hoy); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}

	// Nadie tocó AvisadoPrevioPara: sigue apuntando al 8 de agosto, que ya
	// no es el vencimiento.
	if l.AvisadoPrevioPara == nil || !l.AvisadoPrevioPara.Equal(fecha(2026, time.August, 8)) {
		t.Fatalf("la marca vieja debería seguir intacta, es %v", l.AvisadoPrevioPara)
	}

	// El día previo al vencimiento nuevo (6 de septiembre) vuelve a avisar.
	if !l.CorrespondeAvisoPrevio(fecha(2026, time.September, 5)) {
		t.Error("tras renovar, el ciclo nuevo debería volver a avisar")
	}
}

func ptr[T any](v T) *T { return &v }
