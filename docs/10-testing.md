# Estrategia de testing — SGRC

Qué se prueba, con qué, y dónde vive cada tipo de test. Los comandos están en
el `Makefile` y en §5.

Las pruebas se organizan en reglas de dominio, servicios de aplicación,
contratos HTTP, integración con PostgreSQL, componentes y recorridos de
navegador. Las suites verifican permisos, concurrencia, cancelaciones,
reposición por prioridad, cierre anual, recuperación de errores y navegación
por ambos roles.

Las suites `ux-*` interceptan operaciones concretas para provocar estados
controlados. Las suites de flujos reales crean datos ficticios mediante la API
en bases efímeras y comprueban los cambios persistidos. No utilizan inventario
ni credenciales de una instalación de producción.

`frontend/e2e/flujos-reservas-reales.spec.ts`: crea docentes, materias, marcas,
equipos y reservas ficticios mediante la API real; confirma desde el navegador
reservas puntuales y recurrentes, reposición en cascada y cancelaciones por rol.
Verifica los sustitutos, el horario completo, la conservación de fechas ajenas
y los avisos internos. Incluye una lectura de jornada fallida para comprobar
que reintentar conserva el formulario. Se activa únicamente con
`E2E_RESERVAS_AISLADAS=1`, un origen localhost y una base sin ciclos ni equipos;
esa condición se comprueba antes de crear datos. Deben proporcionarse
`E2E_BASE_URL`, `E2E_ADMIN_EMAIL`, `E2E_ADMIN_PASSWORD` y una contraseña ficticia
en `E2E_DOCENTE_PASSWORD`. Se ejecuta con
`npm run e2e -- e2e/flujos-reservas-reales.spec.ts --workers=1`.
La base debe ser efímera: estos escenarios sí guardan reservas y cancelaciones.

`frontend/e2e/flujos-entregas-reales.spec.ts` revisa entregas y devoluciones
contra la API real. Crea una reserva recurrente y comprueba entrega parcial,
liberación de lo no retirado sin afectar la próxima fecha, responsable y
retirante, devolución con observaciones, entregas sin cuenta, competencia entre
mostradores, reparación y permisos de docente. Sólo intercepta una lectura de
inventario para verificar bloqueo y recuperación del formulario.
Exige `E2E_ENTREGAS_AISLADAS=1`, origen localhost y una base sin ciclos, equipos
ni préstamos. Usa las mismas variables del administrador y `E2E_DOCENTE_EMAIL`
y `E2E_DOCENTE_PASSWORD` para crear el docente ficticio. Ejecutar con
`npm run e2e -- e2e/flujos-entregas-reales.spec.ts --workers=1`.
La base debe ser efímera y restaurarse vacía antes de repetir: estos escenarios
sí guardan préstamos y devoluciones. No habilitar para ello la suite de cierre
anual `E2E_REVISION_AISLADA`.

`frontend/e2e/flujos-cierre-anual-reales.spec.ts` crea un ciclo, curso con
modalidad, espacio, docente, prioridades, reserva, préstamo e incidencia
ficticios. Cierra desde el navegador con y sin ciclo nuevo, verifica histórico,
asignaciones, prioridades y devolución posterior, consulta la auditoría y
comprueba la apertura manual después del cierre. Repite la misma solicitud y
simula una respuesta perdida después de confirmar el cierre real para verificar
su recuperación. Sólo intercepta lecturas fallidas y el transporte de esa
respuesta: las operaciones de archivo atraviesan el backend real.
Exige `E2E_CIERRE_AISLADO=1`, origen localhost y una base efímera sin ciclos,
equipos ni préstamos antes de crear datos. Usa las mismas variables de
administrador y docente indicadas para entregas. Ejecutar con
`npm run e2e -- e2e/flujos-cierre-anual-reales.spec.ts --workers=1`.
Estos escenarios eliminan reservas ficticias al cerrar: restaurar la base
aislada vacía antes de repetir, sin habilitar las otras suites de escritura.
Las pruebas de rollback y competencia real entre transacciones se ejecutan con
`go test -tags integration ./cmd -run '^TestCierreAnual_' -count=1 -timeout=5m`.

`frontend/e2e/flujos-inventario-reales.spec.ts` verifica altas, prioridades por
dispositivo, licencias, movimiento entre carros, mantenimiento y cancelación de
reservas, custodia antes de una baja, reactivación con identificador reutilizado
y permisos por rol. Las escrituras atraviesan la API real. Sólo se intercepta
una lectura de inventario para provocar un fallo y comprobar su recuperación.
Exige `E2E_INVENTARIO_AISLADO=1`, origen localhost y una base efímera sin ciclos,
equipos, préstamos ni licencias. Usa `E2E_BASE_URL`, `E2E_ADMIN_EMAIL`,
`E2E_ADMIN_PASSWORD`, `E2E_DOCENTE_EMAIL` y `E2E_DOCENTE_PASSWORD` ficticios.
Se ejecuta con `npm run e2e -- e2e/flujos-inventario-reales.spec.ts --workers=1`.
Restaurar vacía la base aislada antes de repetir; no habilitar otras suites de
escritura sobre la misma base.

