package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramiro/sgrc/internal/inventory/application"
	"github.com/ramiro/sgrc/internal/inventory/domain"
)

const columnasLicencia = `id, equipo_id, nombre, dias_duracion, dias_aviso, fecha_vencimiento, ` +
	`ultima_renovacion, vencimiento_fijado_por, vencimiento_fijado_en, ` +
	`avisado_previo_para, avisado_vencimiento_para, creada_en`

// columnasLicenciaConUbicacion agrega cómo se llama el equipo y dónde está.
// Van prefijadas con l./p./c. porque la consulta hace JOIN.
//
// Los COALESCE están porque un equipo suelto con software licenciado no
// tiene carro ni identificador, y sin ellos el escaneo a string/int revienta.
const columnasLicenciaConUbicacion = `l.id, l.equipo_id, l.nombre, l.dias_duracion, l.dias_aviso, l.fecha_vencimiento, ` +
	`l.ultima_renovacion, l.vencimiento_fijado_por, l.vencimiento_fijado_en, ` +
	`l.avisado_previo_para, l.avisado_vencimiento_para, l.creada_en, ` +
	`COALESCE(p.nombre, 'PC ' || p.identificador), COALESCE(p.identificador, 0), ` +
	`p.dado_de_baja, COALESCE(c.id::text, ''), COALESCE(c.nombre, '')`

func (r *PostgresRepo) CrearLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO licencia_software (
			id, equipo_id, nombre, dias_duracion, dias_aviso, fecha_vencimiento,
			ultima_renovacion, vencimiento_fijado_por, vencimiento_fijado_en,
			avisado_previo_para, avisado_vencimiento_para, creada_en
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, l.ID, l.EquipoID, l.Nombre, l.DiasDuracion, l.DiasAviso, l.FechaVencimiento,
		l.UltimaRenovacion, l.VencimientoFijadoPor, l.VencimientoFijadoEn,
		l.AvisadoPrevioPara, l.AvisadoVencimientoPara, l.CreadaEn)
	if err != nil {
		// Acá el UNIQUE es uno solo (equipo_id + lower(nombre)), así que a
		// diferencia de CrearEquipo no hay ambigüedad sobre cuál se violó.
		if esViolacionUnica(err) {
			return application.ErrLicenciaDuplicada
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("creando licencia: %w", err)
	}
	return nil
}

func (r *PostgresRepo) BuscarLicenciaPorID(ctx context.Context, id string) (*domain.LicenciaSoftware, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+columnasLicencia+` FROM licencia_software WHERE id = $1`, id)
	return escanearLicencia(row)
}

func escanearLicencia(row pgx.Row) (*domain.LicenciaSoftware, error) {
	var l domain.LicenciaSoftware
	err := row.Scan(
		&l.ID, &l.EquipoID, &l.Nombre, &l.DiasDuracion, &l.DiasAviso, &l.FechaVencimiento,
		&l.UltimaRenovacion, &l.VencimientoFijadoPor, &l.VencimientoFijadoEn,
		&l.AvisadoPrevioPara, &l.AvisadoVencimientoPara, &l.CreadaEn,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrLicenciaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando licencia: %w", err)
	}
	return &l, nil
}

func escanearLicenciaConUbicacion(row pgx.Row) (*application.LicenciaConUbicacion, error) {
	var l domain.LicenciaSoftware
	var u application.LicenciaConUbicacion

	err := row.Scan(
		&l.ID, &l.EquipoID, &l.Nombre, &l.DiasDuracion, &l.DiasAviso, &l.FechaVencimiento,
		&l.UltimaRenovacion, &l.VencimientoFijadoPor, &l.VencimientoFijadoEn,
		&l.AvisadoPrevioPara, &l.AvisadoVencimientoPara, &l.CreadaEn,
		&u.Etiqueta, &u.Identificador, &u.EquipoDadoDeBaja, &u.CarroID, &u.CarroNombre,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrLicenciaNoEncontrada
		}
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("escaneando licencia con ubicación: %w", err)
	}
	u.Licencia = &l
	return &u, nil
}

