export function AyudaDePrioridad() {
  return (
    <div className="text-muted-foreground grid gap-2 text-sm">
      <p>
        La prioridad se define entre una materia y cada dispositivo. No depende
        automáticamente del hardware ni del software. Si el equipo está libre, cualquier
        materia puede reservarlo.
      </p>
      <details>
        <summary className="cursor-pointer py-2 font-medium">
          Cómo se resuelven las prioridades
        </summary>
        <ul className="mt-2 grid list-disc gap-2 pl-5">
          <li>
            1 es la mayor prioridad y 9 la menor. Una materia con una marca aplicable
            puede desplazar a otra con un número mayor o sin marca. En un empate ninguna
            desplaza a la otra.
          </li>
          <li>
            Para un curso concreto, elegí modalidad, año, división y luego su materia.
            «Sin modalidad» o «Sin división» identifica cursos que carecen de ese dato; no
            significa «Todas». Para abarcar varios cursos, elegí una regla general. El
            alcance indica a qué cursos se aplica la marca. Si varias marcas alcanzan a la
            misma materia y curso, se usa la más específica; con la misma especificidad,
            el número menor.
          </li>
          <li>
            No se desplazan bloqueos administrativos, reservas que ya tuvieron una entrega
            (aunque el equipo se haya devuelto) ni reservas que empiezan en una hora o
            menos. Las fechas siguientes de una serie sin entregas conservan las
            prioridades habituales.
          </li>
          <li>
            El sistema intenta reponer los equipos desplazados: primero busca libres y
            después aplica las prioridades de cada sustituto, en cascada. Si no consigue
            reemplazos, pueden quedar equipos faltantes. Se avisa a los docentes
            afectados.
          </li>
        </ul>
        <p className="mt-2">
          Ejemplo en este dispositivo: Dibujo Técnico 1, TIC 2 y Matemática 3. En otro
          dispositivo el orden puede ser distinto.
        </p>
      </details>
    </div>
  )
}
