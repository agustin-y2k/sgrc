import type { QueryClient } from "@tanstack/react-query"

/** Una ficha modificada también cambia disponibilidad, ubicaciones y reportes. */
export async function invalidarInventario(cliente: QueryClient) {
  await Promise.all(
    [
      "equipos",
      "equipos-sueltos",
      "carros",
      "reservas",
      "equipos-disponibles",
      "prestamos",
      "licencias",
      "preferencias",
      "reporte",
    ].map((clave) => cliente.invalidateQueries({ queryKey: [clave] }))
  )
}