func (r *PostgresRepo) GuardarLicencia(ctx context.Context, l *domain.LicenciaSoftware) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE licencia_software SET
			nombre=$2, dias_duracion=$3, dias_aviso=$4, fecha_vencimiento=$5,
			ultima_renovacion=$6, vencimiento_fijado_por=$7, vencimiento_fijado_en=$8,
			avisado_previo_para=$9, avisado_vencimiento_para=$10
		WHERE id=$1
	`, l.ID, l.Nombre, l.DiasDuracion, l.DiasAviso, l.FechaVencimiento,
		l.UltimaRenovacion, l.VencimientoFijadoPor, l.VencimientoFijadoEn,
		l.AvisadoPrevioPara, l.AvisadoVencimientoPara)
	if err != nil {
		if esViolacionUnica(err) {
			return application.ErrLicenciaDuplicada
		}
		if esViolacionFK(err) {
			return application.ErrReferenciaInexistente
		}
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("actualizando licencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrLicenciaNoEncontrada
	}
	return nil
}

// MarcarAvisosEnviados toca SOLO las dos marcas.
//
// No es un GuardarLicencia con otro nombre: entre que el job lee la
// licencia y termina de hablar con el servidor de correo pueden pasar
// decenas de segundos, y en ese rato un Admin puede haberla renovado desde
// la pantalla. Con un UPDATE completo, el job escribiría de vuelta el
// vencimiento viejo que tenía en memoria y desharía la renovación.
func (r *PostgresRepo) MarcarAvisosEnviados(ctx context.Context, l *domain.LicenciaSoftware) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE licencia_software SET
			avisado_previo_para=$2, avisado_vencimiento_para=$3
		WHERE id=$1
	`, l.ID, l.AvisadoPrevioPara, l.AvisadoVencimientoPara)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("marcando avisos de licencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrLicenciaNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) BorrarLicencia(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM licencia_software WHERE id = $1`, id)
	if err != nil {
		if esIDInvalido(err) {
			return application.ErrIDInvalido
		}
		return fmt.Errorf("borrando licencia: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return application.ErrLicenciaNoEncontrada
	}
	return nil
}

func (r *PostgresRepo) ListarLicenciasPorEquipo(ctx context.Context, equipoID string) ([]*domain.LicenciaSoftware, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+columnasLicencia+` FROM licencia_software WHERE equipo_id = $1 ORDER BY nombre`, equipoID)
	if err != nil {
		if esIDInvalido(err) {
			return nil, application.ErrIDInvalido
		}
		return nil, fmt.Errorf("listando licencias del equipo: %w", err)
	}
	defer rows.Close()

	var resultado []*domain.LicenciaSoftware
	for rows.Next() {
		l, err := escanearLicencia(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de licencia: %w", err)
		}
		resultado = append(resultado, l)
	}
	return resultado, errorDeFilas(rows)
}

// ordenDeLaPantalla pone primero lo que hay que resolver.
//
// Las que no tienen fecha van arriba de todo: son las que están esperando
// que alguien se siente delante de la máquina, y son las únicas que no se
// pueden ordenar por días restantes porque no tienen ninguno. Después, de
// la más vencida a la que más le falta. El desempate por carro y PC es para
// que dos licencias que vencen el mismo día salgan siempre en el mismo
// orden — sin él, Postgres puede devolverlas alternadas entre corridas y la
// tabla parece moverse sola.
// Los COALESCE del orden hacen falta porque con c.nombre y p.identificador
// en NULL los equipos sueltos empatarían todos entre sí y la lista se
// movería sola entre corridas. Ordenados por nombre quedan juntos y estables.
const ordenDeLaPantalla = `ORDER BY l.fecha_vencimiento IS NOT NULL, l.fecha_vencimiento, ` +
	`COALESCE(c.nombre, ''), COALESCE(p.identificador, 0), COALESCE(p.nombre, ''), l.nombre`