`frontend/e2e/flujos-soporte-reales.spec.ts` verifica pedidos, respuestas, cierre
explícito y reapertura, avisos para varios administradores sin duplicados,
lectura/borrado de avisos, privacidad entre docentes, preferencias y recuperación
ante fallos conservando borradores. Las escrituras válidas atraviesan la API real;
sólo se intercepta un envío fallido y una consulta fallida. Incluye accesibilidad
WCAG A/AA y ancho móvil con mensajes largos.
Exige `E2E_SOPORTE_AISLADO=1`, origen localhost y una base efímera sin ciclos,
equipos, conversaciones ni avisos, con sólo el administrador inicial. Declara
una jornada de prueba y crea un segundo administrador y dos docentes. Usa las
mismas cinco variables `E2E_*` de las suites anteriores y se ejecuta con
`npm run e2e -- e2e/flujos-soporte-reales.spec.ts --workers=1`.
Restaurar vacía esa base antes de repetir. `e2e/ux-soporte.spec.ts` complementa
la entrada al panel desde escritorio y menú móvil con lecturas simuladas.

`frontend/e2e/flujos-reportes-auditoria-reales.spec.ts` revisa totales reales,
filtros, CSV y neutralización de fórmulas, fallos de lectura con datos en caché,
paginación y detalle legible de auditoría, coherencia al cambiar el tipo de
ficha, límites de fechas, permisos por rol, histórico tras el cierre y actividad
de una cuenta eliminada. Sólo intercepta lecturas para provocar fallos: crea y
modifica los datos mediante la API real, incluidos los cambios necesarios para
obtener más de una página de auditoría.
Exige `E2E_REPORTES_AISLADO=1`, origen localhost y una base efímera sin ciclos,
equipos ni préstamos, con sólo el administrador inicial. Usa las cinco variables
`E2E_*` de las suites anteriores. Ejecutar con
`npm run e2e -- e2e/flujos-reportes-auditoria-reales.spec.ts --workers=1`.
Cierra el ciclo ficticio y elimina al docente ficticio; restaurar vacía la base
antes de repetir y no habilitar otras suites de escritura sobre ella.

`frontend/e2e/flujos-docentes-reales.spec.ts` comprueba la confirmación de baja,
Volver sin escritura, el rechazo de un envío y la conservación de reservas con
dos titulares y dos suplentes. Sólo la cuarta baja cancela la reserva de esa
materia; otra división y otro año conservan sus reservas. El bloque `docentes`
del script conjunto crea una base efímera, siembra las cuentas y declara
`E2E_DOCENTES_AISLADOS=1`. La suite exige origen localhost y comprueba la base
esperada antes de crear sus datos. Reutiliza los dos docentes sembrados y crea
dos más, respetando el límite real de registros. Repetir con una base nueva.

`frontend/e2e/revision-final.spec.ts` se ejecuta en el bloque `vistas`. Interrumpe
una descarga de módulo y comprueba recuperación real mediante recarga, con la
sesión conservada y ninguna escritura de negocio. También simula 125 avisos y
conversaciones para recorrer páginas como administrador y docente, midiendo
los controles contra el viewport y sus ancestros a 320, 390, 768 y 1280 px.

### Regresión conjunta de navegador

Desde la raíz del repositorio, ejecutar `make regresion-navegador` (o
`node scripts/regresion-navegador.mjs`). Requiere Docker con Compose, Node,
dependencias instaladas con `npm ci` en `frontend` y Chromium de Playwright
(`npx playwright install chromium` desde `frontend`).

El script compila una única pareja de imágenes y ejecuta once bloques: las
seis suites reales anteriores, el bloque de asignaciones docentes, las siete suites `ux-*`, las pruebas básicas
de acceso/móvil/tema/reservas, la revisión transversal y las regresiones RV-01 a
RV-09 (`revision-vistas.spec.ts`) y FF-02/FF-03 (`revision-final.spec.ts`). Cada bloque recibe
un PostgreSQL efímero vacío y cuentas ficticias; el transversal puede cambiar
la contraseña y cerrar el ciclo sin afectar al siguiente bloque. Se declara
una jornada de siete días y una clase futura de una hora para que el escenario
táctil se ejecute. Los bloques complementarios reciben dos docentes aprobados;
`E2E_OTRO_DOCENTE_EMAIL` y `E2E_OTRO_DOCENTE_PASSWORD` identifican la segunda
cuenta para comprobar que no se transfiera el borrador al cambiar de usuario.
Las suites `ux-*` conservan sus interceptaciones; el conjunto no sustituye
la revisión manual con lector de pantalla ni la observación de usuarios.

No utiliza `.env.local`, copias de inventario ni el proyecto `sgrc-local`.
Las credenciales y claves son aleatorias y el puerto sólo se publica en
`127.0.0.1`. Los contenedores y la base en memoria se retiran al terminar,
también ante fallos. Ejecutar una corrida por vez: la red aislada usa
`172.31.44.0/24`. Las imágenes compiladas se conservan para inspección.

