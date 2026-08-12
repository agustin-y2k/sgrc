# SGRC — Frontend

SPA en React + TypeScript que consume la API de `sgrc-app`. El contrato está en
[`../docs/08-api-spec.yaml`](../docs/08-api-spec.yaml).

## Stack

- **Vite** + **React 19** + **TypeScript**
- **Tailwind CSS v4** + **shadcn/ui** (Radix) — primitivas en `src/components/ui/`
- **TanStack Query** — estado del servidor y cache de datos
- **React Router** — ruteo
- **react-hook-form** + **zod** — formularios y validación
- **oxlint** — linter
- **Prettier** — formateo
- **Vitest** + **React Testing Library** — pruebas de pantalla
- **Playwright** — E2E contra el sistema levantado (ver `e2e/`)

## Desarrollo local

```bash
npm install
npm run dev
```

El servidor de desarrollo levanta en `http://localhost:5173` y proxea `/api/*`
hacia `http://localhost:8080` (ver `vite.config.ts`) — así se evita CORS en
desarrollo sin tocar `FRONTEND_ORIGIN` del backend. Para que funcione, el
backend tiene que estar corriendo (`make run` desde la raíz del repositorio, o
`go run ./cmd`).

## Scripts

```bash
npm run dev            # servidor de desarrollo
npm run build          # type-check + build de producción a dist/
npm run lint           # oxlint
npm run format         # prettier --write
npm run format:check   # prettier --check
npm run test           # vitest en modo watch
npm run test:coverage  # vitest run --coverage
npm run e2e            # playwright test (requiere el sistema levantado)
```

### E2E

Los specs de `e2e/` corren contra el backend real, no contra mocks, así que
necesitan datos cargados de antemano. `make seed-datos` desde la raíz del
repositorio deja exactamente lo que hace falta: un ciclo activo, una materia con
docente asignado y equipos disponibles.

No piden configuración: por defecto apuntan a `http://localhost:8081` —la SPA
compilada servida por nginx, no Vite— y toman las credenciales del docente
sembrado y del `.env` del proyecto. Si no las encuentran **se saltean** en vez
de fallar, para que `npm run e2e` nunca rompa solo por el entorno.

| Variable | Para qué |
| --- | --- |
| `E2E_BASE_URL` | Dónde está el frontend (por defecto `http://localhost:8081`) |
| `E2E_ADMIN_EMAIL` / `E2E_ADMIN_PASSWORD` | El Admin sembrado por `cmd/main.go` |
| `E2E_DOCENTE_EMAIL` / `E2E_DOCENTE_PASSWORD` | Un docente **aprobado y asignado a una materia de un ciclo activo** |
| `E2E_FECHA_RESERVA` | Fecha `YYYY-MM-DD` a reservar, si el valor por defecto cae fuera del ciclo activo |

Cada corrida reserva en una **franja distinta**: el test cancela su reserva pero
no puede borrarla, así que con una franja fija la segunda corrida encontraría
dos tarjetas iguales y fallaría por ambigüedad en vez de por un problema real.

## Estructura

```
src/
├── main.tsx                ← entrypoint, monta los providers (QueryClient, Auth)
├── App.tsx                 ← definición del router
├── lib/
│   ├── api-client.ts       ← fetch tipado, header de auth y parseo de errores
│   ├── token-store.ts      ← wrapper de localStorage para el JWT
│   ├── query-client.ts     ← instancia de TanStack QueryClient
│   ├── fechas.ts           ← formato legible de fechas (la API habla ISO)
│   ├── csv.ts              ← descarga de los reportes
│   ├── google-identity.ts  ← carga y render del botón de Google
│   └── tema.ts             ← claro/oscuro (ver abajo)
├── features/
│   ├── inicio/             ← el mostrador del Admin y el tablero del docente
│   ├── auth/               ← login, registro, cambio y recuperación de contraseña, aprobación
│   ├── inventory/          ← carros y equipos (vista de consulta)
│   ├── calendario/         ← calendario semanal de un equipo
│   ├── reservas/           ← crear reserva simple o recurrente, listar, cancelar, bloquear
│   ├── disponibilidad/     ← horarios de atención de los Admin
│   ├── notificaciones/     ← la campana y el listado de avisos
│   ├── academico/          ← ciclos, cursos, materias y docentes
│   └── admin/              ← usuarios, inventario, licencias, entregas, reportes
├── components/
│   ├── ui/                 ← primitivas de shadcn (no se editan a mano salvo bugfix)
│   ├── EstadoBadge.tsx     ← un color por estado del dominio
│   ├── SelectorDeHora.tsx  ← dos <select>, sin AM/PM (ver abajo)
│   ├── BotonDeTema.tsx     ← interruptor claro/oscuro
│   └── layout/             ← shell autenticado (nav + logout) y marco de login
├── routes/                 ← guards de ruteo (ProtectedRoute, AdminRoute)
└── test/                   ← setup de Vitest y dobles compartidos
```

