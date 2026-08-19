//go:build integration

package infrastructure

import (
	"context"
	"testing"

	"github.com/ramiro/sgrc/internal/notification/domain"
)

func TestPreferenciasEmail_GuardarYLeer(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	prefs := NewPreferenciasEmailPostgres(pool)
	ctx := context.Background()
	admin := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")

	// Arranca sin filas: no eligió nada todavía, y ahí mandan los defaults.
	iniciales, err := prefs.ElegidasDe(ctx, admin)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(iniciales) != 0 {
		t.Fatalf("esperaba ninguna decisión guardada, obtuve %v", iniciales)
	}

	if err := prefs.Reemplazar(ctx, admin, domain.Decisiones(
		[]domain.CategoriaEmail{domain.CatCuentaPendiente, domain.CatSugerencia}, true)); err != nil {
		t.Fatalf("guardando: %v", err)
	}

	guardadas, err := prefs.ElegidasDe(ctx, admin)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	// Una fila por categoría configurable: las dos encendidas y el resto
	// apagadas. Las de la cuenta no se guardan nunca.
	if len(guardadas) != len(domain.Configurables(true)) {
		t.Fatalf("esperaba una fila por categoría configurable, obtuve %v", guardadas)
	}
	if !guardadas[domain.CatCuentaPendiente] || !guardadas[domain.CatSugerencia] {
		t.Errorf("no quedaron encendidas las dos elegidas: %v", guardadas)
	}
	if guardadas[domain.CatLicenciaPorVencer] {
		t.Errorf("quedó encendida una que no se eligió: %v", guardadas)
	}
}

// Reemplazar es reemplazar: lo que no viene en la lista nueva deja de estar,
// aunque estuviera antes.
func TestPreferenciasEmail_Reemplazar_NoAcumula(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	prefs := NewPreferenciasEmailPostgres(pool)
	ctx := context.Background()
	admin := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")

	if err := prefs.Reemplazar(ctx, admin, domain.Decisiones(
		[]domain.CategoriaEmail{domain.CatCuentaPendiente, domain.CatSugerencia}, true)); err != nil {
		t.Fatalf("guardando: %v", err)
	}
	if err := prefs.Reemplazar(ctx, admin, domain.Decisiones(
		[]domain.CategoriaEmail{domain.CatSugerencia}, true)); err != nil {
		t.Fatalf("reemplazando: %v", err)
	}

	quedaron, err := prefs.ElegidasDe(ctx, admin)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if !quedaron[domain.CatSugerencia] || quedaron[domain.CatCuentaPendiente] {
		t.Fatalf("esperaba solo el buzón encendido, quedó %v", quedaron)
	}

	// Y destildar todo también se guarda, incluida la que arranca encendida:
	// si esto no quedara escrito, volvería sola en la próxima lectura.
	if err := prefs.Reemplazar(ctx, admin, domain.Decisiones(nil, true)); err != nil {
		t.Fatalf("vaciando: %v", err)
	}
	vacias, err := prefs.ElegidasDe(ctx, admin)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	for categoria, activa := range vacias {
		if activa {
			t.Errorf("se destildó todo y %s quedó encendida", categoria)
		}
	}
}

// El CHECK de la tabla y la lista de Go tienen que decir lo mismo: si alguien
// agrega una categoría configurable en un solo lado, este test falla al
// guardarla.
func TestPreferenciasEmail_TodasLasConfigurablesEntranEnLaTabla(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	prefs := NewPreferenciasEmailPostgres(pool)
	ctx := context.Background()
	admin := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")

	todas := domain.Configurables(true)
	if err := prefs.Reemplazar(ctx, admin, domain.Decisiones(todas, true)); err != nil {
		t.Fatalf("la base rechazó alguna categoría que la aplicación conoce: %v", err)
	}

	guardadas, err := prefs.ElegidasDe(ctx, admin)
	if err != nil {
		t.Fatalf("leyendo: %v", err)
	}
	if len(guardadas) != len(todas) {
		t.Fatalf("esperaba las %d, volvieron %d: %v", len(todas), len(guardadas), guardadas)
	}
}

