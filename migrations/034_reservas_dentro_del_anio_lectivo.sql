-- +goose Up
-- Se protege toda nueva planificación. El histórico existente no se reasigna
-- ni se cancela al instalar la regla, y sigue admitiendo cierre y cancelación.
-- +goose StatementBegin
CREATE FUNCTION verificar_anio_lectivo_de_reserva() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE anio_ciclo integer; desde date; hasta date; ultima date; dia_elegido integer; termino timestamp;
BEGIN
    IF NEW.materia_id IS NULL THEN RETURN NEW; END IF;
    SELECT cl.anio INTO anio_ciclo FROM contenedor_de_materia cm
      JOIN ciclo_lectivo cl ON cl.id=cm.ciclo_lectivo_id WHERE cm.materia_id=NEW.materia_id;
    IF anio_ciclo IS NULL THEN RETURN NEW; END IF;
    IF TG_TABLE_NAME='regla_recurrencia' THEN
        desde:=NEW.fecha_inicio; hasta:=NEW.fecha_fin;
        dia_elegido:=CASE NEW.dia_semana WHEN 'DOMINGO' THEN 0 WHEN 'LUNES' THEN 1 WHEN 'MARTES' THEN 2 WHEN 'MIERCOLES' THEN 3 WHEN 'JUEVES' THEN 4 WHEN 'VIERNES' THEN 5 WHEN 'SABADO' THEN 6 END;
        ultima:=hasta-((EXTRACT(DOW FROM hasta)::integer-dia_elegido+7)%7);
        IF ultima<desde THEN ultima:=NULL; END IF;
    ELSE
        desde:=NEW.fecha; hasta:=NEW.fecha; ultima:=NEW.fecha;
    END IF;
    termino:=ultima+NEW.hora_fin;
    IF NEW.hora_fin<NEW.hora_inicio THEN termino:=termino+interval '1 day'; END IF;
    IF desde<make_date(anio_ciclo,1,1) OR hasta>=make_date(anio_ciclo+1,1,1)
       OR termino>make_date(anio_ciclo+1,1,1) THEN
        RAISE EXCEPTION 'La reserva debe quedar dentro del año del ciclo lectivo %',anio_ciclo
          USING ERRCODE='23514',CONSTRAINT='reserva_anio_lectivo';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER reserva_anio_lectivo BEFORE INSERT OR UPDATE OF materia_id,fecha,hora_inicio,hora_fin ON reserva
FOR EACH ROW EXECUTE FUNCTION verificar_anio_lectivo_de_reserva();
CREATE TRIGGER reserva_grupo_anio_lectivo BEFORE INSERT OR UPDATE OF materia_id,fecha,hora_inicio,hora_fin ON reserva_grupo
FOR EACH ROW EXECUTE FUNCTION verificar_anio_lectivo_de_reserva();
CREATE TRIGGER regla_recurrencia_anio_lectivo BEFORE INSERT OR UPDATE OF materia_id,fecha_inicio,fecha_fin,hora_inicio,hora_fin,dia_semana ON regla_recurrencia
FOR EACH ROW EXECUTE FUNCTION verificar_anio_lectivo_de_reserva();

-- +goose Down
DROP TRIGGER regla_recurrencia_anio_lectivo ON regla_recurrencia;
DROP TRIGGER reserva_grupo_anio_lectivo ON reserva_grupo;
DROP TRIGGER reserva_anio_lectivo ON reserva;
DROP FUNCTION verificar_anio_lectivo_de_reserva();