Cada feature sigue la misma forma: `api.ts` (llamadas tipadas), `types.ts`
(espejo de los DTOs del backend), componentes de página y sus tests al lado. Los
E2E viven aparte, en `e2e/`.

## Tres decisiones que conviene conocer antes de tocar algo

**El color sale de tokens, no de clases sueltas.** Todo el color vive en
`src/index.css`; no hay ni un `bg-gray-700` en el código. Además de los de
shadcn hay tres propios para los estados del dominio —`exito`, `alerta`,
`info`— porque con las tres variantes de shadcn un equipo en mantenimiento y
uno fuera de servicio se veían igual, los dos en rojo. `EstadoBadge.tsx` mapea
cada estado a su tono, y el color nunca va solo: cada badge lleva su texto.

**La decisión del tema inicial está duplicada a propósito.** El modo oscuro
arranca siguiendo la preferencia del sistema operativo y el interruptor de la
barra permite forzar uno u otro; la elección se guarda y le gana al sistema. La
lógica está en `src/lib/tema.ts` **y** en un script inline de `index.html`: es
el único modo de no mostrar un fogonazo blanco antes de que cargue el bundle. Si
cambia la clave `sgrc-tema` o la clase `dark`, hay que tocar los dos. Ese script
inline está autorizado en la CSP por su hash SHA-256, y `csp.test.ts` lo
recalcula y falla con el valor nuevo listo para pegar.

**La hora se elige con dos `<select>`, no con `<input type="time">`.** El input
nativo decide su formato según la configuración regional del navegador y mete
hora, minutos y AM/PM en un solo campo. `SelectorDeHora.tsx` ofrece hora 00–23 y
minutos de cinco en cinco, sin AM/PM, y conserva un minuto fuera de la grilla si
ya venía cargado — el backend acepta cualquiera, los horarios son libres a
propósito.

Al agregar una pantalla, dos cosas que el layout ya resuelve y no hay que
repetir: el ancho máximo y el padding los pone `AppLayout` (`max-w-6xl px-4
py-6`), y el encabezado va con `<EncabezadoDePagina>`.

## Variables de entorno

Ver `.env.example`. `VITE_API_URL` vacío usa paths relativos (`/api/...`), que
funcionan tanto con el proxy de Vite en desarrollo como con un despliegue
same-origin en producción. Es una variable de **compilación**, no de runtime:
cambiarla exige recompilar el bundle, no reiniciar el contenedor.

## Docker

```bash
docker build -t sgrc-frontend .
docker run -p 8080:80 sgrc-frontend
```

Build multi-stage: Node compila a `dist/`, nginx sirve los estáticos con
fallback a `index.html` para las rutas de React Router (`nginx.conf`).

En producción este contenedor es **el borde**: es el único servicio que expone
el túnel. Además de servir la SPA, nginx proxea `/api/*` y `/health` hacia
`sgrc-app:8080` por la red interna de Docker, lo que hace que el despliegue sea
same-origin y que `VITE_API_URL` pueda quedar vacío. Los headers de seguridad
del HTML y los assets salen de `nginx-seguridad.conf`, incluido desde los dos
`location` que sirven contenido propio; el detalle de por qué no van en el
bloque `server` está en [`../docs/09-seguridad-rbac.md`](../docs/09-seguridad-rbac.md) §4.

No hace falta levantarlo para desarrollar: `npm run dev` ya proxea `/api` al
backend. Pero antes de desplegar conviene mirar el `:8081`, que es el build de
producción real.
</content>
