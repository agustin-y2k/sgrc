# Generación de las guías

Las guías de docente y administrador se mantienen en `../guia-docentes.html`
y `../guia-admins.html`. Sus imágenes se capturan con **Playwright y Chromium**
sobre el frontend compilado, nginx, la API y PostgreSQL reales. Los PDF se
imprimen en A4 con Chromium. Las capturas no se dibujan ni simulan respuestas.

Desde la raíz del repositorio:

```sh
make capturas
```

Requiere Docker con Compose, Node, `npm ci` en `frontend`, Chromium de
Playwright (`npx playwright install chromium` desde `frontend`) y `pdftotext`.
El punto de entrada es `scripts/generar-guias.mjs`; el shell
`capturar-todo.sh` conserva la entrada del Makefile.

## Entorno y datos

La ejecución compila ambos servicios y levanta un proyecto único `sgrc-guias-*`.
PostgreSQL usa memoria temporal, sin volumen persistente ni puerto publicado.
La interfaz se publica en un puerto libre de `127.0.0.1`; la red usa
`172.31.45.0/24`, por lo que se ejecuta una generación por vez. El entorno
habitual `sgrc-local` puede seguir funcionando.

No se leen configuraciones activas ni respaldos de inventario.
Las claves y contraseñas de las cuentas del sistema se generan para la corrida;
los nombres y correos de demostración usan el dominio reservado `escuela.test`.
El correo externo y el ingreso externo quedan deshabilitados. Las pantallas
opcionales se explican en el texto; las capturas muestran lo que ofrece esta
instalación, sin agregar botones artificialmente.

`datos-actuales.mjs` crea, mediante la API, cursos, materias, docentes, equipos,
prioridades, reservas puntuales y recurrentes, licencias, cuentas ficticias de
notebook, incidencias, un préstamo, horarios y una conversación respondida.
El asistente de primera jornada se captura antes de cargar esos datos.
Las contraseñas visibles de las cuentas de notebook son ejemplos ficticios.

## Captura y publicación

`capturar-actual.mjs` navega con las sesiones de cada rol, abre formularios y
confirmaciones y espera sus consultas. Incluye la previsualización y el
consentimiento de una serie con prioridad y el detalle legible de auditoría.
También captura la confirmación al quitar docentes con y sin otras cuentas
asignadas, y el menú de administración en un teléfono. Esas confirmaciones se
cierran con **Volver** para conservar las asignaciones y sus reservas.
No confirma cierres, bajas ni entregas para obtener una imagen de sus formularios.

La imagen de recuperación se obtiene interrumpiendo deliberadamente la descarga
de un módulo del frontend. Se muestra el mensaje real del sistema, sin sustituir
respuestas de la API, y se comprueba que **Recargar la página** vuelva a abrir la
vista. Este escenario queda identificado en `generacion.json`.

Las imágenes se capturan directamente del navegador, con encuadre de pantalla
o de panel. No se recortan después ni se numeran con coordenadas antiguas.
Cada imagen que usa el HTML debe haberse capturado en esta ejecución; un fallo
corta la generación en vez de conservar una imagen anterior como si fuera nueva.
También se rechazan errores de JavaScript.

Los archivos se preparan primero en una carpeta privada `/tmp/sgrc-guias-*`.
Allí quedan `capturas.json`, las imágenes y los diagnósticos si falla una captura.
Antes de sustituir imágenes y PDF publicados se generan ambos documentos, se
comprueba que todas sus imágenes carguen y se extrae el texto para verificar
que contengan el final y al menos treinta páginas. Al terminar se retiran los
contenedores y la base temporal, también ante fallos. El archivo publicado
`../generacion.json` registra la versión, la fecha y la huella SHA-256 de cada
imagen para comprobar su correspondencia con esa ejecución. Los PDF incluyen
índice con enlaces, marcadores y etiquetas de estructura.
La fecha de capturas de las portadas se actualiza a partir de esa ejecución,
en la zona horaria de la institución, sólo después de validar ambos PDF.
Regenerar únicamente los PDF conserva esa fecha.

Revisar visualmente las imágenes y páginas representativas de ambos PDF antes
de dar por terminada una actualización: que una imagen exista no garantiza que
el encuadre ni el texto de la guía expliquen bien la tarea.

## Cambios de texto

```sh
make capturas-pdf
```

Regenera ambos PDF con las imágenes publicadas, sin levantar Docker. Si cambió
la interfaz, usar `make capturas` para renovar también las imágenes. La
renovación se decide por cambios en el sistema; no es un requisito automático
de cada commit y no la ejecuta CI.

Las capturas documentales se destinan únicamente a las dos guías de uso y se
guardan en `docs/guias/imagenes`. El README enlaza las guías y sus PDF.
