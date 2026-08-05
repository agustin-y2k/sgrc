-- SGRC — Correcciones de esquema detectadas en la revisión previa al 1.0.0
--
-- Va en un archivo aparte y no editando 001_init.sql para que sirva en los
-- dos escenarios: una base nueva (docker-entrypoint-initdb.d corre los
-- archivos en orden) y una base ya cargada, donde se aplica a mano.

-- ══════════════════════════════════════════════════════════════════
-- 1. FKs a usuario que impedían el hard delete de RF-01.9
-- ══════════════════════════════════════════════════════════════════
-- RF-01.9 permite eliminar definitivamente una cuenta en BAJA para liberar
-- el email, y dice que lo asociado a esa cuenta "no se elimina, solo pierde
-- la referencia al usuario". Estas dos FKs no tenían política de borrado
-- (NO ACTION), así que el DELETE fallaba con violación de FK y el handler
-- lo devolvía como 500. Eran el caso común, no el raro: cualquier docente
-- que hubiera creado una reserva recurrente, o que figurara en el snapshot
-- histórico de un ciclo archivado, quedaba imposible de eliminar y su email
-- bloqueado para siempre.

-- regla_recurrencia.creado_por era además NOT NULL. Se relaja a nullable
-- para poder aplicar SET NULL, igual que reserva.creado_por y
-- reserva_grupo.creado_por, que ya funcionaban así.
ALTER TABLE regla_recurrencia ALTER COLUMN creado_por DROP NOT NULL;

ALTER TABLE regla_recurrencia DROP CONSTRAINT regla_recurrencia_creado_por_fkey;
ALTER TABLE regla_recurrencia ADD CONSTRAINT regla_recurrencia_creado_por_fkey
    FOREIGN KEY (creado_por) REFERENCES usuario(id) ON DELETE SET NULL;

-- historico_uso_docente ya conserva nombre_docente_snapshot, así que la fila
-- sigue siendo legible sin el usuario.
ALTER TABLE historico_uso_docente DROP CONSTRAINT historico_uso_docente_usuario_id_fkey;
ALTER TABLE historico_uso_docente ADD CONSTRAINT historico_uso_docente_usuario_id_fkey
    FOREIGN KEY (usuario_id) REFERENCES usuario(id) ON DELETE SET NULL;

-- ══════════════════════════════════════════════════════════════════
-- 2. regla_recurrencia_pc: tabla muerta
-- ══════════════════════════════════════════════════════════════════
-- Solo tenía un INSERT y ninguna lectura. La cancelación de ocurrencias
-- futuras (RF-04.6) resuelve por reserva_grupo.regla_recurrencia_id, no por
-- esta tabla, así que no aportaba nada y confundía al leer el modelo.
DROP TABLE IF EXISTS regla_recurrencia_pc;

-- ══════════════════════════════════════════════════════════════════
-- 3. Cascada del archivado hacia regla_recurrencia
-- ══════════════════════════════════════════════════════════════════
-- RF-02.4 exige borrar físicamente ReservaGrupo, Reserva Y ReglaRecurrencia
-- al archivar un ciclo. reserva.reserva_grupo_id ya tenía ON DELETE CASCADE;
-- reserva_grupo.regla_recurrencia_id no, así que borrar una regla fallaba
-- mientras existieran sus grupos. Con SET NULL, el borrado del ciclo puede
-- eliminar grupos y reglas en cualquier orden sin pelearse con la FK.
ALTER TABLE reserva_grupo DROP CONSTRAINT reserva_grupo_regla_recurrencia_id_fkey;
ALTER TABLE reserva_grupo ADD CONSTRAINT reserva_grupo_regla_recurrencia_id_fkey
    FOREIGN KEY (regla_recurrencia_id) REFERENCES regla_recurrencia(id) ON DELETE SET NULL;

-- ══════════════════════════════════════════════════════════════════
-- 4. Limpieza de datos preexistentes
-- ══════════════════════════════════════════════════════════════════
-- Reglas que quedaron huérfanas por el bug de RF-02.4: su ciclo ya fue
-- archivado (la materia quedó archivada) pero la regla nunca se borró.
DELETE FROM regla_recurrencia r
WHERE EXISTS (
    SELECT 1 FROM materia m
    JOIN curso c ON c.id = m.curso_id
    JOIN ciclo_lectivo cl ON cl.id = c.ciclo_lectivo_id
    WHERE m.id = r.materia_id AND cl.archivado = true
);

-- Reservas en sábado o domingo: la semana lectiva es de lunes a viernes y
-- ahora el dominio lo valida, así que no deberían existir. Las Reserva
-- hijas se van solas por el ON DELETE CASCADE de reserva.reserva_grupo_id
-- (una Reserva NORMAL siempre tiene grupo, lo garantiza
-- chk_reserva_tipo_coherente). Los bloqueos de evaluación quedan afuera a
-- propósito: RF-04.7 no los limita a días lectivos.
-- EXTRACT(DOW): 0 = domingo, 6 = sábado.
DELETE FROM reserva_grupo
WHERE EXTRACT(DOW FROM fecha) IN (0, 6);

-- Reglas de recurrencia en sábado: se eliminan en vez de reasignarlas a otro
-- día. Mover una reserva de sábado a viernes en silencio sería inventar una
-- decisión que le corresponde a quien la creó, y sus ocurrencias ya se
-- borraron en el DELETE de arriba, así que la regla no describe nada.
DELETE FROM regla_recurrencia WHERE dia_semana = 'SABADO';
