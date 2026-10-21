package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/agustin-y2k/sgrc/internal/auditoria/domain"
	"github.com/google/uuid"
)

func tipoDeReferencia(entidad string) string {
	switch entidad {
	case "pc":
		return "equipo"
	case "licencia":
		return "licencia_software"
	}
	return entidad
}

// resolverNombres añade referencias actuales mediante una sola consulta para
// toda la página. No modifica audit_log ni recupera contraseñas o textos privados.
func (r *PostgresRepo) resolverNombres(ctx context.Context, entradas []domain.Entrada) error {
	porEntrada := make([]map[string]bool, len(entradas))
	ids := make(map[string]bool)
	for i, e := range entradas {
		porEntrada[i] = make(map[string]bool)
		agregar := func(id string) {
			if parsed, err := uuid.Parse(id); err == nil {
				valor := parsed.String()
				ids[valor] = true
				porEntrada[i][valor] = true
			}
		}
		agregar(e.EntidadID)
		var recorrer func(any)
		recorrer = func(v any) {
			switch x := v.(type) {
			case string:
				agregar(x)
			case map[string]any:
				for _, valor := range x {
					recorrer(valor)
				}
			case []any:
				for _, valor := range x {
					recorrer(valor)
				}
			}
		}
		var detalle any
		if json.Unmarshal(e.Detalle, &detalle) == nil {
			recorrer(detalle)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	lista := make([]string, 0, len(ids))
	for id := range ids {
		lista = append(lista, id)
	}
	sort.Strings(lista)
	rows, err := r.pool.Query(ctx, `
		WITH ids AS (SELECT unnest($1::uuid[]) AS id),
		equipos_con_nombre AS (
			SELECT e.id, COALESCE(e.nombre,'PC ' || e.identificador) || COALESCE(' del ' || c.nombre,'') AS nombre
			FROM equipo e LEFT JOIN carro c ON c.id=e.carro_id
		)
		SELECT 'usuario',u.id::text,u.nombre || ' ' || u.apellido FROM usuario u JOIN ids ON ids.id=u.id
		UNION ALL SELECT 'carro',c.id::text,c.nombre FROM carro c JOIN ids ON ids.id=c.id
		UNION ALL SELECT 'equipo',e.id::text,e.nombre FROM equipos_con_nombre e JOIN ids ON ids.id=e.id
		UNION ALL SELECT 'ciclo_lectivo',c.id::text,'Ciclo ' || c.anio FROM ciclo_lectivo c JOIN ids ON ids.id=c.id
		UNION ALL SELECT 'curso',c.id::text,c.nombre || COALESCE(' · ' || NULLIF(c.modalidad,''),'') FROM curso c JOIN ids ON ids.id=c.id
		UNION ALL SELECT 'espacio',e.id::text,e.nombre FROM espacio e JOIN ids ON ids.id=e.id
		UNION ALL SELECT 'materia',m.id::text,m.nombre || ' · ' || COALESCE(c.nombre,es.nombre,'')
			FROM materia m JOIN ids ON ids.id=m.id LEFT JOIN curso c ON c.id=m.curso_id LEFT JOIN espacio es ON es.id=m.espacio_id
		UNION ALL SELECT 'equipo_cuenta',cu.id::text,cu.clase || ' · ' || cu.usuario || ' · ' || e.nombre
			FROM equipo_cuenta cu JOIN ids ON ids.id=cu.id JOIN equipos_con_nombre e ON e.id=cu.equipo_id
		UNION ALL SELECT 'licencia_software',l.id::text,l.nombre || ' · ' || e.nombre
			FROM licencia_software l JOIN ids ON ids.id=l.id JOIN equipos_con_nombre e ON e.id=l.equipo_id
		UNION ALL SELECT 'docente_materia',d.id::text,u.nombre || ' ' || u.apellido || ' · ' || m.nombre
			FROM docente_materia d JOIN ids ON ids.id=d.id JOIN usuario u ON u.id=d.usuario_id JOIN materia m ON m.id=d.materia_id
		UNION ALL SELECT 'reserva',r.id::text,COALESCE(m.nombre,'Bloqueo institucional') || ' · ' || to_char(r.fecha,'DD/MM/YYYY') || ' · ' || e.nombre
			FROM reserva r JOIN ids ON ids.id=r.id JOIN equipos_con_nombre e ON e.id=r.equipo_id LEFT JOIN materia m ON m.id=r.materia_id
		UNION ALL SELECT 'reserva_grupo',g.id::text,m.nombre || ' · ' || to_char(g.fecha,'DD/MM/YYYY')
			FROM reserva_grupo g JOIN ids ON ids.id=g.id JOIN materia m ON m.id=g.materia_id
		UNION ALL SELECT 'regla_recurrencia',r.id::text,m.nombre || ' · serie desde ' || to_char(r.fecha_inicio,'DD/MM/YYYY')
			FROM regla_recurrencia r JOIN ids ON ids.id=r.id JOIN materia m ON m.id=r.materia_id
		UNION ALL SELECT 'prestamo',p.id::text,e.nombre || ' · ' || p.entregado_a_nombre
			FROM prestamo p JOIN ids ON ids.id=p.id JOIN equipos_con_nombre e ON e.id=p.equipo_id
		UNION ALL SELECT 'equipo_preferencia',p.id::text,p.materia_nombre || ' · ' || e.nombre
			FROM equipo_preferencia p JOIN ids ON ids.id=p.id JOIN equipos_con_nombre e ON e.id=p.equipo_id
		UNION ALL SELECT 'incidencia',i.id::text,'Incidencia de ' || e.nombre
			FROM incidencia i JOIN ids ON ids.id=i.id JOIN equipos_con_nombre e ON e.id=i.equipo_id
		UNION ALL SELECT 'pedido_de_materia',p.id::text,'Pedido de ' || COALESCE(m.nombre,p.materia_solicitada,'materia')
			FROM pedido_de_materia p JOIN ids ON ids.id=p.id LEFT JOIN materia m ON m.id=p.materia_id
	`, lista)
	if err != nil {
		return fmt.Errorf("resolviendo nombres de auditoría: %w", err)
	}
	defer rows.Close()
	nombres := make(map[string]string)
	for rows.Next() {
		var tipo, id, nombre string
		if err := rows.Scan(&tipo, &id, &nombre); err != nil {
			return fmt.Errorf("leyendo nombres de auditoría: %w", err)
		}
		nombres[tipo+":"+id] = nombre
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range entradas {
		e := &entradas[i]
		e.EntidadNombre = nombres[tipoDeReferencia(e.Entidad)+":"+e.EntidadID]
		for clave, nombre := range nombres {
			_, id, _ := strings.Cut(clave, ":")
			if porEntrada[i][id] {
				if e.Referencias == nil {
					e.Referencias = make(map[string]string)
				}
				e.Referencias[clave] = nombre
			}
		}
	}
	return nil
}
