-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — La misma materia no entra dos veces en el mismo curso
-- ═══════════════════════════════════════════════════════════════════════
--
-- `materia` nació con `UNIQUE (curso_id, nombre)`, que compara el texto EXACTO.
-- Alcanza contra el doble clic y no alcanza contra nada más: en el mismo curso
-- conviven «Matemática», «Matematica», «MATEMATICA» y «matematica» como cuatro
-- materias distintas, porque para ese único lo son.
--
-- No es una molestia estética. Una materia es la unidad sobre la que se
-- reserva (RF-04.1) y a la que se asigna un docente (RF-02.6), así que dos
-- filas para la misma materia parten sus reservas y sus docentes en dos
-- mitades que nadie ve juntas: el docente elige una de las dos en el selector
-- —idénticas en pantalla— y el reporte de uso cuenta cada mitad por separado.
--
-- El caso se volvió frecuente con la carga masiva de RF-02.12: una planilla
-- escrita a mano trae la misma materia con y sin acento en dos renglones sin
-- que nadie lo note. La carga masiva lo resolvía de su lado, con su propia
-- comparación, pero eso dejaba el agujero abierto en el camino de a uno, que es
-- el que se usa todos los días — y una regla que vale en una puerta y no en la
-- otra no es una regla. De acá en adelante la sostiene la base, y las dos
-- puertas preguntan lo mismo porque preguntan con la misma función.
--
-- ── La forma: el mismo patrón que usó la 009 para `curso` ──────────────
--
-- La unicidad pasa a una expresión que hace tres cosas: baja a minúsculas,
-- saca las tildes y **colapsa los espacios**. El único viejo se ELIMINA en vez
-- de convivir: el nuevo es estrictamente más fuerte —dos nombres iguales dan la
-- misma clave—, así que dejarlo sería un índice redundante que sólo cuesta
-- escrituras y confunde a quien lea el esquema.
--
-- **Por qué una expresión y no la columna generada `nombre_norm`.** Esa columna
-- existe y hace las dos primeras cosas, pero no la tercera, y el espacio de más
-- es el caso PEOR de los tres: «Educación Física» y «Educación  Física» se
-- imprimen idénticas en todas las pantallas del sistema, así que si entran las
-- dos nadie puede ver por qué la materia aparece duplicada ni cuál borrar. Con
-- las tildes al menos se ve la diferencia.
--
-- `nombre_norm` se queda como está —es lo que cruza las marcas de preferencia
-- de equipo (RF-03.21) y no debe cambiar por esto—; el índice no la usa.
--
-- Las dos funciones son IMMUTABLE, que es lo que permite indexarlas.
--
-- ── Qué pasa con los duplicados que ya están cargados ──────────────────
--
-- Se RENOMBRAN, no se borran. Una materia duplicada puede tener reservas,
-- docentes asignados y pedidos colgando, y una migración que las borre se
-- lleva puesto todo eso en silencio — justo lo que no puede hacer el paso que
-- viene a proteger la integridad de esa tabla.
--
-- Antes de desambiguar se limpian los espacios de más («Educación  Física» →
-- «Educación Física»), porque un espacio doble no es un dato sino un accidente,
-- y dejarlo sin limpiar hace que el desempate lo trate como un nombre más.
--
-- De lo que siga repitiéndose, una de cada grupo conserva su nombre intacto y a
-- las demás se les agrega « (2)», « (3)»…: ninguna fila se pierde, ninguna referencia se rompe, y el
-- duplicado queda VISIBLE en la pantalla del curso para que un Admin decida a
-- mano cuál sobrevive. Renombrar es reversible; borrar no.
--
-- **Cuál conserva el nombre limpio: la MÁS USADA.** No es un desempate
-- cosmético. El nombre de una materia aparece en las reservas que ya están
-- hechas y en la lista de lo que dicta cada docente, así que renombrar la fila
-- que todo el mundo usa —para dejarle el nombre bueno a una copia huérfana que
-- nadie miró nunca— le cambia el nombre a la materia de verdad y le da el
-- limpio a la basura. Se cuentan docentes asignados, reservas y reglas de
-- recurrencia; recién si empatan desempata el texto y el id, que es arbitrario
-- pero estable.
--
-- El `left(..., 94)` deja lugar para el sufijo sin pasarse de VARCHAR(100).

-- +goose Up

