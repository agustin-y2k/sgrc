import { apiFetch } from "@/lib/api-client"
import type {
  Carro,
  GravedadIncidencia,
  Incidencia,
  Equipo,
  RespuestaLista,
} from "@/features/inventory/types"

// RF-03.7: cualquier usuario autenticado puede ver carros y equipos — un
// docente necesita mirar el software instalado antes de elegir qué
// reservar, no es información solo de Admin.
export function listarCarros() {
  return apiFetch<RespuestaLista<Carro>>("/api/inventory/carros")
}

/**
 * RF-03.15 — lo prestable que no está en ningún carro: el proyector, los
 * cargadores, las notebooks de otro modelo.
 *
 * Lo puede ver cualquier autenticado por el mismo motivo que los carros: un
 * docente necesita saber que existe un proyector antes de pedirlo.
 */
export function listarEquiposSueltos() {
  return apiFetch<RespuestaLista<Equipo>>("/api/inventory/equipos/sueltos")
}

export function listarEquiposDeCarro(carroId: string) {
  return apiFetch<RespuestaLista<Equipo>>(`/api/inventory/carros/${carroId}/equipos`)
}

// ── Incidencias (RF-03.5) ─────────────────────────────────────────────
//
// Estas dos son de `autenticado`, no de Admin: el docente que se sienta
// frente a el equipo es el que ve que falla. Gestionarlas después (cambiar el
// estado, marcar el envío a DGE) sí es de Admin y vive en features/admin.

export function listarIncidenciasDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<Incidencia>>(`/api/inventory/equipos/${equipoId}/incidencias`)
}

export function reportarIncidencia(req: {
  equipoId: string
  descripcion: string
  gravedad: GravedadIncidencia
}) {
  return apiFetch<Incidencia>("/api/inventory/incidencias", {
    method: "POST",
    body: req,
  })
}
