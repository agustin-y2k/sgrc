# Prioridades de dispositivos y relación académica

Cada prioridad relaciona una materia con un dispositivo concreto. El nivel 1
es el mayor y el 9 el menor. Las características de hardware y el software
instalado no asignan prioridades automáticamente; cada institución configura
las marcas según sus necesidades.

Una materia con mayor prioridad puede desplazar una reserva de menor prioridad
sobre ese equipo. Los empates no desplazan. Se respetan los bloqueos,
las reservas que ya tuvieron una entrega, aunque el equipo se haya devuelto,
y la protección de reservas que empiezan en una hora
o menos. El sistema intenta reponer los equipos desplazados en cascada; si no
encuentra sustitutos, cancela sólo los equipos afectados en las fechas en
conflicto y genera los avisos correspondientes.

La entrega protege esa ocurrencia concreta frente a la toma por prioridad y
también frente a los desplazamientos secundarios del autorrelleno. Las próximas
fechas de una serie sin entregas siguen participando de las prioridades. Si una
serie solicitada choca con una ocurrencia protegida, se rechaza completa sin
modificar las demás reservas. Disponibilidad, previsualización y confirmación
utilizan la misma regla; una entrega concurrente confirmada antes de la toma
queda protegida al volver a comprobar los conflictos dentro de la transacción.

## Selección académica

El formulario permite seleccionar modalidad → año → división → materia. Las
opciones proceden de las relaciones del ciclo activo, sin cursos ni materias
archivados. Cambiar un dato anterior limpia los siguientes. El mismo formulario
se utiliza en un equipo y en una selección del carro.

Una materia de 2°A no se combina con 2°B por tener el mismo nombre. La API
rechaza las combinaciones inexistentes con 400 antes de modificar el lote.
Una edición inválida conserva la marca anterior.

## Curso concreto y regla general

| Alcance | Significado |
|---|---|
| Curso concreto, opción inicial | Compara exactamente modalidad, año y división de la materia. |
| Sin modalidad / Sin división | El curso realmente carece de ese atributo. No son comodines. |
| Regla general para varios cursos o espacios | Los filtros «Todas» abarcan todos los valores. Sólo admite combinaciones con alguna coincidencia activa. |

Una regla general puede indicar Matemática de Electromecánica para todos los
años o todas las materias llamadas Dibujo Técnico. Las materias de espacios se
configuran mediante reglas generales. La prioridad sigue perteneciendo a cada
dispositivo; no se deduce de sus características técnicas.

La marca de curso concreto tiene especificidad tres, incluso si carece de
modalidad o división. Una regla general cuenta sus filtros presentes. A igual
especificidad gana el número de prioridad menor y luego la marca más antigua.
No cambia la protección de empates, bloqueos, equipos entregados y reservas que
empiezan en una hora o menos, ni el autorrelleno en cascada de reservas desplazadas.

## API, datos y compatibilidad

- `GET /api/catalogo-prioridades`, sólo para administradores, devuelve cada materia
  con su curso o espacio y atributos. Un catálogo vacío devuelve `data: []`.
- Las altas y ediciones verifican que el alcance exista sin archivar en el ciclo
  activo. La comprobación participa en la coordinación de cambios académicos y
  reservas; no depende de la validación del navegador.
- `cursoExacto=true` exige año y compara también la ausencia de modalidad/división.
  Con false, los atributos omitidos conservan el significado general anterior.
  En PATCH, omitir `cursoExacto` conserva el indicador de la marca existente;
  los ejes se envían completos con el alcance deseado.
- `/api/materias-en-uso` conserva el catálogo histórico por compatibilidad.
  No sustituye la validación de la relación académica.
- La migración 029 agrega el indicador con false a las marcas anteriores, sin
  reescribir sus alcances. Una marca antigua sin coincidencia activa puede
  quitarse o reemplazarse por una válida.
- Las marcas siguen vinculadas por nombre normalizado y atributos, de manera
  que sobreviven a la clonación anual. No dependen del ID anual de materia/curso.
- Los motivos de disponibilidad, marcas huérfanas y auditoría muestran el alcance
  con términos legibles. Los cambios académicos invalidan el catálogo del navegador.

La bajada de la migración se bloquea mientras haya marcas concretas: eliminar
el indicador podría convertir una ausencia exacta en un comodín. Antes de
revertir hay que revisar y resolver esos alcances.

## Pruebas

La cobertura incluye selectores dependientes, atributos ausentes, reglas
generales, lotes atómicos, especificidad, marcas huérfanas, desplazamientos,
reposición y continuidad de las marcas al clonar el ciclo. Los comandos y
entornos de ejecución se describen en [Testing](10-testing.md).

## Cambiar un curso y conservar sus prioridades

Antes de modificar año, división o modalidad, el administrador revisa las marcas
que dejan de aplicar o empiezan a aplicar. Puede conservar las prioridades exactas
con ese curso o aceptar el alcance que resulta del cambio. Una marca compartida con
otros cursos no se traslada dejándolos sin prioridad: se conserva y se copia cuando
corresponde. Un destino con otra prioridad incompatible obliga a revisar inventario.
La confirmación incluye la versión del impacto; si alguien cambió el curso o las
marcas mientras tanto, se exige revisar de nuevo.

## Coincidencias archivadas

Las prioridades que ordenan y permiten tomar equipos sólo consideran materias y
contenedores sin archivar, en ciclos sin archivar. Cerrar un ciclo no hace operativas
sus materias antiguas. Una coincidencia únicamente archivada sigue apareciendo en
«Prioridades sin materias vigentes», identificada con `coincideConArchivadas`.
La marca no se borra automáticamente: puede volver a corresponder a la estructura
clonada del año siguiente. Las marcas generales y los alcances exactos conservan
sus reglas de comparación; archivar no convierte una ausencia exacta en comodín.