-- clave_materia es la forma en que se comparan dos nombres de materia para
-- decidir si son EL MISMO. Vive en una función y no repetida en cada sentencia
-- porque la usan tres lugares —el dedup de acá abajo, el índice, y la consulta
-- de la carga masiva en Go—, y el día que las tres dejen de coincidir el
-- síntoma es una carga que revienta contra el índice en vez de saltear la fila.
--
-- IMMUTABLE es obligatorio para poder indexarla; las dos funciones que usa
-- adentro lo son. STRICT devuelve NULL con entrada NULL, que acá no pasa
-- (`nombre` es NOT NULL) pero deja la función correcta si alguien la reusa.
-- +goose StatementBegin
CREATE FUNCTION clave_materia(nombre text) RETURNS text
    LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
    RETURN regexp_replace(
               btrim(translate(lower(nombre), 'áéíóúüñ', 'aeiouun')),
               '\s+', ' ', 'g');
-- +goose StatementEnd

-- El único viejo se va PRIMERO, antes de tocar los nombres: el paso que sigue
-- canoniza los espacios, y eso convierte «Educación  Física» en el mismo texto
-- exacto que «Educación Física» — que es justo lo que ese único prohíbe.
ALTER TABLE materia DROP CONSTRAINT materia_curso_id_nombre_key;

-- Paso 1: limpiar los espacios de más. Va ANTES de desambiguar, y ése es el
-- orden que importa.
--
-- Un espacio doble no es un dato: es un accidente de una celda de planilla o de
-- un copiado. Si no se limpia acá, el desempate del paso 2 lo trata como un
-- nombre más y puede dejar a la fila MALFORMADA con el nombre limpio y ponerle
-- el sufijo a la correcta — que es exactamente al revés de lo que hay que
-- hacer. Limpiarlo primero borra la diferencia y deja que el desempate mire lo
-- único que importa: cuál está en uso.
--
-- No toca mayúsculas ni tildes: eso sí es cómo eligió escribirlo la institución.
UPDATE materia
   SET nombre = btrim(regexp_replace(nombre, '\s+', ' ', 'g'))
 WHERE nombre <> btrim(regexp_replace(nombre, '\s+', ' ', 'g'));

-- Paso 2: desambiguar lo que sigue repitiéndose. Antes de crear el índice, o
-- falla y la migración entera se va al rollback.
UPDATE materia m
   SET nombre = left(d.nombre, 94) || ' (' || d.n || ')'
  FROM (
        SELECT id,
               nombre,
               row_number() OVER (
                   PARTITION BY curso_id, clave_materia(nombre)
                   -- La más usada primero: es la que conserva el nombre limpio.
                   -- Por fecha no se podría desempatar aunque se quisiera —
                   -- `materia` no guarda ninguna—, y "cuál se cargó primero" no
                   -- es la pregunta correcta: importa cuál está en uso.
                   ORDER BY usos DESC, nombre, id
               ) AS n
          FROM (
                SELECT mm.id, mm.nombre, mm.curso_id,
                       (SELECT count(*) FROM docente_materia dm WHERE dm.materia_id = mm.id)
                     + (SELECT count(*) FROM reserva_grupo rg WHERE rg.materia_id = mm.id)
                     + (SELECT count(*) FROM regla_recurrencia rr WHERE rr.materia_id = mm.id)
                       AS usos
                  FROM materia mm
               ) AS conteo
       ) AS d
 WHERE m.id = d.id
   AND d.n > 1;

-- Paso 3: y recién ahora la regla, que de acá en adelante la sostiene la base.
CREATE UNIQUE INDEX ux_materia_curso_nombre ON materia (curso_id, clave_materia(nombre));

-- `idx_materia_nombre_norm` sigue existiendo y NO es redundante con este: el
-- índice único es compuesto, arranca por `curso_id` y va sobre otra expresión,
-- así que no sirve para la búsqueda por nombre suelto que hace el cruce de
-- preferencias (RF-03.21).

COMMENT ON INDEX ux_materia_curso_nombre IS
    'Una materia por curso, comparando sin tildes, sin mayúsculas y con los '
    'espacios colapsados. Reemplaza al UNIQUE por nombre exacto, que dejaba '
    'entrar «Matematica» al lado de «Matemática» y «Educación  Física» al lado '
    'de «Educación Física», partiendo en dos las reservas y los docentes de esa '
    'materia.';

-- +goose Down

DROP INDEX ux_materia_curso_nombre;
DROP FUNCTION clave_materia(text);

-- El nombre de la constraint se escribe explícito para que vuelva a llamarse
-- como la creó la 001: el que Postgres genera solo es este mismo, pero
-- depender de eso deja la vuelta atrás a merced de una convención.
ALTER TABLE materia ADD CONSTRAINT materia_curso_id_nombre_key UNIQUE (curso_id, nombre);

-- Los « (2)» no se deshacen: quitarlos volvería a chocar contra el UNIQUE que
-- esta misma sentencia acaba de reponer. El Down recupera el ESQUEMA anterior,
-- no los nombres que la subida tuvo que desambiguar.
