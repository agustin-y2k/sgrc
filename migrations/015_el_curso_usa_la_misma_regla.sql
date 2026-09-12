-- ═══════════════════════════════════════════════════════════════════════
-- SGRC — El curso pasa a compararse con la misma regla que todo lo demás
-- ═══════════════════════════════════════════════════════════════════════
--
-- RF-00.1 dice que hay UNA regla para decidir si dos textos nombran la misma
-- cosa, y `clave_texto()` de la 011 la implementa: sin tildes, sin mayúsculas,
-- sin espacios de más. Después de esa migración la sostenían cinco índices —el
-- carro, el equipo suelto, el número de serie, la materia y la licencia—.
--
-- El curso quedó afuera. Su único siguió apoyado en las columnas generadas
-- `division_norm` y `modalidad_norm` que trajo la 009, que sólo bajan a
-- minúsculas y sacan tildes: no recortan los bordes ni colapsan los espacios
-- repetidos. Es decir, el requisito decía «una regla» y había dos.
--
-- ── Por qué no alcanzaba con que nadie lo notara ───────────────────────
--
-- El síntoma no es un duplicado —la validación de Go canoniza antes de
-- guardar, así que por la aplicación no entra—. El síntoma es que la carga
-- masiva de RF-02.12 tiene que REPLICAR la expresión de las columnas
-- generadas para buscar un curso existente, mientras que para buscar una
-- materia llama a `clave_texto()`. El comentario que acompaña esa constante
-- dice lo que pasa si las dos copias se separan: una importación que revienta
-- contra el índice en vez de saltear la fila repetida.
--
-- Con el índice apoyado en la función, esa constante desaparece y que se
-- separen deja de ser posible.
--
-- ── Qué NO hace esta migración ─────────────────────────────────────────
--
-- Las columnas `*_norm` se quedan donde están. No son sólo del índice del
-- curso: el emparejamiento de preferencias de equipo con materias (RF-03.21)
-- compara `equipo_preferencia.materia_norm` contra `materia.nombre_norm`, y
-- `curso.division_norm`/`modalidad_norm` contra las de la preferencia. Eso es
-- una heurística de COINCIDENCIA, no una regla de IDENTIDAD, y es
-- internamente consistente porque las dos puntas usan las mismas columnas.
-- Cambiarla es reescribir la consulta más difícil del sistema para arreglar
-- algo que no está roto.
--
-- ── Los datos que ya están cargados ────────────────────────────────────
--
-- Los bordes ya los impide un CHECK (`division = btrim(division)`), así que
-- lo único que la regla nueva ve distinto son los espacios REPETIDOS adentro:
-- «Ciclo Básico» y «Ciclo  Básico» eran dos modalidades y pasan a ser una.
--
-- Se canonizan, igual que en la 010 y la 011: un espacio doble es un
-- accidente de tipeo o de copiado, no un dato.
--
-- Lo que NO se hace acá, y es la diferencia con la 011: desambiguar
-- renombrando. Allá un carro repetido se podía dejar como «Carro 1 (2)» —
-- visible, reversible y sin inventar nada—. Un curso no tiene nombre propio:
-- se llama por su año, su división y su modalidad, y ésos son datos de la
-- institución. Renombrar automáticamente la división de un curso sería
-- inventar una división que no existe, y encima entra en 12 caracteres.
--
-- Así que si hubiera dos cursos que la regla nueva ve indistinguibles, la
-- migración FALLA con el detalle de cuáles son, y no se aplica nada —goose
-- corre cada migración en una transacción—. Es el caso en el que hace falta
-- una persona que sepa qué curso es cuál.

-- +goose Up

