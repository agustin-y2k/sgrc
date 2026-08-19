import { apiFetch } from "@/lib/api-client"
import type {
  Carro,
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
