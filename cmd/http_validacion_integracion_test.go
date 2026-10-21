//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	auditoriaapp "github.com/agustin-y2k/sgrc/internal/auditoria/application"
	auditoriainfra "github.com/agustin-y2k/sgrc/internal/auditoria/infrastructure"
	auditoriahttp "github.com/agustin-y2k/sgrc/internal/auditoria/interfaces/http"
	authinfra "github.com/agustin-y2k/sgrc/internal/auth/infrastructure"
	reservationhttp "github.com/agustin-y2k/sgrc/internal/reservation/interfaces/http"
	"github.com/agustin-y2k/sgrc/internal/shared/authtest"
	"github.com/agustin-y2k/sgrc/internal/shared/middleware"
	sugerenciashttp "github.com/agustin-y2k/sgrc/internal/sugerencias/interfaces/http"
)

func autenticacionHTTPReal(pool *pgxpool.Pool) (middleware.Autenticacion, func(string, string) string) {
	secret := []byte("clave-de-pruebas-http-con-postgres")
	registro := authtest.Nuevo()
	return middleware.Autenticacion{Secret: secret, Vigente: authinfra.NewVerificadorCuentaVigente(pool).Vigente}, func(id, rol string) string { return registro.Token(secret, id, rol) }
}
func solicitudBackend(app *fiber.App, metodo, ruta, token, cuerpo string) (int, []byte, error) {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, 10000)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(resp.Body)
	return resp.StatusCode, datos, err
}
func pedirBackend(t *testing.T, app *fiber.App, metodo, ruta, token, cuerpo string, esperado int) map[string]json.RawMessage {
	t.Helper()
	estado, datos, err := solicitudBackend(app, metodo, ruta, token, cuerpo)
	if err != nil || estado != esperado {
		t.Fatalf("%s %s: estado=%d, esperado=%d, error=%v, cuerpo=%s", metodo, ruta, estado, esperado, err, datos)
	}
	if estado != 200 && estado != 201 {
		return nil
	}
	var resultado map[string]json.RawMessage
	if err := json.Unmarshal(datos, &resultado); err != nil {
		t.Fatal(err)
	}
	return resultado
}