func (r *PostgresRepo) ListarLicencias(ctx context.Context) ([]*application.LicenciaConUbicacion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasLicenciaConUbicacion+`
		FROM licencia_software l
		JOIN equipo p ON p.id = l.equipo_id
		-- LEFT: un equipo suelto puede tener software licenciado igual que los
		-- del carro, y con INNER su licencia no llega a la pantalla — se vence
		-- sin que nadie la vea.
		LEFT JOIN carro c ON c.id = p.carro_id
	`+ordenDeLaPantalla)
	if err != nil {
		return nil, fmt.Errorf("listando licencias: %w", err)
	}
	return escanearFilasConUbicacion(rows)
}

// ListarCandidatasAAviso es el filtro grueso del job. La regla fina la
// aplica el dominio (ver el comentario del puerto).
//
// `l.fecha_vencimiento - l.dias_aviso <= $1` es aritmética de DATE:
// en Postgres, date menos integer da date. Cubre de una las dos situaciones
// —ya entró en la ventana de antelación, o ya venció— sin repetir la
// condición.
//
// La comparación es date CONTRA date, sin ningún timestamp de por medio, y
// eso no es casual: Postgres infiere el tipo de `$1` como `date` por el
// operando izquierdo, así que pgx codifica el time.Time usando solo su
// año/mes/día y la zona de la SESIÓN nunca entra en juego.
//
// Importa porque si entrara, el borde se correría un día —con la sesión en
// -03:00 el 2026-08-07 pasaría a ser 2026-08-07 03:00 UTC y dejaría de ser
// <= las 00:00 de ese mismo día— y la licencia que vence mañana se caería
// del resultado sin ningún error: el aviso no saldría nunca. Ese es el peor
// modo de falla posible acá, así que hay un test que lo fija con la sesión
// en tres zonas distintas
// (TestPostgresRepo_Licencia_CandidatasNoDependenDeLaZonaDeLaSesion).
//
// El filtro de marcas descarta las que ya avisaron todo lo que tenían que
// avisar para este vencimiento. IS DISTINCT FROM y no <>: con NULL, el <>
// devuelve NULL y la fila se pierde, que es exactamente al revés de lo que
// hace falta (marca vacía = todavía no se avisó).
//
// Las PCs dadas de baja quedan afuera: nadie las va a usar, así que
// renovarles la licencia no le sirve a nadie. Las FUERA_DE_SERVICIO sí
// entran — son recuperables editándolas y la licencia les sigue corriendo.
func (r *PostgresRepo) ListarCandidatasAAviso(ctx context.Context, hoy time.Time) ([]*application.LicenciaConUbicacion, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+columnasLicenciaConUbicacion+`
		FROM licencia_software l
		JOIN equipo p ON p.id = l.equipo_id
		-- LEFT por lo mismo que en ListarLicencias, y acá es peor: con INNER
		-- JOIN la licencia de un equipo suelto no era candidata a aviso
		-- NUNCA, así que el correo no salía y nadie se enteraba.
		LEFT JOIN carro c ON c.id = p.carro_id
		WHERE l.fecha_vencimiento IS NOT NULL
		  AND p.dado_de_baja = false
		  AND l.fecha_vencimiento - l.dias_aviso <= $1
		  AND (l.avisado_previo_para IS DISTINCT FROM l.fecha_vencimiento
		       OR l.avisado_vencimiento_para IS DISTINCT FROM l.fecha_vencimiento)
	`+ordenDeLaPantalla, domain.Dia(hoy))
	if err != nil {
		return nil, fmt.Errorf("listando licencias candidatas a aviso: %w", err)
	}
	return escanearFilasConUbicacion(rows)
}

func escanearFilasConUbicacion(rows pgx.Rows) ([]*application.LicenciaConUbicacion, error) {
	defer rows.Close()

	var resultado []*application.LicenciaConUbicacion
	for rows.Next() {
		u, err := escanearLicenciaConUbicacion(rows)
		if err != nil {
			return nil, fmt.Errorf("escaneando fila de licencia: %w", err)
		}
		resultado = append(resultado, u)
	}
	return resultado, errorDeFilas(rows)
}