Imprime una carpeta privada `/tmp/sgrc-regresion-*` con `resultado.json`,
identificadores de las imágenes y los informes/artefactos por bloque. Devuelve
un código distinto de cero si falla un bloque o queda alguna prueba omitida.
Para diagnosticar bloques concretos:

```sh
node scripts/regresion-navegador.mjs --solo reservas,entregas
node scripts/regresion-navegador.mjs --solo vistas
node scripts/regresion-navegador.mjs --solo docentes
```

La selección vuelve a compilar y sigue usando bases nuevas. Después de corregir
un fallo de la aplicación, ejecutar el conjunto completo para verificar la
misma versión en todos los bloques. Si sólo se ajustan datos o esperas de una
prueba, repetir su bloque y conservar ambos informes con el alcance indicado.

---

## 1. Backend (Go)

| Tipo | Qué cubre | Cómo corre |
|---|---|---|
| **Unitario de dominio** | Reglas puras: máquinas de estado, solapamiento, generación de ocurrencias, ventana temporal de una reserva | `go test ./...` |
| **Unitario de aplicación** | Casos de uso completos contra repositorios en memoria (fakes), incluidas las cascadas entre paquetes | `go test ./...` |
| **De handler** | Contrato HTTP: códigos de estado, permisos por rol, parseo de query y body | `go test ./...` |
| **De integración** | Repositorios contra un **Postgres real** levantado en un contenedor efímero | `go test -tags integration ./...` |
| **De migración** | Que la convención del esquema se cumpla, y que una actualización aplicada sobre una base **con datos** no se los lleve puestos | `go test ./migrations/` y con `-tags integration` |
| **De arquitectura** | Que ningún paquete importe el `domain/` de otro | `go test ./...` |

Los tests viven **junto al código** que prueban (`reserva.go` /
`reserva_test.go`), siguiendo la convención de Go.

### Por qué los de integración van detrás de un build tag

Necesitan Docker: `testcontainers-go` levanta un Postgres, le aplica las
migraciones reales de `/migrations` y lo destruye al terminar. Con
`//go:build integration`, en una máquina sin Docker esos archivos ni se
compilan y `make test` sigue andando — no hace falta un flag extra ni
recordar saltearlos.

Son lentos (varios minutos: cada paquete levanta su contenedor), así que no
van en el ciclo corto. Lo que se prueba ahí es lo que **solo puede fallar
contra la base**: la constraint `EXCLUDE` de anti-solapamiento, la aritmética
`fecha + hora_fin` con zona horaria, los `$n` del `LIMIT/OFFSET` cuando hay
filtros dinámicos, y que `COUNT(*) OVER()` cuente antes del recorte.

### Los tests del esquema

`migrations/` tiene dos tests propios, y los dos cuidan errores que ninguna
herramienta avisa sola:

- **Sin Docker** (`go test ./migrations/`): que las versiones sean
  correlativas y no se repitan —dos ramas numerando 002 a la vez—, que cada
  archivo traiga sus dos anotaciones de goose, y que el SQL de la 001 no haya
  cambiado. La 001 está congelada: editarla anda perfecto en desarrollo, donde
  la base nace de cero, y no llega nunca a una instalación que ya la aplicó.
- **Con Postgres** (`-tags integration`): arma una base en el punto de
  partida, le mete datos de una escuela en uso y **después** le aplica las
  migraciones que falten. Es el único test que ejercita el camino del
  servidor; los demás levantan la base vacía, donde una migración destructiva
  no tiene nada que destruir. Falla si una tabla pierde filas, si desaparece,
  o si los datos sembrados cambian de valor al migrar.

El detalle de cómo escribir una migración que sobreviva a esto está en
`11-operacion.md` §5.

### Coordinación entre módulos

`cmd/inventario_coordinacion_integracion_test.go` ejercita los servicios y
adaptadores reales sobre PostgreSQL. Pausa lecturas y valida que la operación
competidora espera en la base antes de continuar; después comprueba el estado
persistido, la respuesta y los avisos.

Cubre reservas simples y recurrentes y cambio de equipo contra mantenimiento,
entregas con y sin reserva contra bajas, altas/movimientos/reactivación contra
baja de carro, edición contra circulación/baja, jornada contra nuevas reservas,
rollback de cascadas sin avisos, barrido contra entrega espontánea y ejecución
con un pool de una sola conexión. Las regresiones de H-12, préstamos, bajas de
cuenta y cierre anual se conservan.

```bash
go test -tags integration ./cmd -run '^TestInventarioCoordinado' -count=1 -timeout=4m
```

### El test de límites de dominio

`internal/shared/archtest` recorre los imports de cada paquete y falla si uno
importa el `domain/` de otro. Es lo que sostiene la decisión de
`06-arquitectura.md` §3: sin un test, esa disciplina se erosiona con el
primer atajo que parezca inofensivo.

---

## 2. Cobertura

No hay una meta única para todo el repo: el número global mezcla el dominio
con el código de infraestructura, que solo cubren los tests de integración
(y por eso no aparece en `make test`).

Donde importa —**`domain/` y `application/`, que es donde viven las reglas**—
la cobertura medida el **12/10/2026** con `go test ./... -coverprofile=...`,
sin el tag de integración, es:

