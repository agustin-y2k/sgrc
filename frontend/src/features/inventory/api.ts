import { apiFetch } from "@/lib/api-client"
import type {
  Carro,
  GravedadIncidencia,
  Incidencia,
  PC,
  RespuestaLista,
} from "@/features/inventory/types"

// RF-03.7: cualquier usuario autenticado puede ver carros y PCs — un
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
  return apiFetch<RespuestaLista<PC>>("/api/inventory/equipos")
}

export function listarPCsDeCarro(carroId: string) {
  return apiFetch<RespuestaLista<PC>>(`/api/inventory/carros/${carroId}/pcs`)
}

// ── Incidencias (RF-03.5) ─────────────────────────────────────────────
//
// Estas dos son de `autenticado`, no de Admin: el docente que se sienta
// frente a la PC es el que ve que falla. Gestionarlas después (cambiar el
// estado, marcar el envío a DGE) sí es de Admin y vive en features/admin.

export function listarIncidenciasDePC(pcId: string) {
  return apiFetch<RespuestaLista<Incidencia>>(`/api/inventory/pcs/${pcId}/incidencias`)
}

export function reportarIncidencia(req: {
  pcId: string
  descripcion: string
  gravedad: GravedadIncidencia
}) {
  return apiFetch<Incidencia>("/api/inventory/incidencias", {
    method: "POST",
    body: req,
  })
}
