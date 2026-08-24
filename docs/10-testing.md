# Estrategia de testing — SGRC

Qué se prueba, con qué, y dónde vive cada tipo de test. Los comandos están en
el `Makefile` y en §5.

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
la cobertura actual es:

| Capa | Cobertura |
|---|---|
| `domain/` de los 7 paquetes | 89–100% |
| `application/` de los 7 paquetes | 62–90% |

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
deje mandar algo que el backend va a rechazar. Son 622 tests en 57 archivos.

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
  cada vez que se agrega un ítem al menú.
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

Los E2E no piden configuración: apuntan a `http://localhost:8081` —la SPA
compilada servida por nginx, no Vite— y toman las credenciales del docente
sembrado y del Admin del `.env` del proyecto. Cada corrida reserva en una
**franja distinta**, porque el test cancela su reserva pero no puede
borrarla: con una franja fija, la segunda corrida encontraba dos tarjetas
iguales y fallaba por ambigüedad en vez de por un problema real.

---

## 4. Lo que los tests no cubren

Vale tenerlo escrito, porque es donde hay que mirar a mano:

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
go test -tags integration ./...   # + Postgres real en contenedores (lento)

cd frontend && npx vitest run       # tests de pantalla
cd frontend && npx playwright test  # e2e contra el sistema levantado
```

---

## 6. Integración continua

`.github/workflows/ci.yml` corre en cada push a `main` y a `develop`, y en
cada Pull Request contra esas ramas, lo mismo que §5 salvo los E2E. Son seis
jobs independientes —si uno falla, los demás igual terminan—:

| Job | Qué corre |
|---|---|
| **Backend — build y tests rápidos** | `go build ./...`, `go vet ./...`, `go mod tidy` sin cambios, `make test` |
| **Backend — golangci-lint** | `make lint`, con la versión de la herramienta fijada |
| **Backend — govulncheck** | vulnerabilidades conocidas de las dependencias y de la biblioteca estándar |
| **Backend — integración** | `go test -tags integration ./...` (Docker del runner) |
| **Frontend — lint, build y tests** | `npm ci`, `npm run lint`, `npm run build`, `vitest run` |
| **Imágenes de Docker** | `docker build` de las dos imágenes, sin publicarlas |

Cuatro decisiones que conviene conocer antes de tocar el archivo:

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
- **Por eso la línea `go` de `go.mod` lleva el parche**, `1.25.13` y no
  `1.25.0`: es el piso de versión con el que se acepta compilar este módulo,
  y `setup-go` instala exactamente lo que ahí diga. Con `1.25.0` la corrida
  encontró 26 vulnerabilidades de la biblioteca estándar, una alcanzable
  desde `ValidarEmail`. Cuando ese piso suba hay que verificar que la imagen
  `golang:1.25-alpine` del Dockerfile lo siga cumpliendo — hoy trae
  exactamente `go1.25.13`.
- **Las imágenes se construyen y se descartan.** Lo que se despliega son
  ellas, no el binario que compila el job de build: un Dockerfile roto, o un
  `npm ci` que falla sobre `node:24-alpine` y no sobre el Node del runner,
  aparecería recién al actualizar el servidor. No se publican a ningún
  registro; el servidor sigue compilando las suyas con `docker compose up
  --build`.
- **Los E2E no corren en CI.** Necesitan el sistema entero levantado (nginx
  con la SPA compilada, el backend y la base sembrada), y sus credenciales
  salen del `.env`, que no está en el repositorio. Se corren a mano contra
  `make run` antes de un release.

`prettier --check` tampoco está en CI: el formato del código todavía no está
normalizado y el chequeo fallaría en archivos que nadie tocó. Correr
`npm run format` una vez y sumarlo al job de frontend es un cambio aparte,
con su propio commit de formateo.

`.github/dependabot.yml` completa lo anterior: propone semanalmente las
actualizaciones de Go, de npm y de las propias acciones del workflow, en
Pull Requests contra `develop`. govulncheck avisa que hay un problema;
Dependabot es lo que hace que la actualización sea una decisión de rutina y
no una urgencia.

**`main` está protegida**: los seis checks son obligatorios para entrar, no se
puede forzar el historial ni borrar la rama, y hay que llegar con la rama al
día respecto de `main`. En la práctica eso significa que un release va por
Pull Request desde `develop` y no mergeando en local. `develop` queda sin
proteger a propósito: el CI corre igual y avisa, sin volver ceremonia cada
cambio del día a día.

La protección **no se aplica a los administradores**, que es una decisión
tomada y no un olvido: si el sistema se cae en la institución, hay que poder
subir el arreglo sin esperar veinte minutos de suite de integración. Como
guardarraíl sirve —el botón de merge avisa antes de dejar pasar algo en
rojo—, pero no es un candado.