| Capa | Cobertura |
|---|---|
| `domain/` de los 9 módulos | 80,3–100% |
| `application/` con pruebas propias (8 módulos) | 71,9–88,6% |
| `auditoria/application` sin pruebas propias | 0% en esta medición |

El total de esta medición, después de ampliar las pruebas HTTP, dio **51,7%**,
y ese número mezcla lo de arriba
con la infraestructura, que solo cubren los tests de integración — así que no
sirve como meta: bajaría agregando un repositorio y subiría borrándolo.

Un cero que no significa lo que parece: **`auditoria/application` figura en 0%**.
No tiene tests propios en ese paquete. Las pruebas HTTP normales ejecutan el
servicio, pero el perfil por defecto atribuye cobertura al paquete probado;
para medir también sus dependencias se necesita `-coverpkg`. La suite normal
excluye las pruebas HTTP con PostgreSQL de `cmd`. Los filtros y repositorios
también tienen pruebas con el tag de integración. Los valores cambiarán al
modificar código o pruebas.

Lo que se busca cubrir siempre: las tres cascadas de cancelación (bloqueo
administrativo, equipo fuera de servicio, materia sin docente), el archivado de
ciclo —donde el orden de los pasos es lo único que evita perder datos— y las
transiciones de estado inválidas.

Reportes:

```bash
make test              # corre todo y muestra el total
make coverage-report   # genera coverage.html navegable
```

---

## 3. Frontend (React)

| Tipo | Qué cubre | Herramienta |
|---|---|---|
| **De pantalla** | Cada página con la capa `api` mockeada: qué se muestra, qué se manda, qué se deshabilita | Vitest + Testing Library |
| **E2E** | Flujos completos contra el sistema levantado | Playwright (`frontend/e2e/`) |

Se prueba **por pantalla y por rol**, no por componente aislado: lo que
importa es que un docente no vea acciones de Admin y que un formulario no
deje mandar algo que el backend va a rechazar. La revisión del 17/10/2026
verificó **1068 pruebas en 79 archivos**, incluyendo sesión, cliente HTTP,
paginación, formularios y los seis bloques funcionales del informe de flujos.
La entrega del 19/10/2026 amplió ese total a **1089 pruebas en 79 archivos**
y verificó las nuevas regresiones RV junto con las anteriores.

Dos criterios que ya evitaron falsos verdes:

- **Fechas relativas a hoy, nunca constantes.** Los inputs de fecha tienen
  `min={hoy}` y jsdom valida restricciones: una fecha fija funciona hasta que
  queda en el pasado, y ahí el test empieza a fallar sin que nadie haya
  tocado nada. Ver `src/test/fechas.ts`.
- **Orden estable en los fakes.** Los dobles de prueba que devuelven páginas
  ordenan explícitamente: sobre un `map` el orden cambia entre corridas y un
  test de paginación pasa o falla al azar.
- **El ancho se mide, no se mira.** `e2e/responsive.spec.ts` recorre las
  pantallas a 320, 375, 768, 1024, 1180 y 1440px y falla si el documento es más
  ancho que la ventana, nombrando al elemento culpable. El caso que cubre es la
  barra de navegación completa de un Admin: no desborda en un monitor de
  desarrollo, sí en un portátil de 1024, no se ve en una captura, y vuelve sola
  cada vez que se agrega un ítem al menú. El punto de corte entre la barra
  horizontal y el menú del teléfono es un número **medido**, no elegido, y el
  mismo valor vive en cinco clases de `AppLayout.tsx`: si una queda atrás, hay
  un ancho en el que se ven los dos menús a la vez, o ninguno. Agregar "Inicio"
  lo corrió de 1100 a 1140 —a 1121px la barra deja de partirse en dos
  renglones—, que es exactamente el modo en que esto vuelve.
- **Un E2E no puede asumir un sistema sin configurar.** `e2e/reserva.spec.ts`
  elegía su franja horaria entre las 05:00 y las 07:00 —una banda poco habitual,
  a propósito, para no chocar con reservas reales—, lo que funciona mientras la
  jornada institucional esté sin declarar, que es el estado de un entorno recién
  sembrado. Apenas alguien carga el horario real de la escuela el backend
  rechaza esa franja y el test del flujo crítico se cae: dejaba de probarse justo
  en la instalación más parecida a la de producción. Ahora consulta
  `GET /api/jornada` y arma la franja adentro de lo declarado, con la banda vieja
  como respaldo para cuando no hay jornada. La regla general: si el test elige un
  valor que el sistema puede llegar a rechazar por configuración, tiene que
  leer esa configuración en vez de adivinarla.
- **Lo que se toca, también.** `e2e/tactil.spec.ts` mide en un teléfono el alto
  de cada control a la vista —cuerpo, barra, menú desplegado y pie— y falla por
  debajo de 44px (24px si el enlace va embebido en una frase, que no puede
  crecer sin partir el renglón). Mismo tipo de defecto que el anterior: un botón
  con el tamaño `sm` del sistema de diseño son 28px —la mitad del ancho de un
  dedo—, se ve perfecto en una captura y solo molesta con el teléfono en la
  mano. El menú se mide en un test aparte porque sus enlaces no existen en el
  DOM hasta que alguien lo abre.