func TestHTTPBackend_SoporteYAuditoriaPostgres(t *testing.T) {
	pool := baseVacia(t)
	e := prepararBajaReal(t, pool, false)
	ctx := context.Background()
	var adminID string
	if err := pool.QueryRow(ctx, `SELECT id FROM usuario WHERE rol='ADMIN'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	otroID := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Otra','Docente','otra@test.local','hash','DOCENTE','APROBADA')`, otroID)
	bus := busDurableReal(t, pool)
	svc := sugerenciasDurables(pool, bus)
	app := fiber.New()
	aut, token := autenticacionHTTPReal(pool)
	docente, admin, otro := token(e.usuario, "DOCENTE"), token(adminID, "ADMIN"), token(otroID, "DOCENTE")
	sugerenciashttp.RegisterRoutes(app, sugerenciashttp.NewHandler(svc), aut)
	zona, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}
	auditoriahttp.RegisterRoutes(app, auditoriahttp.NewHandler(auditoriaapp.NewService(auditoriainfra.NewPostgresRepo(pool, zona))), aut)
	alta := pedirBackend(t, app, "POST", "/api/sugerencias", docente, `{"tipo":"AYUDA","asunto":"Consulta HTTP","texto":"Texto inicial","usuarioId":"ajeno"}`, 201)
	var hiloID string
	if err := json.Unmarshal(alta["id"], &hiloID); err != nil {
		t.Fatal(err)
	}
	detalleRuta := "/api/sugerencias/" + hiloID
	pedirBackend(t, app, "GET", "/api/sugerencias/invalid", admin, "", 400)
	pedirBackend(t, app, "GET", detalleRuta, "", "", 401)
	pedirBackend(t, app, "GET", detalleRuta, otro, "", 403)
	for _, sesion := range []string{docente, admin} {
		detalle := pedirBackend(t, app, "GET", detalleRuta, sesion, "", 200)
		if string(detalle["id"]) != string(alta["id"]) {
			t.Fatal("detalle incorrecto")
		}
	}
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM sugerencia WHERE id=$1 AND usuario_id=$2`, hiloID, e.usuario)
	propias := pedirBackend(t, app, "GET", "/api/mis-sugerencias?usuarioId="+e.usuario, otro, "", 200)
	if string(propias["data"]) != "[]" {
		t.Fatal("otra cuenta pudo leer el hilo")
	}
	for _, ruta := range []string{"/api/sugerencias", "/api/auditoria", "/api/auditoria/opciones"} {
		pedirBackend(t, app, "GET", ruta, "", "", 401)
		pedirBackend(t, app, "GET", ruta, docente, "", 403)
	}
	mensajes := "/api/sugerencias/" + hiloID + "/mensajes"
	pedirBackend(t, app, "POST", mensajes, otro, `{"texto":"No permitido"}`, 403)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM sugerencia_mensaje WHERE sugerencia_id=$1`, hiloID)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM evento_pendiente`)
	quitar := falloBajaSQL(t, pool, "evento_pendiente", "INSERT", "")
	pedirBackend(t, app, "POST", mensajes, admin, `{"texto":"Respuesta que debe revertirse"}`, 500)
	quitar()
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM sugerencia_mensaje WHERE sugerencia_id=$1`, hiloID)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM evento_pendiente`)
	pedirBackend(t, app, "POST", mensajes, admin, `{"texto":"Respuesta confirmada"}`, 200)
	pedirBackend(t, app, "POST", "/api/sugerencias/"+hiloID+"/resolver", admin, `{}`, 200)
	pedirBackend(t, app, "POST", mensajes, docente, `{"texto":"Seguimiento"}`, 200)
	if _, err := bus.Despachar(ctx, 50); err != nil {
		t.Fatal(err)
	}
	comprobarCantidad(t, pool, 0, `SELECT count(*) FROM evento_pendiente WHERE procesado_en IS NULL`)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND usuario_id=$2 AND tipo='SUGERENCIA' AND estado='NO_LEIDA'`, hiloID, adminID)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM notificacion WHERE hilo_id=$1 AND usuario_id=$2 AND tipo='SUGERENCIA_RESPONDIDA'`, hiloID, e.usuario)
	if _, err := svc.Escribir(ctx, otroID, "PROBLEMA", "Otro hilo", "Texto", "", ""); err != nil {
		t.Fatal(err)
	}
	listado := pedirBackend(t, app, "GET", "/api/sugerencias?abiertas=true&pageSize=1", admin, "", 200)
	var meta struct {
		Total    int
		PageSize int
	}
	if err := json.Unmarshal(listado["meta"], &meta); err != nil {
		t.Fatal(err)
	}
	var data []map[string]json.RawMessage
	if err := json.Unmarshal(listado["data"], &data); err != nil {
		t.Fatal(err)
	}
	if meta.Total != 2 || meta.PageSize != 1 || len(data) != 1 || len(data[0]["usuarioId"]) == 0 {
		t.Fatalf("listado=%v", listado)
	}
	propias = pedirBackend(t, app, "GET", "/api/mis-sugerencias", docente, "", 200)
	data = nil
	if err := json.Unmarshal(propias["data"], &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || string(data[0]["id"]) != string(alta["id"]) || len(data[0]["usuarioId"]) != 0 {
		t.Fatal("privacidad del listado propio incorrecta")
	}
	actor := uuid.NewString()
	instantes := []string{"2026-05-04T02:59:59Z", "2026-05-04T03:00:00Z", "2026-05-05T02:59:59Z", "2026-05-05T03:00:00Z"}
	for _, instante := range instantes {
		ejecutarCierreSQL(t, pool, `INSERT INTO audit_log(usuario_id,accion,entidad,entidad_id,creado_en) VALUES($1,'PRUEBA_HTTP','equipo',$2,$3::timestamptz)`, actor, e.materias[0], instante)
	}
	auditoria := pedirBackend(t, app, "GET", "/api/auditoria?accion=PRUEBA_HTTP&entidad=equipo&entidadId="+e.materias[0]+"&usuarioId="+actor+"&desde=2026-05-04&hasta=2026-05-04&page=2&pageSize=1", admin, "", 200)
	if err := json.Unmarshal(auditoria["meta"], &meta); err != nil {
		t.Fatal(err)
	}
	data = nil
	if err := json.Unmarshal(auditoria["data"], &data); err != nil {
		t.Fatal(err)
	}
	if meta.Total != 2 || len(data) != 1 || string(data[0]["actorId"]) != `"`+actor+`"` || len(data[0]["actorNombre"]) != 0 {
		t.Fatalf("auditoría=%v", auditoria)
	}
	pedirBackend(t, app, "GET", "/api/auditoria?usuarioId=no-es-uuid", admin, "", 400)
	pedirBackend(t, app, "GET", "/api/auditoria/opciones", admin, "", 200)
	ejecutarCierreSQL(t, pool, `UPDATE usuario SET version_sesion=version_sesion+1 WHERE id=$1`, adminID)
	pedirBackend(t, app, "GET", "/api/auditoria", admin, "", 401)
	pedirBackend(t, app, "POST", "/api/sugerencias/"+hiloID+"/resolver", admin, `{}`, 401)
}

func TestHTTPBackend_CargaConcurrenteSinDobleReserva(t *testing.T) {
	pool := baseVacia(t)
	e := prepararInstitucion(t, pool)
	adminID := uuid.NewString()
	ejecutarCierreSQL(t, pool, `INSERT INTO usuario(id,nombre,apellido,email,password_hash,rol,estado) VALUES($1,'Admin','Carga','carga@test.local','hash','ADMIN','APROBADA')`, adminID)
	app := fiber.New()
	aut, token := autenticacionHTTPReal(pool)
	admin := token(adminID, "ADMIN")
	reservationhttp.RegisterRoutes(app, reservationhttp.NewHandler(e.reservas, nil), aut)
	auditoriahttp.RegisterRoutes(app, auditoriahttp.NewHandler(auditoriaapp.NewService(auditoriainfra.NewPostgresRepo(pool))), aut)
	sugerenciashttp.RegisterRoutes(app, sugerenciashttp.NewHandler(sugerenciasDurables(pool, busDurableReal(t, pool))), aut)
	cuerpo := fmt.Sprintf(`{"materiaId":%q,"fecha":"2026-05-11","horaInicio":"08:00","horaFin":"09:00","equipoIds":[%q]}`, e.d.materia, e.d.equipo)
	type resultado struct {
		metodo   string
		estado   int
		duracion time.Duration
		err      error
	}
	const escritores = 25
	const lectores = 75
	inicio := make(chan struct{})
	resultados := make(chan resultado, escritores+lectores)
	var wg sync.WaitGroup
	for i := range escritores + lectores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			metodo, ruta, body := "GET", "/api/auditoria?pageSize=5", ""
			if i < escritores {
				metodo, ruta, body = "POST", "/api/reservas", cuerpo
			} else if i%2 == 0 {
				ruta = "/api/sugerencias?pageSize=5"
			}
			desde := time.Now()
			estado, _, err := solicitudBackend(app, metodo, ruta, admin, body)
			resultados <- resultado{metodo, estado, time.Since(desde), err}
		}()
	}
	close(inicio)
	wg.Wait()
	close(resultados)
	creadas, conflictos, leidas := 0, 0, 0
	duraciones := make([]time.Duration, 0, escritores+lectores)
	for r := range resultados {
		if r.err != nil {
			t.Fatal(r.err)
		}
		duraciones = append(duraciones, r.duracion)
		switch {
		case r.metodo == "POST" && r.estado == 201:
			creadas++
		case r.metodo == "POST" && r.estado == 409:
			conflictos++
		case r.metodo == "GET" && r.estado == 200:
			leidas++
		default:
			t.Fatalf("respuesta inesperada: %+v", r)
		}
	}
	if creadas != 1 || conflictos != escritores-1 || leidas != lectores {
		t.Fatalf("creadas=%d conflictos=%d lecturas=%d", creadas, conflictos, leidas)
	}
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM reserva WHERE equipo_id=$1 AND fecha='2026-05-11' AND estado='CONFIRMADA'`, e.d.equipo)
	comprobarCantidad(t, pool, 1, `SELECT count(*) FROM reserva_grupo WHERE fecha='2026-05-11'`)
	sort.Slice(duraciones, func(i, j int) bool { return duraciones[i] < duraciones[j] })
	t.Logf("100 solicitudes simultáneas: 1 alta, 24 conflictos y 75 lecturas; p50=%s p95=%s máximo=%s (medición local, sin umbral de rendimiento)", duraciones[49], duraciones[94], duraciones[99])
}
