import { apiFetch } from "@/lib/api-client"
import type {
  Carro,
  CuentaDeEquipo,
  CuentaRequest,
  GravedadIncidencia,
  Incidencia,
  Equipo,
  RespuestaLista,
} from "@/features/inventory/types"

// RF-03.7: cualquier usuario autenticado puede ver carros y equipos — un
// docente necesita mirar el software instalado antes de elegir qué reservar,
// no es información solo de Admin.
export function listarCarros() {
  return apiFetch<RespuestaLista<Carro>>("/api/inventory/carros")
}

/**
 * RF-03.15 — lo prestable que no está en ningún carro: el proyector, los
 * cargadores, las notebooks de otro modelo.
 */
/** El inventario entero, o solo lo que no está en ningún carro. */
export function listarEquipos(opciones?: { soloSueltos?: boolean }) {
  const ruta = opciones?.soloSueltos
    ? "/api/inventory/equipos?enCarro=false"
    : "/api/inventory/equipos"
  return apiFetch<RespuestaLista<Equipo>>(ruta)
}

export function listarEquiposDeCarro(carroId: string) {
  return apiFetch<RespuestaLista<Equipo>>(`/api/inventory/carros/${carroId}/equipos`)
}

// ── Incidencias (RF-03.5) ─────────────────────────────────────────────
// Estas dos son de `autenticado`, no de Admin: el docente que se sienta
// frente a el equipo es el que ve que falla.

export function listarIncidenciasDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<Incidencia>>(
    `/api/inventory/equipos/${equipoId}/incidencias`
  )
}

export function reportarIncidencia(req: {
  equipoId: string
  descripcion: string
  gravedad: GravedadIncidencia
  /** Opcional: quien reporta no siempre sabe qué es lo que falla. */
  categoria?: string
}) {
  return apiFetch<Incidencia>("/api/inventory/incidencias", {
    method: "POST",
    body: req,
  })
}

/**
 * Las categorías de falla ya usadas, para sugerirlas al reportar una nueva.
 */
export function listarCategoriasDeFalla() {
  return apiFetch<RespuestaLista<string>>("/api/inventory/categorias-de-falla")
}

// ── Cuentas de usuario de cada equipo (RF-03.22) ────────────────────────

export function listarCuentasDeEquipo(equipoId: string) {
  return apiFetch<{ data: CuentaDeEquipo[] }>(
    `/api/inventory/equipos/${equipoId}/cuentas`
  )
}

/** Las clases ya cargadas, para sugerirlas sin cerrar la lista. */
export function listarClasesDeCuenta() {
  return apiFetch<{ data: string[] }>("/api/inventory/clases-de-cuenta")
}

export function crearCuentaDeEquipo(equipoId: string, req: CuentaRequest) {
  return apiFetch<CuentaDeEquipo>(`/api/inventory/equipos/${equipoId}/cuentas`, {
    method: "POST",
    body: req,
  })
}

export function editarCuentaDeEquipo(cuentaId: string, req: Partial<CuentaRequest>) {
  return apiFetch<CuentaDeEquipo>(`/api/inventory/cuentas/${cuentaId}`, {
    method: "PATCH",
    body: req,
  })
}

export function borrarCuentaDeEquipo(cuentaId: string) {
  return apiFetch<void>(`/api/inventory/cuentas/${cuentaId}`, { method: "DELETE" })
}

/**
 * POST y no GET a propósito: un GET termina en el historial del navegador y en
 * los logs con la URL completa, y además esto no es una lectura inocua — cada
 * llamada queda registrada como que alguien miró esa contraseña.
 */
export function revelarPasswordDeCuenta(cuentaId: string) {
  return apiFetch<{ password: string }>(`/api/inventory/cuentas/${cuentaId}/password`, {
    method: "POST",
  })
}