- **El contraste se calcula, no se opina.** Los tokens del proyecto son
  `oklch(…)` y los botones tintados componen su color con un alpha, así que el
  color efectivo no se puede deducir leyendo el CSS: hay que pintarlo. El
  botón `destructive` medía 3.62:1 en claro y 4.3:1 en oscuro —por debajo del
  4.5:1 de WCAG AA— y se veía "un poco pálido", que es exactamente el tipo de
  juicio que no sirve para decidir. Al medirlo se corrige una vez y queda
  arreglado en las dos variantes de tema a la vez.

```bash
cd frontend
npx vitest run        # tests de pantalla
npx playwright test   # e2e (requiere el sistema levantado con `make run`)
```

Los E2E habituales apuntan a `http://localhost:8081` —la SPA
compilada servida por nginx, no Vite— y toman las credenciales del docente
sembrado y del Admin del `.env` del proyecto. Cada corrida reserva en una
**franja distinta**, porque el test cancela su reserva pero no puede
borrarla: con una franja fija, la segunda corrida encontraba dos tarjetas
iguales y fallaba por ambigüedad en vez de por un problema real.

---

## 4. Lo que los tests no cubren

Vale tenerlo escrito, porque es donde hay que mirar a mano:

- **Google y SMTP externos.** Hay pruebas automatizadas de HTTP y protocolo,
  pero los proveedores reales requieren configuración y ejecución local.
  La [guía local](13-pruebas-servicios-locales.md) explica cómo verificarlos.
- **Los tests de pantalla mockean la capa `api`**, así que no prueban que el
  backend acepte esos cuerpos. Cuando se agrega un endpoint conviene pegarle
  una vez con el payload real (el sistema levantado + `curl`), o el
  desacuerdo aparece recién en producción.
- **El túnel de Cloudflare** no se prueba automáticamente: es lo único del
  camino de producción que no se puede ensayar en local. nginx y el build de
  producción sí, porque los E2E van contra `:8081`.
- **El salto de una versión a la siguiente sobre datos *reales*.** Que la
  migración respete los datos sí se prueba (`go test -tags integration
  ./migrations/`), pero contra una muestra sembrada por el test: unas pocas
  filas prolijas. Lo que esa muestra no tiene es lo que hace fallar de verdad
  a una migración —el nombre con el carácter raro, el año con dos ciclos
  abiertos, la tabla con cien mil filas que tarda—. Antes de un cambio de
  esquema grande sigue conviniendo ensayarlo a mano contra una copia
  restaurada del backup.

---

## 5. Comandos

```bash
make test              # tests rápidos (sin Docker) + cobertura total
make lint              # golangci-lint
go test -tags integration ./... -timeout 25m   # + Postgres real (lento)

cd frontend && npx vitest run       # tests de pantalla
cd frontend && npx playwright test  # e2e contra el sistema levantado
```

> **El `-timeout 25m` no es decorativo.** Go corta cada paquete a los 10 minutos
> por omisión, y los paquetes grandes de integración levantan su propio Postgres
> y lo pasan: sin el flag la corrida muere con un panic de timeout que parece un
> test colgado y no lo es. CI lo lleva puesto (`ci.yml`), y a mano hay que
> acordarse. Los de vitest pueden tardar varios minutos; los E2E necesitan
> el sistema levantado y la base sembrada.

---

## 6. Integración continua

`.github/workflows/ci.yml` corre en cada push a `develop`, `main`, `release/**`
y `hotfix/**`, y en cada Pull Request dirigido a `develop`, `main` o
`release/**`. También admite ejecución manual con `workflow_dispatch` una vez
que el workflow esté disponible en la rama predeterminada. Comprueba lo mismo
que §5 salvo los E2E. Son seis jobs
independientes —si uno falla, los demás igual terminan—:

| Job | Qué corre |
|---|---|
| **Backend — build y tests rápidos** | `go build ./...`, `go vet ./...`, `go mod tidy -diff`, `make test` |
| **Backend — golangci-lint** | `make lint`, con la versión de la herramienta fijada |
| **Backend — govulncheck** | vulnerabilidades conocidas de las dependencias y de la biblioteca estándar |
| **Backend — integración** | `go test -tags integration -p 2 ./... -timeout 25m` (Docker del runner) |
| **Frontend — lint, build y tests** | `npm ci`, `npm run lint`, `npm run build`, `vitest run` |
| **Imágenes de Docker** | `docker build` de las dos imágenes, sin publicarlas |

**Las imágenes de las guías se generan de forma explícita.** Se revisan
visualmente cuando cambia la interfaz; CI no compara capturas por píxeles ni
regenera documentos en cada commit.

Las guías se renovaron el **20/10/2026** con **56 capturas reales** de Playwright y
Chromium, incluidas las confirmaciones al quitar docentes, el menú móvil de
administración y la recuperación de una vista que no se descargó. En ese último
caso se interrumpe deliberadamente la descarga del módulo y se comprueba que la
recarga abra la vista; el escenario queda identificado en el manifiesto.
`make capturas` crea una instalación Docker efímera con datos
ficticios, captura las dos sesiones y genera ambos PDF; no usa la configuración
ni la base de inventario local. `make capturas-pdf` regenera únicamente los PDF
cuando cambia el texto. La renovación se decide según los cambios de interfaz;
no es una obligación de cada commit ni una tarea de CI.

