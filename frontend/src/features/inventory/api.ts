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
/**
 * El inventario entero, o solo lo que no está en ningún carro.
 *
 * `enCarro=false` es un filtro de la colección y no una ruta aparte: con la
 * condición metida en el path (`/equipos/sueltos`) el segmento literal tiene
 * que registrarse antes que `/:id` o el parámetro se lo traga, y eso falla
 * en tiempo de ejecución sin avisar en compilación.
 */
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
//
// Estas dos son de `autenticado`, no de Admin: el docente que se sienta
// frente a el equipo es el que ve que falla. Gestionarlas después (cambiar el
// estado, marcar el envío a soporte) sí es de Admin y vive en features/admin.

export function listarIncidenciasDeEquipo(equipoId: string) {
  return apiFetch<RespuestaLista<Incidencia>>(`/api/inventory/equipos/${equipoId}/incidencias`)
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
 *
 * Es lo que hace que el texto libre converja: sin la lista a la vista,
 * "batería" y "Bateria" nacen como dos categorías distintas y la estadística
 * se fragmenta desde el primer día.
 */
export function listarCategoriasDeFalla() {
  return apiFetch<RespuestaLista<string>>("/api/inventory/categorias-de-falla")
}
