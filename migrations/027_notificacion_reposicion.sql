-- +goose Up
ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_tipo;
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_tipo CHECK (tipo IN (
    'GENERAL', 'DOCENTE_PENDIENTE', 'RESERVA_CANCELADA', 'RESERVA_REUBICADA',
    'LICENCIA_POR_VENCER', 'EQUIPO_SIN_DEVOLVER', 'PEDIDO_DE_LIBERACION',
    'PEDIDO_DE_MATERIA', 'PEDIDO_DE_MATERIA_RESUELTO', 'SUGERENCIA', 'SUGERENCIA_RESPONDIDA'
));

-- Los reemplazos conservan el ID de reserva: la auditoría registra ambos equipos.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION auditar_cambio_sensible() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    anterior jsonb := '{}'::jsonb;
    actual jsonb := '{}'::jsonb;
    fila jsonb;
    campos jsonb;
    antes_visible jsonb;
    despues_visible jsonb;
    permitidos text[] := ARRAY[
        'estado','rol','archivado','activo','anio','division','tipo','reservable','dado_de_baja',
        'equipo_id','carro_id','materia_id','curso_id','espacio_id','creado_por','cancelado_por',
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

-- +goose Down
UPDATE notificacion SET tipo='GENERAL' WHERE tipo='RESERVA_REUBICADA';
ALTER TABLE notificacion DROP CONSTRAINT chk_notificacion_tipo;
ALTER TABLE notificacion ADD CONSTRAINT chk_notificacion_tipo CHECK (tipo IN (
    'GENERAL', 'DOCENTE_PENDIENTE', 'RESERVA_CANCELADA',
    'LICENCIA_POR_VENCER', 'EQUIPO_SIN_DEVOLVER', 'PEDIDO_DE_LIBERACION',
    'PEDIDO_DE_MATERIA', 'PEDIDO_DE_MATERIA_RESUELTO', 'SUGERENCIA', 'SUGERENCIA_RESPONDIDA'
));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION auditar_cambio_sensible() RETURNS trigger LANGUAGE plpgsql AS $$
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