El generador exige una imagen nueva por cada referencia del HTML, rechaza
errores de JavaScript y verifica imágenes cargadas y texto completo de ambos
PDF antes de publicarlos. `docs/guias/generacion.json` registra la versión,
fecha y huella SHA-256 de cada captura. Estas comprobaciones se complementan
con la revisión visual de encuadres y páginas representativas; no acreditan
por sí solas que todos los recorridos sean correctos. Ver el
[procedimiento de generación](guias/generar/README.md).

**Los PR hacia `main` ejecutan los seis controles sobre la combinación propuesta.**
El resultado de un push a `develop` no sustituye la validación del PR de release
o hotfix. Se conservan los nombres de los jobs para mantener la correspondencia
con los controles requeridos en la protección de ramas. No se aplican filtros
por archivos que dejen controles obligatorios pendientes. El push posterior al
merge valida también el estado publicado de `main`.

Decisiones que conviene conocer antes de tocar el archivo:

- **La versión de golangci-lint está fija.** El formato de `.golangci.yml`
  está atado a la línea mayor de la herramienta —pasar de v1 a v2 cambió el
  archivo entero y fusionó `gosimple` dentro de `staticcheck`—, así que
  seguir "la última" rompería la configuración en una corrida donde nadie
  tocó el repo. Se sube de versión cambiando las dos cosas en el mismo
  commit.
- **govulncheck analiza con la versión de Go de `go.mod`**, no con la última.
  Informa también las vulnerabilidades de la biblioteca estándar, y las que
  importan son las de la versión con la que se compila el binario que se
  despliega. Falla solo si el código **llama** a la función vulnerable: una
  dependencia con un CVE que nadie invoca no rompe la corrida, porque una
  alarma que suena por todo se aprende a ignorar.

  La herramienta se construye con `GOTOOLCHAIN: auto` sólo en ese paso,
  para admitir sus futuras necesidades de compilación. El análisis usa el
  Go del proyecto, actualmente 1.26.9, que coincide con la imagen de despliegue.

  Un resultado verde no acredita que todos los módulos estén libres de avisos.
  El análisis de llamadas puede tener límites ante ejecución dinámica o rutas
  indirectas de un framework. Revisar también los avisos de módulos y paquetes
  con `govulncheck -show verbose ./...`, y evaluar cada actualización según su
  exposición real.
- **La línea `go` de `go.mod` lleva el parche**, `1.26.9`, y la imagen
  usa `golang:1.26.9-alpine`. CI y despliegue deben analizar/compilar con esa
  misma versión. Al actualizar Go, modificar ambos archivos conjuntamente.
- **Las imágenes se construyen y se descartan.** Lo que se despliega son
  ellas, no el binario que compila el job de build: un Dockerfile roto, o un
  `npm ci` que falla sobre `node:24-alpine` y no sobre el Node del runner,
  aparecería recién al actualizar el servidor. No se publican a ningún
  registro; el servidor sigue compilando las suyas con `docker compose up
  --build`.
- **Los E2E no corren en CI.** Necesitan el sistema entero levantado (nginx
  con la SPA compilada, el backend y una base efímera con datos ficticios).
  Se ejecutan antes de un release con el procedimiento aislado de §1; el
  workflow actual no levanta ese entorno ni lee configuraciones privadas.

`prettier --check` tampoco está en CI: el formato del código todavía no está
normalizado y el chequeo fallaría en archivos que nadie tocó. Correr
`npm run format` una vez y sumarlo al job de frontend es un cambio aparte,
con su propio commit de formateo.

`.github/dependabot.yml` completa lo anterior: propone semanalmente las
actualizaciones de Go, de npm y de las propias acciones del workflow, en
Pull Requests contra `develop`. govulncheck avisa que hay un problema;
Dependabot es lo que hace que la actualización sea una decisión de rutina y
no una urgencia.

La protección de ramas se configura en GitHub y no se instala al clonar el
repositorio. Para el flujo con releases, configurar en `main` los seis controles
de la tabla como obligatorios, exigir la rama actualizada y deshabilitar los
push forzados y el borrado. Incorporar releases y hotfixes mediante Pull Request;
propagar sus correcciones a `develop`. Un fork debe revisar sus propias reglas.

Revisar expresamente si las reglas incluyen a administradores y otros actores
con permiso de excepción: exigir controles no impide su omisión por quienes
tengan autorizado el bypass.

## Regresiones de los hallazgos pendientes