// El filtro del correo: Admin aprobado + que reciba esa categoría, contando
// el valor por defecto de quien nunca abrió el panel.
func TestListadorAdminsPostgres_EmailsDeAdminsSuscriptos(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	prefs := NewPreferenciasEmailPostgres(pool)
	listador := NewListadorAdminsPostgres(pool)
	ctx := context.Background()

	suscripto := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	sinElegir := crearUsuarioDeTest(t, pool, "ADMIN", "APROBADA")
	adminPendiente := crearUsuarioDeTest(t, pool, "ADMIN", "PENDIENTE")
	docente := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	// Los tres eligen lo mismo; solo uno cumple las dos condiciones. El cuarto
	// (sinElegir) no tiene ninguna fila: se rige por los defaults.
	for _, id := range []string{suscripto, adminPendiente, docente} {
		if err := prefs.Reemplazar(ctx, id, domain.Decisiones(
			[]domain.CategoriaEmail{domain.CatLicenciaPorVencer}, true)); err != nil {
			t.Fatalf("guardando preferencia de %s: %v", id, err)
		}
	}

	emails, err := listador.EmailsDeAdminsSuscriptos(ctx, domain.CatLicenciaPorVencer)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(emails) != 1 || emails[0] != suscripto+"@escuela.edu.ar" {
		t.Fatalf("esperaba solo al Admin aprobado que la tildó, obtuve %v", emails)
	}

	// Las cuentas pendientes arrancan encendidas: le llega a quien nunca
	// eligió, y NO a quien guardó el panel sin tildarla.
	cuentas, err := listador.EmailsDeAdminsSuscriptos(ctx, domain.CatCuentaPendiente)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if len(cuentas) != 1 || cuentas[0] != sinElegir+"@escuela.edu.ar" {
		t.Fatalf("esperaba solo al Admin que no abrió el panel, obtuve %v", cuentas)
	}
}

// El otro filtro, el de los correos personales: qué contesta la base para una
// dirección que eligió, para una que no, y para una que ni siquiera tiene
// cuenta (pasa con las entregas a nombre de alguien de afuera).
func TestPreferenciasEmail_RecibePorEmail(t *testing.T) {
	pool := levantarPostgresDeTest(t)
	prefs := NewPreferenciasEmailPostgres(pool)
	ctx := context.Background()

	eligio := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")
	sinElegir := crearUsuarioDeTest(t, pool, "DOCENTE", "APROBADA")

	// Pide el recordatorio (que arranca apagado) y apaga el aviso de equipo no
	// disponible (que arranca encendido): las dos cosas al revés del default.
	if err := prefs.Reemplazar(ctx, eligio, domain.Decisiones(
		[]domain.CategoriaEmail{domain.CatRecordatorioDeReserva}, false)); err != nil {
		t.Fatalf("guardando: %v", err)
	}

	casos := []struct {
		nombre    string
		email     string
		categoria domain.CategoriaEmail
		esperado  bool
	}{
		{"eligió recibirlo", eligio + "@escuela.edu.ar", domain.CatRecordatorioDeReserva, true},
		{"eligió no recibirlo", eligio + "@escuela.edu.ar", domain.CatEquipoNoDisponible, false},
		{"no eligió, default apagado", sinElegir + "@escuela.edu.ar", domain.CatRecordatorioDeReserva, false},
		{"no eligió, default encendido", sinElegir + "@escuela.edu.ar", domain.CatEquipoNoDisponible, true},
		{"sin cuenta, vale el default", "de-afuera@otra-escuela.edu.ar", domain.CatEquipoNoDisponible, true},
		{"sin cuenta y default apagado", "de-afuera@otra-escuela.edu.ar", domain.CatDevolucionPendiente, false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			recibe, err := prefs.RecibePorEmail(ctx, caso.email, caso.categoria)
			if err != nil {
				t.Fatalf("no debería fallar: %v", err)
			}
			if recibe != caso.esperado {
				t.Errorf("%s / %s: esperaba %t, obtuve %t", caso.email, caso.categoria, caso.esperado, recibe)
			}
		})
	}
}