-- ── El orden importa, y no es el obvio ─────────────────────────────────
--
-- La comprobación va ANTES de canonizar, no después. `division_norm` y
-- `modalidad_norm` son columnas GENERADAS: se recalculan solas con cada UPDATE.
-- Así que el UPDATE que saca los espacios dobles es el que hace colisionar a
-- las dos filas bajo el índice VIEJO, y falla ahí con un «duplicate key» crudo
-- antes de que nadie pueda explicar qué pasó.
--
-- Preguntarlo antes no pierde nada: `clave_texto()` ya colapsa los espacios,
-- así que aplicada sobre el texto sin canonizar responde exactamente lo mismo
-- que responderá después.

-- +goose StatementBegin
DO $$
DECLARE
    colisiones text;
BEGIN
    -- Se agrupa SÓLO por la clave normalizada. Meter también el texto crudo en
    -- el GROUP BY haría lo contrario de lo que hace falta: separaría en dos
    -- grupos justamente a las dos filas que la regla nueva ve iguales, que son
    -- las que hay que encontrar.
    SELECT string_agg(detalle, '; ') INTO colisiones
    FROM (
        SELECT format('ciclo %s, %s° división «%s» modalidad «%s»: %s cursos (%s)',
                      ciclo_lectivo_id, anio,
                      clave_texto(COALESCE(division, '')),
                      clave_texto(COALESCE(modalidad, '')),
                      count(*),
                      string_agg(id::text, ', ')) AS detalle
        FROM curso
        GROUP BY ciclo_lectivo_id, anio,
                 clave_texto(COALESCE(division, '')), clave_texto(COALESCE(modalidad, ''))
        HAVING count(*) > 1
    ) x;

    IF colisiones IS NOT NULL THEN
        RAISE EXCEPTION
            'hay cursos que la regla de unicidad de RF-00.1 ve como el mismo: %. '
            'Renombrá o eliminá los repetidos antes de aplicar esta migración: '
            'un curso se identifica por su año, división y modalidad, y el sistema '
            'no puede elegir por vos cuál es cuál.', colisiones;
    END IF;
END $$;
-- +goose StatementEnd

-- Los espacios repetidos adentro del texto. Los del borde ya los impide el
-- CHECK de la 009, así que esto sólo toca lo que la regla nueva vería distinto.
--
-- No puede violar el índice viejo: el DO de arriba garantizó que no hay dos
-- filas con la misma clave, y sobre texto ya canonizado `*_norm` y
-- `clave_texto()` dan lo mismo.
UPDATE curso SET division = regexp_replace(division, ' {2,}', ' ', 'g')
 WHERE division ~ '  ';

UPDATE curso SET modalidad = regexp_replace(modalidad, ' {2,}', ' ', 'g')
 WHERE modalidad ~ '  ';

DROP INDEX ux_curso_ciclo_nombre;

-- El COALESCE no es decorativo: `clave_texto()` es STRICT, así que sobre NULL
-- devuelve NULL, y en un índice único los NULL son distintos entre sí. Sin él,
-- dos cursos de 1° sin división ni modalidad —el caso normal de una
-- universidad— dejarían de ser el mismo curso para la base.
CREATE UNIQUE INDEX ux_curso_ciclo_nombre ON curso (
    ciclo_lectivo_id,
    clave_texto(COALESCE(modalidad, '')),
    anio,
    clave_texto(COALESCE(division, ''))
);

COMMENT ON INDEX ux_curso_ciclo_nombre IS
    'Un curso por ciclo, año, división y modalidad (RF-02.2), comparando con '
    'clave_texto() — la misma regla que la materia, el carro, el equipo '
    'suelto, el número de serie y la licencia (RF-00.1).';

-- +goose Down

-- La vuelta atrás es limpia: la regla vieja es MÁS permisiva que la nueva, así
-- que todo lo que pasa la nueva pasa la vieja. Lo que no vuelve es el texto
-- canonizado, y está bien que no vuelva: nadie escribió ese espacio doble a
-- propósito.
DROP INDEX ux_curso_ciclo_nombre;

CREATE UNIQUE INDEX ux_curso_ciclo_nombre
    ON curso (ciclo_lectivo_id, modalidad_norm, anio, division_norm);
