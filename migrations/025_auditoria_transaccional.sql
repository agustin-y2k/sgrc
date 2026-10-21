-- +goose Up
-- La escritura sensible y su constancia se confirman o revierten juntas.
-- Nombres de columnas y valores de una lista explícita de estados, roles,
-- referencias y fechas. Nunca contraseñas, hashes, ciphertext ni texto libre.
-- +goose StatementBegin
CREATE FUNCTION auditar_cambio_sensible() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    anterior jsonb := '{}'::jsonb;
    actual jsonb := '{}'::jsonb;
    fila jsonb;
    campos jsonb;
    antes_visible jsonb;
    despues_visible jsonb;
    permitidos text[] := ARRAY[
        'estado','rol','archivado','activo','anio','division','tipo','reservable','dado_de_baja',
        'carro_id','materia_id','curso_id','espacio_id','creado_por','cancelado_por',
        'fecha','hora_inicio','hora_fin','aprobado_por','fecha_aprobacion',
        'version_sesion','debe_cambiar_password','devuelto_en','recibido_por',
        'fecha_vencimiento','vencimiento_fijado_por'
    ];
BEGIN
    IF TG_OP <> 'INSERT' THEN anterior := to_jsonb(OLD); END IF;
    IF TG_OP <> 'DELETE' THEN actual := to_jsonb(NEW); END IF;
    IF anterior = actual THEN RETURN NULL; END IF;
    fila := CASE WHEN TG_OP='DELETE' THEN anterior ELSE actual END;
    SELECT coalesce(jsonb_agg(k ORDER BY k),'[]'::jsonb) INTO campos
    FROM (SELECT jsonb_object_keys(anterior || actual) AS k) claves
    WHERE anterior->k IS DISTINCT FROM actual->k;
    SELECT coalesce(jsonb_object_agg(k,anterior->k),'{}'::jsonb) INTO antes_visible
    FROM jsonb_array_elements_text(campos) AS claves(k) WHERE k=ANY(permitidos) AND anterior ? k;
    SELECT coalesce(jsonb_object_agg(k,actual->k),'{}'::jsonb) INTO despues_visible
    FROM jsonb_array_elements_text(campos) AS claves(k) WHERE k=ANY(permitidos) AND actual ? k;
    INSERT INTO audit_log(usuario_id,accion,entidad,entidad_id,detalle,ip_origen)
    VALUES (
        coalesce(nullif(current_setting('sgrc.actor',true),''),'00000000-0000-0000-0000-000000000000')::uuid,
        'CAMBIO_TRANSACCIONAL',TG_TABLE_NAME,
        coalesce(fila->>'id',fila->>'usuario_id',fila->>'ciclo_lectivo_id')::uuid,
        jsonb_build_object('operacion',TG_OP,'campos',campos,'antes',antes_visible,'despues',despues_visible),
        nullif(current_setting('sgrc.ip',true),'')::inet
    );
    RETURN NULL;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
DO $$
DECLARE tabla text;
BEGIN
    FOREACH tabla IN ARRAY ARRAY[
        'usuario','codigo_recuperacion','ciclo_lectivo','curso','espacio','materia','docente_materia',
        'carro','equipo','equipo_cuenta','incidencia','licencia_software',
        'regla_recurrencia','reserva_grupo','reserva','prestamo',
        'jornada_institucion','horario_admin','horario_admin_excepcion','pedido_de_materia'
    ] LOOP
        EXECUTE format('CREATE TRIGGER auditoria_sensible AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION auditar_cambio_sensible()',tabla);
    END LOOP;
END;
$$;
-- +goose StatementEnd
COMMENT ON FUNCTION auditar_cambio_sensible() IS 'Auditoría atómica. Actor UUID cero: sistema/caller sin identidad explícita. Solo incluye valores de la lista permitida; nunca secretos ni texto libre.';

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE tabla text;
BEGIN
    FOREACH tabla IN ARRAY ARRAY[
        'usuario','codigo_recuperacion','ciclo_lectivo','curso','espacio','materia','docente_materia',
        'carro','equipo','equipo_cuenta','incidencia','licencia_software',
        'regla_recurrencia','reserva_grupo','reserva','prestamo',
        'jornada_institucion','horario_admin','horario_admin_excepcion','pedido_de_materia'
    ] LOOP
        EXECUTE format('DROP TRIGGER auditoria_sensible ON %I',tabla);
    END LOOP;
END;
$$;
-- +goose StatementEnd
DROP FUNCTION auditar_cambio_sensible();
