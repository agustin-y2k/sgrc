//go:build integration

// Este archivo solo se compila con la tag "integration" (go test
// -tags=integration ./...), porque levanta un Postgres real con
// testcontainers-go.
//
// Cubre el único camino que el resto de la suite no puede cubrir. Todos los
// demás tests de integración levantan una base VACÍA y le aplican todas las
// migraciones de una sola vez: ahí una 002 que borra media tabla pasa en
// verde, porque no había nada que borrar. El servidor de la institución hace
// lo contrario —tiene el esquema viejo y años de datos adentro— y es el único
// lugar donde ese error se ve, cuando ya es tarde.
package migrations_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ramiro/sgrc/internal/shared/testdb"
)

// versionDelPuntoDePartida es la 001: el esquema congelado del que parte toda
// instalación. Lo que este test simula es una base que se quedó ahí —la del
// servidor, después de meses funcionando— recibiendo la actualización.
//
// No se actualiza al agregar una 002: cuanto más atrás empieza, más largo es
// el camino que se ejercita. Solo cambiaría si algún día se decide que las
// instalaciones viejas de verdad no se soportan más.
const versionDelPuntoDePartida = 1

// Identificadores fijos para poder volver a buscar exactamente estas filas
// después de migrar, sin depender de un ORDER BY.
const (
	idAdmin   = "11111111-1111-1111-1111-111111111111"
	idDocente = "22222222-2222-2222-2222-222222222222"
	idCiclo   = "33333333-3333-3333-3333-333333333333"
	idCurso   = "44444444-4444-4444-4444-444444444444"
	idMateria = "55555555-5555-5555-5555-555555555555"
	idCarro   = "66666666-6666-6666-6666-666666666666"
	idEquipo  = "77777777-7777-7777-7777-777777777777"
	idGrupo   = "88888888-8888-8888-8888-888888888888"
	idReserva = "99999999-9999-9999-9999-999999999999"
)

func TestUnaActualizacionNoSeLlevaLosDatosPuestos(t *testing.T) {
	ctx := context.Background()
	pool, dsn := levantarPostgresDeTest(t)

	// 1. La base como quedó la última vez que se desplegó.
	if err := testdb.AplicarHasta(ctx, dsn, versionDelPuntoDePartida); err != nil {
		t.Fatalf("no se pudo armar el esquema del punto de partida: %v", err)
	}

	// 2. La institución la usó: docentes, cursos, equipos, reservas.
	sembrarInstalacionEnUso(ctx, t, pool)
	antes := contarFilasPorTabla(ctx, t, pool)

	// 3. La actualización. Si acá hay un error, el contenedor de producción
	//    no arranca: goose corre antes de que la aplicación escuche.
	if err := testdb.AplicarEsquema(ctx, dsn); err != nil {
		t.Fatalf("la actualización falló sobre una base con datos: %v\n\n"+
			"En el servidor esto es el contenedor que no levanta. Casi siempre es\n"+
			"una migración que da por sentado que la tabla está vacía: un NOT NULL\n"+
			"sin DEFAULT, un UNIQUE sobre datos que ya se repiten, un CHECK que las\n"+
			"filas viejas no cumplen.", err)
	}

	// 4. Nada de lo que había se perdió por el camino.
	verificarQueNingunaTablaPerdioFilas(ctx, t, pool, antes)
	verificarQueLaReservaSigueEntera(ctx, t, pool)
	verificarQueLaJornadaYaEstabaDecidida(ctx, t, pool)
	verificarQueLaVersionEsLaUltima(ctx, t, pool)
}

