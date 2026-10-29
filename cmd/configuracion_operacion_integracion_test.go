//go:build integration

package main

import (
	"context"
	"errors"
	"testing"

	cfgapp "github.com/agustin-y2k/sgrc/internal/configuracion/application"
	cfgdomain "github.com/agustin-y2k/sgrc/internal/configuracion/domain"
	cfginfra "github.com/agustin-y2k/sgrc/internal/configuracion/infrastructure"
)

func TestConfiguracionOperacion_PersistenciaVersionYAuditoriaAtomica(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	ctx := context.Background()
	repo := cfginfra.NewRepo(pool)
	svc := cfgapp.NewService(repo)
	inicial := cfgdomain.Operacion{RetiroGraciaMinutos: 40, RecordatorioAntelacionMinutos: 60, CierreGraciaMinutos: 60, CierreHoraSinJornada: 23, LicenciasHoraAviso: 8, Version: 1}
	if err := repo.Inicializar(ctx, inicial); err != nil {
		t.Fatal(err)
	}
	actual, err := svc.Leer(ctx)
	if err != nil || actual != inicial {
		t.Fatalf("inicio: %+v %v", actual, err)
	}
	actual.RetiroGraciaMinutos = 10
	actual.CierreGraciaMinutos = 0
	guardada, err := svc.Guardar(ctx, actual, e.d.docente, "127.0.0.1")
	if err != nil || guardada.Version != 2 {
		t.Fatalf("guardar: %+v %v", guardada, err)
	}
	if err = repo.Inicializar(ctx, inicial); err != nil {
		t.Fatal(err)
	}
	if err := repo.InicializarSiFalta(ctx, func() cfgdomain.Operacion { t.Fatal("releyó el entorno al reiniciar"); return inicial }); err != nil {
		t.Fatal(err)
	}
	leida, err := svc.Leer(ctx)
	if err != nil || leida != guardada {
		t.Fatal("el reinicio sobrescribió la configuración")
	}
	if _, err = svc.Guardar(ctx, actual, e.d.docente, ""); !errors.Is(err, cfgapp.ErrDesactualizada) {
		t.Fatalf("sobrescritura de otra sesión: %v", err)
	}
	var auditorias int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE accion='CONFIGURACION_OPERATIVA_CAMBIADA' AND detalle->'antes'->>'retiroGraciaMinutos'='40' AND detalle->'despues'->>'retiroGraciaMinutos'='10'`).Scan(&auditorias); err != nil || auditorias != 1 {
		t.Fatalf("auditoría: %d %v", auditorias, err)
	}
	// Un fallo real al registrar la auditoría debe deshacer la nueva configuración.
	ejecutarCierreSQL(t, pool, `CREATE FUNCTION impedir_config_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.accion='CONFIGURACION_OPERATIVA_CAMBIADA' THEN RAISE EXCEPTION 'fallo de auditoría'; END IF; RETURN NEW; END $$`)
	ejecutarCierreSQL(t, pool, `CREATE TRIGGER impedir_config_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION impedir_config_audit()`)
	guardada.RetiroGraciaMinutos = 20
	if _, err = svc.Guardar(ctx, guardada, e.d.docente, ""); err == nil {
		t.Fatal("confirmó sin auditoría")
	}
	leida, err = svc.Leer(ctx)
	if err != nil || leida.RetiroGraciaMinutos != 10 || leida.Version != 2 {
		t.Fatalf("no revirtió: %+v %v", leida, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE configuracion_operacion SET retiro_gracia_minutos=0`); err == nil {
		t.Fatal("la base aceptó un plazo inválido")
	}
}
