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
deje mandar algo que el backend va a rechazar. Son 418 tests en 40 archivos.

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
- **El esquema sobre una base que ya existe.** `migrations/` se aplica solo
  sobre una base vacía; llevar una instalación en marcha a un esquema nuevo se
  ensaya a mano, contra una copia, antes de tocar el servidor.

---

## 5. Comandos

```bash
make test              # tests rápidos (sin Docker) + cobertura total
make lint              # golangci-lint
go test -tags integration ./...   # + Postgres real en contenedores (lento)

cd frontend && npx vitest run       # tests de pantalla
cd frontend && npx playwright test  # e2e contra el sistema levantado
```