- `cmd/vigencia_academica_integracion_test.go`: estructura archivada y escrituras intercaladas con cierre anual.
- `cmd/fechas_limites_integracion_test.go`: divisiones numéricas de 12 caracteres, días completos de incidencias y entrega/vigilancia/bloqueo nocturnos.
- `cmd/recuperacion_aprobacion_integracion_test.go`: reemplazos, intentos y consumos concurrentes; rollback y atribución de aprobación al recargar.
- `cmd/eventos_auditoria_integracion_test.go`: origen y evento atómicos, reinicio, dos workers, rollback y reintento de consumidor, independencia de pedidos/hilos, reclamo de liberación concurrente, barridos simultáneos y marcas fallidas, cifrado y pool con una conexión. También verifica actor/IP de auditoría y rollback si no puede registrarse.
- `migrations/pendientes_test.go`: actualización desde 021 con códigos duplicados y avisos legados; reconstrucción por tarea y bajada/reaplicación de auditoría sin perder el registro.
- Pruebas HTTP: revelación denegada ante auditor caído o ausente y cancelación administrativa por ocurrencia/serie con alcance. Prueba de servicio: listado propio restringido aunque otro caller pida un filtro ajeno. Los límites civiles se verifican además en días de 23/25 horas.

Ejecutar la suite normal, `go vet ./...` y `go test -tags integration ./... -count=1` en el entorno de Go con Docker disponible para testcontainers.

## Revisión de flujos del backend — 11/10/2026

Se revisan primero cuentas, estructura académica/cierre, reservas y custodia;
después inventario, licencias, horarios, comunicaciones, reportes y auditoría.
Las pruebas de composición de `cmd` usan servicios y adaptadores reales con
PostgreSQL temporal. No todas atraviesan HTTP; los permisos se cubren además
en las pruebas de rutas y middleware de cada módulo.

- `cmd/flujos_backend_integracion_test.go`: devolución de equipos distintos
  intercalada y cierre durable del aviso; alta/edición/borrado de preferencias
  mientras una reserva está en curso; actor/IP y rollback ante auditoría caída;
  filtrado HTTP por días locales y rechazo de rango invertido; respuesta y
  seguimiento de soporte sin contacto con entrega de avisos internos; borrado
  de última licencia pendiente, cierre de aviso y rollback si falla el outbox.
- `internal/availability/application/service_flujos_test.go`: excepción puntual
  sin patrón semanal, tanto ausencia como horario modificado, sin activar el
  fallback de atención fuera de la franja declarada.
- `internal/inventory/application/licencias_flujos_test.go`: el contador de
  pendientes disminuye al borrar licencias y alcanza cero en la última.
- `internal/sugerencias/application/service_flujos_test.go`: respuesta de admin
  y seguimiento del autor conservan su evento aunque falle el contacto.

Validación completa: `go test ./...`, `go vet ./...`,
`go test -tags integration -p 4 ./... -count=1 -timeout=25m` y
`golangci-lint run --build-tags integration ./...`.

## Endurecimiento de JWT, fotos y recurrencias

- `internal/shared/middleware/jwt_contrato_test.go`: HS256 válido; rechazo de
  HS384, HS512, ausencia de `exp` y expiración, tanto en rutas normales como en
  las que permiten contraseña temporal. Un token inválido no consulta la cuenta.
- `internal/auth/domain/foto_de_perfil_test.go`: JPEG, PNG y WebP con/sin pérdida
  reales; firmas falsas, payload truncado, límites de dimensiones y contenedor
  WebP pequeño con frame VP8/VP8L excesivo o inconsistente. Los WebP de prueba
  están en `internal/auth/domain/testdata` y no requieren herramientas externas.
- `internal/auth/interfaces/http/handlers_foto_test.go`: multipart, lectura con
  tipo real y preservación de la foto anterior ante reemplazos inválidos.
- `internal/reservation/domain/recurrencia_conteo_test.go`: conteo inclusivo
  sin recorrer el rango, calendario, bisiestos, DST y años 0001–9999.
- `internal/reservation/application/recurrencia_limites_test.go`: 45 clases
  aceptadas, 46 rechazadas y fecha final 9999 rechazada sin escrituras parciales.

Además de la suite normal, vet y lint, se verificaron con PostgreSQL los casos
`TestCrearReservaRecurrente_` de infraestructura: conflicto y rollback, robo
por ocurrencia y materialización de la serie máxima.

## Validación HTTP y operación — 12/10/2026

- `cmd/backup_test.go`: ejecuta `make backup` con un proceso Docker simulado;
  un volcado parcial fallido o vacío conserva la copia anterior, devuelve error
  y limpia el temporal. Un volcado válido reemplaza la copia con permisos 0600,
  incluso en un directorio con espacios. No consulta una base del entorno.
- `internal/auditoria/interfaces/http/handlers_test.go`: rutas con JWT y rol
  ADMIN, revocación, filtros combinados, paginación, actor eliminado, listado
  vacío, fechas inválidas e invertidas y errores de repositorio.
- `internal/sugerencias/interfaces/http/handlers_test.go`: identidad tomada de
  la sesión, conversación privada, rechazo de terceros, respuesta del admin,
  cierre/reapertura, rol de los mensajes, listados, validación y límite de
  escritura sin efectos al devolver 429.
- `cmd/http_validacion_integracion_test.go`: rutas registradas con autenticación
  contra PostgreSQL real; privacidad, filtrado y paginación efectiva, fechas
  locales de auditoría, sesión revocada, outbox y avisos del hilo. Un fallo
  inyectado al persistir el evento devuelve 500 y revierte la respuesta.
