# SGRC — Frontend

SPA en React + TypeScript que consume la API de `sgrc-app` (ver `../docs/08-api-spec.yaml`
para el contrato, aunque en algunos puntos está desactualizado contra la
implementación real — ver comentarios en `src/lib/api-client.ts`).

## Stack

- **Vite** + **React 19** + **TypeScript**
- **Tailwind CSS v4** + **shadcn/ui** (Radix) — componentes en `src/components/ui/`
- **TanStack Query** — estado del servidor / cache de datos
- **React Router** — ruteo
- **react-hook-form** + **zod** — formularios y validación
- **oxlint** — linter (viene por default con este template de Vite, más
  rápido que ESLint)
- **Prettier** — formateo
- **Vitest** + **React Testing Library** — tests unitarios/de componentes
- **Playwright** — E2E del flujo crítico login → reservar → cancelar (ver
  `e2e/`; se corre a mano, requiere datos sembrados)

## Desarrollo local

```bash
npm install
npm run dev
```

El servidor de dev levanta en `http://localhost:5173` y proxea `/api/*`
hacia `http://localhost:8080` (ver `vite.config.ts`) — así se evita CORS en
desarrollo sin necesitar configurar `FRONTEND_ORIGIN` del backend. Para que
esto funcione, el backend tiene que estar corriendo (`docker compose up`
desde la raíz del repo, o `go run ./cmd`).

## Scripts

```bash
npm run dev              # servidor de desarrollo
npm run build             # type-check + build de producción a dist/
npm run lint               # oxlint
npm run format              # prettier --write
npm run format:check         # prettier --check
npm run test                 # vitest en modo watch
npm run test:coverage         # vitest run --coverage
npm run e2e                    # playwright test (requiere backend + build corriendo)
```

### E2E

Los specs de `e2e/` corren contra el backend real, no contra mocks, así que
necesitan datos cargados de antemano. `make seed-datos` desde la raíz del
repo deja exactamente lo que hace falta (ciclo activo, materia con docente
asignado y PCs disponibles). Se auto-skipean si no encuentran las
credenciales, para que `npm run e2e` nunca falle solo por el entorno:

| Variable                                     | Para qué                                                                                |
| -------------------------------------------- | --------------------------------------------------------------------------------------- |
| `E2E_BASE_URL`                               | Dónde está el frontend (default `http://localhost:5173`)                                |
| `E2E_ADMIN_EMAIL` / `E2E_ADMIN_PASSWORD`     | El Admin sembrado por `cmd/main.go` — `login.spec.ts`                                   |
| `E2E_DOCENTE_EMAIL` / `E2E_DOCENTE_PASSWORD` | Un docente **aprobado y asignado a una materia de un ciclo activo** — `reserva.spec.ts` |
| `E2E_FECHA_RESERVA`                          | Fecha `YYYY-MM-DD` a reservar, si el default (hoy + 14 días) cae fuera del ciclo activo |

`reserva.spec.ts` además necesita al menos una PC en estado `DISPONIBLE`.

```bash
E2E_DOCENTE_EMAIL=docente@escuela.edu.ar \
E2E_DOCENTE_PASSWORD=... \
  npm run e2e
```

## Estructura

```
src/
├── main.tsx              ← entrypoint, monta providers (QueryClient, Auth)
├── App.tsx                ← definición del router
├── lib/
│   ├── api-client.ts       ← fetch tipado, maneja auth header y parseo de errores
│   ├── token-store.ts        ← wrapper de localStorage para el JWT
│   ├── query-client.ts        ← instancia de TanStack QueryClient
│   ├── fechas.ts               ← formato legible de fechas (la API habla ISO)
│   └── tema.ts                  ← claro/oscuro (ver abajo)
├── features/
│   ├── inicio/                ← tablero: qué hay hoy y qué está esperando
│   ├── auth/                  ← login, registro, cambio de password, aprobación
│   ├── inventory/              ← carros y PCs (vista de consulta)
│   ├── calendario/              ← calendario semanal de reservas de una PC
│   ├── reservas/                 ← crear reserva simple/recurrente, listar, cancelar
│   └── admin/                     ← panel de Admin: usuarios, inventario, reportes
├── components/
│   ├── ui/                     ← primitivas de shadcn (no se editan a mano salvo bugfix)
│   ├── EstadoBadge.tsx          ← un color por estado del dominio
│   ├── BotonDeTema.tsx           ← interruptor claro/oscuro
│   └── layout/                    ← shell autenticado (nav + logout) y marco de login
├── routes/                       ← guards de ruteo (ProtectedRoute, AdminRoute)
└── test/                          ← setup de Vitest
```

Cada feature sigue la misma forma: `api.ts` (llamadas tipadas), `types.ts`
(espejo de los DTOs del backend), componentes de página, y sus tests al lado.

Los E2E viven aparte, en `e2e/` — ver arriba.

## Colores y tema

Todo el color sale de tokens CSS de `src/index.css`; no hay ni un
`bg-gray-700` suelto en el código. Además de los de shadcn hay tres propios
para los estados del dominio —`exito`, `alerta`, `info`— porque con las tres
variantes de shadcn una PC en mantenimiento y una fuera de servicio se veían
igual, las dos en rojo. `src/components/EstadoBadge.tsx` mapea cada estado a
su tono, y el color nunca va solo: cada badge lleva su texto.

El modo oscuro arranca siguiendo la preferencia del sistema operativo y el
interruptor de la barra permite forzar uno u otro; la elección se guarda y le
gana al sistema. La decisión del tema inicial está **duplicada** en el script
inline de `index.html`: es el único modo de no mostrar un fogonazo blanco
antes de que cargue el bundle. Si cambia la clave `sgrc-tema` o la clase
`dark`, hay que tocar `index.html` y `src/lib/tema.ts`.

Al agregar una pantalla, dos cosas que el layout ya resuelve y no hay que
repetir: el ancho máximo y el padding los pone `AppLayout` (`max-w-6xl px-4
py-6`), y el encabezado va con `<EncabezadoDePagina>`.

## Variables de entorno

Ver `.env.example`. `VITE_API_URL` vacío usa paths relativos (`/api/...`),
que funcionan tanto con el proxy de Vite en dev como con un deploy
same-origin en producción (ver comentario en `vite.config.ts`).

## Docker

```bash
docker build -t sgrc-frontend .
docker run -p 8080:80 sgrc-frontend
```

Build multi-stage: Node compila a `dist/`, nginx sirve los estáticos con
fallback a `index.html` para las rutas de React Router (`nginx.conf`).

En producción este contenedor es **el borde**: es el único servicio que
expone el túnel de Cloudflare. Además de servir la SPA, nginx proxea `/api/*`
y `/health` hacia `sgrc-app:8080` por la red interna de Docker, lo que hace
que el deploy sea same-origin y que `VITE_API_URL` pueda quedar vacío. Ver
el diagrama en el README de la raíz.

No hace falta levantarlo para desarrollar: `npm run dev` ya proxea `/api` al
backend.