// sembrarInstalacionEnUso escribe una muestra de cada familia de datos que la
// escuela no puede perder. No busca ser exhaustiva: busca que toda cadena de
// claves foráneas larga —usuario → materia → reserva → equipo— tenga al menos
// un representante, porque son las que se rompen al reescribir una tabla.
func sembrarInstalacionEnUso(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ejecutar(ctx, t, pool, `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado)
		VALUES ($1, 'Ada', 'Directora', 'admin@escuela.test', 'hash', 'ADMIN', 'APROBADA')`, idAdmin)

	ejecutar(ctx, t, pool, `
		INSERT INTO usuario (id, nombre, apellido, email, password_hash, rol, estado, fecha_aprobacion, aprobado_por)
		VALUES ($1, 'Blas', 'Docente', 'docente@escuela.test', 'hash', 'DOCENTE', 'APROBADA', now(), $2)`, idDocente, idAdmin)

	ejecutar(ctx, t, pool, `INSERT INTO ciclo_lectivo (id, anio) VALUES ($1, 2026)`, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO curso (id, ciclo_lectivo_id, nombre) VALUES ($1, $2, '3°B')`, idCurso, idCiclo)
	ejecutar(ctx, t, pool, `INSERT INTO materia (id, curso_id, nombre) VALUES ($1, $2, 'Matemática')`, idMateria, idCurso)
	ejecutar(ctx, t, pool, `
		INSERT INTO docente_materia (usuario_id, materia_id, rol) VALUES ($1, $2, 'TITULAR')`, idDocente, idMateria)

	ejecutar(ctx, t, pool, `INSERT INTO carro (id, nombre) VALUES ($1, 'Carro 1')`, idCarro)
	ejecutar(ctx, t, pool, `
		INSERT INTO equipo (id, carro_id, identificador, tipo, numero_serie)
		VALUES ($1, $2, 7, 'PC', 'SERIE-007')`, idEquipo, idCarro)

	ejecutar(ctx, t, pool, `
		INSERT INTO reserva_grupo (id, materia_id, creado_por, nombre_docente_snapshot, fecha, hora_inicio, hora_fin)
		VALUES ($1, $2, $3, 'Blas Docente', DATE '2026-05-04', TIME '08:00', TIME '09:20')`, idGrupo, idMateria, idDocente)

	ejecutar(ctx, t, pool, `
		INSERT INTO reserva (id, reserva_grupo_id, equipo_id, materia_id, nombre_docente_snapshot,
		                     fecha, hora_inicio, hora_fin, creado_por)
		VALUES ($1, $2, $3, $4, 'Blas Docente', DATE '2026-05-04', TIME '08:00', TIME '09:20', $5)`,
		idReserva, idGrupo, idEquipo, idMateria, idDocente)

	ejecutar(ctx, t, pool, `
		INSERT INTO prestamo (equipo_id, reserva_id, entregado_a_usuario_id, entregado_a_nombre, entregado_por, devuelto_en)
		VALUES ($1, $2, $3, 'Blas Docente', $4, now())`, idEquipo, idReserva, idDocente, idAdmin)

	ejecutar(ctx, t, pool, `
		INSERT INTO incidencia (equipo_id, reportado_por, descripcion, gravedad)
		VALUES ($1, $2, 'La batería no carga', 'MODERADA')`, idEquipo, idDocente)

	ejecutar(ctx, t, pool, `
		INSERT INTO licencia_software (equipo_id, nombre, dias_duracion, fecha_vencimiento, vencimiento_fijado_por)
		VALUES ($1, 'Antivirus', 365, DATE '2027-03-01', $2)`, idEquipo, idAdmin)

	ejecutar(ctx, t, pool, `
		INSERT INTO notificacion (usuario_id, reserva_id, mensaje, tipo)
		VALUES ($1, $2, 'Tu reserva está por comenzar', 'RESERVA_POR_COMENZAR')`, idDocente, idReserva)

	ejecutar(ctx, t, pool, `
		INSERT INTO preferencia_email (usuario_id, categoria, activa)
		VALUES ($1, 'RECORDATORIO_DE_RESERVA', false)`, idDocente)

	ejecutar(ctx, t, pool, `
		INSERT INTO jornada_institucion (dia_semana, hora_inicio, hora_fin)
		VALUES ('LUNES', TIME '07:30', TIME '17:00')`)
}

// contarFilasPorTabla mira el esquema en vez de una lista escrita a mano: una
// tabla agregada el año que viene queda cubierta sin que nadie se acuerde de
// venir a anotarla acá.
func contarFilasPorTabla(ctx context.Context, t *testing.T, pool *pgxpool.Pool) map[string]int {
	t.Helper()

	filas, err := pool.Query(ctx, `
		SELECT table_name
		  FROM information_schema.tables
		 WHERE table_schema = 'public'
		   AND table_type = 'BASE TABLE'
		   AND table_name <> 'goose_db_version'`)
	if err != nil {
		t.Fatalf("no se pudieron listar las tablas: %v", err)
	}
	tablas, err := pgx.CollectRows(filas, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("no se pudieron leer los nombres de las tablas: %v", err)
	}

	conteo := make(map[string]int, len(tablas))
	for _, tabla := range tablas {
		var cantidad int
		consulta := fmt.Sprintf(`SELECT count(*) FROM %s`, pgx.Identifier{tabla}.Sanitize())
		if err := pool.QueryRow(ctx, consulta).Scan(&cantidad); err != nil {
			t.Fatalf("no se pudo contar %q: %v", tabla, err)
		}
		conteo[tabla] = cantidad
	}
	return conteo
}

func verificarQueNingunaTablaPerdioFilas(ctx context.Context, t *testing.T, pool *pgxpool.Pool, antes map[string]int) {
	t.Helper()
	despues := contarFilasPorTabla(ctx, t, pool)

	for tabla, cantidadAntes := range antes {
		cantidadDespues, sigue := despues[tabla]
		if !sigue {
			t.Errorf("la tabla %q desapareció con la actualización, y tenía %d fila(s)", tabla, cantidadAntes)
			continue
		}
		if cantidadDespues < cantidadAntes {
			t.Errorf("%q pasó de %d a %d fila(s): la actualización borró datos", tabla, cantidadAntes, cantidadDespues)
		}
	}
}

// verificarQueLaReservaSigueEntera es el contrapeso del conteo: una migración
// puede dejar la misma cantidad de filas y arruinarlas igual —perdiendo el
// vínculo con el equipo, corriendo un horario al pasarlo a otro tipo—.
// verificarQueLaJornadaYaEstabaDecidida cubre la única decisión de la 002 que
// no es estructural.
//
// La bandera arranca en FALSE para una instalación nueva, y eso manda al
// primer Admin a declarar la jornada. Pero esta base viene funcionando y ya
// tiene su jornada cargada hace meses: si la actualización la dejara en
// FALSE, el Admin abriría el sistema y se encontraría con un cuestionario
// pidiéndole que responda algo que ya respondió.
func verificarQueLaJornadaYaEstabaDecidida(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	var definida bool
	if err := pool.QueryRow(ctx,
		`SELECT jornada_definida FROM configuracion_institucion`).Scan(&definida); err != nil {
		t.Fatalf("la 002 tiene que dejar la fila única de configuración creada: %v", err)
	}
	if !definida {
		t.Error("la instalación ya tenía tramos cargados: su jornada ya estaba decidida")
	}

	// Y sigue siendo una sola fila: el CHECK sobre la clave primaria es lo
	// que impide que aparezca una segunda escribiendo directo contra la base.
	if _, err := pool.Exec(ctx,
		`INSERT INTO configuracion_institucion (unica, jornada_definida) VALUES (false, true)`); err == nil {
		t.Error("la configuración de la institución tiene que ser una sola fila")
	}
}

func verificarQueLaReservaSigueEntera(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	var (
		docente, materia, equipo string
		fecha                    time.Time
		horaInicio, horaFin      string
		estado                   string
	)
	err := pool.QueryRow(ctx, `
		SELECT u.email, m.nombre, e.numero_serie, r.fecha,
		       to_char(r.hora_inicio, 'HH24:MI'), to_char(r.hora_fin, 'HH24:MI'), r.estado
		  FROM reserva r
		  JOIN materia m ON m.id = r.materia_id
		  JOIN equipo  e ON e.id = r.equipo_id
		  JOIN usuario u ON u.id = r.creado_por
		 WHERE r.id = $1`, idReserva).
		Scan(&docente, &materia, &equipo, &fecha, &horaInicio, &horaFin, &estado)
	if err != nil {
		t.Fatalf("la reserva sembrada no se pudo recuperar entera después de migrar: %v", err)
	}

	comprobar := func(campo, obtenido, esperado string) {
		if obtenido != esperado {
			t.Errorf("la reserva cambió de %s: %q en vez de %q", campo, obtenido, esperado)
		}
	}
	comprobar("docente", docente, "docente@escuela.test")
	comprobar("materia", materia, "Matemática")
	comprobar("equipo", equipo, "SERIE-007")
	comprobar("hora de inicio", horaInicio, "08:00")
	comprobar("hora de fin", horaFin, "09:20")
	comprobar("estado", estado, "CONFIRMADA")
	if esperada := "2026-05-04"; fecha.Format("2006-01-02") != esperada {
		t.Errorf("la reserva cambió de fecha: %q en vez de %q", fecha.Format("2006-01-02"), esperada)
	}
}

// verificarQueLaVersionEsLaUltima cierra el círculo: sin esto, una suite que
// no aplica ninguna migración nueva pasaría igual y el test daría una falsa
// sensación de cobertura.
func verificarQueLaVersionEsLaUltima(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	var version int64
	if err := pool.QueryRow(ctx, `
		SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil {
		t.Fatalf("no se pudo leer goose_db_version: %v", err)
	}

	ultima := ultimaVersionDeLosArchivos(t)
	if version != ultima {
		t.Errorf("la base quedó en la versión %d y el último archivo es el %d", version, ultima)
	}
}

func ejecutar(ctx context.Context, t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("sembrando datos: %v\nSQL: %s", err, sql)
	}
}

// levantarPostgresDeTest arranca un contenedor Postgres efímero y devuelve un
// pool conectado junto al connection string, porque acá el esquema se aplica
// en dos tiempos y goose necesita el dsn.
func levantarPostgresDeTest(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()

	contenedor, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("sgrc_test"),
		postgres.WithUsername("sgrc_test"),
		postgres.WithPassword("sgrc_test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("no se pudo levantar el contenedor de Postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := contenedor.Terminate(context.Background()); err != nil {
			t.Logf("advertencia: no se pudo terminar el contenedor limpiamente: %v", err)
		}
	})

	connStr, err := contenedor.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("no se pudo obtener el connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("no se pudo conectar al pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool, connStr
}