- `TestHTTPBackend_CargaConcurrenteSinDobleReserva`: 25 altas simultáneas para
  un mismo equipo/franja y 75 consultas HTTP de soporte/auditoría. Exige una
  sola alta, 24 conflictos, todas las consultas válidas y ningún grupo
  huérfano. Registra p50/p95/máximo; es una carga sintética local y no determina
  capacidad máxima ni un SLA del servidor de la institución.
- `cmd/operacion_integracion_test.go`: PostgreSQL 16, actualización 021→026
  con datos legados, cierre con clonado, volcado SQL por `pg_dump`, restauración
  en una segunda base vacía con `psql`, comparación de filas, índices,
  restricciones y triggers. Reparsea las expresiones CHECK con PostgreSQL para
  comparar su representación normalizada tras el restore. Verifica foto,
  cuenta cifrada, histórico, migraciones al reiniciar, cierre idempotente,
  secuencia del outbox y entrega del pendiente sin duplicarlo.
- `internal/shared/email/smtp_protocolo_test.go`: diálogo TCP/STARTTLS con una
  CA temporal confiada por el cliente, AUTH sólo después de validar TLS,
  destinatario y MIME con acentos, rechazo sin STARTTLS, CA no confiable,
  autenticación/destinatario rechazados y servidor sin saludo con timeout.
  Es un servidor SMTP local de prueba; no valida un proveedor externo.

Pruebas nuevas con PostgreSQL:

```bash
go test -tags integration ./cmd \
  -run 'Test(HTTPBackend_|OperacionBackend_)' -count=1 -timeout=5m -v
go test -tags integration ./migrations \
  -run '^TestHallazgosPendientes_ActualizacionConDatosYAuditoriaReversible$' \
  -count=1 -timeout=5m -v
```

Las pruebas normales de las interfaces HTTP alcanzaron **100%** en auditoría
y **86,5%** en soporte. Estos porcentajes miden sentencias ejecutadas, no
todos los comportamientos posibles ni la conformidad completa con OpenAPI.

Los ensayos reales de Google/SMTP quedan para el entorno local por decisión
del usuario. `TestGoogleReal_IDToken` y `TestSMTPReal_EnvioAutorizado` tienen
activación explícita por entorno y se omiten en la suite habitual. La
[guía local](13-pruebas-servicios-locales.md) explica configuración, comandos,
alcance y comprobación de recepción del correo.

## Revisión del frontend — 12/10/2026

La suite normal pasó 881 pruebas en 73 archivos. Las regresiones añadidas
cubren aislamiento de caché entre cuentas, respuestas tardías y revocación,
mutaciones sin reintento automático, fotos autenticadas, recurrencias,
errores de consulta/importación, préstamos homónimos, clases nocturnas y
lectura completa de páginas de la API.

`frontend/e2e/revision-frontend.spec.ts` usa la API real y un build servido por
nginx. Sus siete escenarios verifican 19 pantallas con axe, teclado y ancho
móvil; permisos entre cuentas; entrega/devolución; conversación de soporte;
alta/renovación de licencias; cambio de contraseña temporal y cierre anual.
Los siete pasaron. La suite habitual pasó 11 casos y omitió uno táctil por no
tener una clase de hoy en el fixture.

La suite ampliada está omitida por defecto: cambia la contraseña del docente y
archiva el ciclo. Sólo permite localhost y exige activación explícita. Preparar
una base ficticia separada con jornada declarada, ciclo activo, docente aprobado
con materia asignada y PC 1. Configurar `E2E_BASE_URL`, `E2E_ADMIN_EMAIL`,
`E2E_ADMIN_PASSWORD`, `E2E_DOCENTE_EMAIL` y `E2E_DOCENTE_PASSWORD`.

```bash
cd frontend
E2E_REVISION_AISLADA=1 npm run e2e -- e2e/revision-frontend.spec.ts --workers=1
```

Para repetirla, restaurar primero los fixtures aislados. Playwright usa es-AR y
America/Argentina/Buenos_Aires. axe verifica WCAG A/AA en los estados recorridos;
no sustituye una revisión manual con lector de pantalla. Google y SMTP reales
quedan para la prueba local. El build y el lint terminan sin errores; el lint
conserva avisos de Fast Refresh por exportaciones compartidas. `npm ci`,
`npm audit` y la auditoría de dependencias de producción se verificaron: sin
vulnerabilidades reportadas. El CSS estático de shadcn conserva su atribución
en `frontend/THIRD_PARTY_LICENSES.md`.

## Comprobaciones de seguridad

CI incluye `govulncheck` para el backend. Las dependencias del frontend se
revisan con `npm audit`. Los resultados dependen de la versión y de la base de
vulnerabilidades disponible en la ejecución; no se presentan como una garantía
permanente.

Las regresiones cubren restricciones de sesión, respuestas sin caché, datos
sensibles, fórmulas en CSV y configuración segura de observabilidad. La
[documentación de seguridad](09-seguridad-rbac.md) describe las medidas del
producto y sus límites.
