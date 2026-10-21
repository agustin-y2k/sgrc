import { actorLegible, detalleLegible, resumenDelDetalle } from "./detalle"
import type { EntradaDeAuditoria } from "./types"

const carro = "41f418de-1800-4226-b6b2-9f604e4ee78e"
function entrada(
  detalle: unknown,
  extra: Partial<EntradaDeAuditoria> = {}
): EntradaDeAuditoria {
  return {
    id: "entrada",
    actorId: "actor",
    accion: "CAMBIO_TRANSACCIONAL",
    entidad: "equipo",
    creadoEn: "2026-10-13T12:00:00Z",
    detalle,
    ...extra,
  }
}

describe("presentación de auditoría", () => {
  it("traduce estados, nulos y referencias sin alterar la constancia histórica", () => {
    const registro = entrada(
      {
        operacion: "UPDATE",
        campos: ["estado", "carro_id", "reservable", "nombre"],
        antes: { estado: "DISPONIBLE", carro_id: null, reservable: false },
        despues: { estado: "EN_MANTENIMIENTO", carro_id: carro, reservable: true },
      },
      { referencias: { [`carro:${carro}`]: "Carro 2" } }
    )
    const original = JSON.stringify(registro)
    const detalle = detalleLegible(registro)
    expect(detalle.titulo).toBe("Se actualizó el equipo")
    expect(detalle.cambios).toEqual([
      { campo: "Estado", antes: "Disponible", despues: "En mantenimiento" },
      { campo: "Carro", antes: "Sin asignar", despues: "Carro 2" },
      { campo: "Se puede reservar", antes: "No", despues: "Sí" },
    ])
    expect(detalle.otrosCampos).toEqual(["Nombre"])
    expect(resumenDelDetalle(detalle)).toContain("Disponible → En mantenimiento")
    expect(JSON.stringify(registro)).toBe(original)
  })

  it.each(["INSERT", "DELETE"])(
    "reconoce %s sin inventar un valor faltante",
    (operacion) => {
      const lado = operacion === "INSERT" ? "despues" : "antes"
      const detalle = detalleLegible(
        entrada({ operacion, [lado]: { rol: "DOCENTE" } }, { entidad: "usuario" })
      )
      expect(detalle.titulo).toBe(
        operacion === "INSERT" ? "Se creó el usuario" : "Se eliminó el usuario"
      )
      expect(detalle.cambios[0][lado]).toBe("Docente")
      expect(detalle.cambios[0][lado === "antes" ? "despues" : "antes"]).toBeUndefined()
    }
  )

  it("presenta los detalles HTTP antiguos y las referencias eliminadas", () => {
    const detalle = detalleLegible(
      entrada(
        {
          estado: { antes: "DISPONIBLE", despues: "FUERA_DE_SERVICIO" },
          carroDestinoId: carro,
          soloEsta: false,
          cursosDestinoIds: [carro],
        },
        { accion: "EQUIPO_EDITADO" }
      )
    )
    expect(detalle.cambios).toEqual([
      { campo: "Estado", antes: "Disponible", despues: "Fuera de servicio" },
    ])
    expect(detalle.datos).toContainEqual({
      campo: "Carro de destino",
      valor: "Referencia no disponible",
    })
    expect(detalle.datos).toContainEqual({ campo: "Sólo esta fecha", valor: "No" })
    expect(JSON.stringify(detalle)).not.toContain(carro)
  })

  it("oculta secretos y detalles técnicos, pero conserva las señales de seguridad", () => {
    const detalle = detalleLegible(
      entrada(
        {
          operacion: "UPDATE",
          campos: [
            "password_hash",
            "version_sesion",
            "nombre_norm",
            "debe_cambiar_password",
          ],
          antes: { password_hash: "privado-anterior", version_sesion: 7 },
          despues: {
            password_hash: "privado-nuevo",
            token: "privado-token",
            debe_cambiar_password: true,
          },
        },
        { entidad: "usuario" }
      )
    )
    expect(detalle.seguridadActualizada).toBe(true)
    expect(detalle.cambios).toEqual([
      { campo: "Debe cambiar su contraseña", antes: undefined, despues: "Sí" },
    ])
    expect(detalle.otrosCampos).toEqual([])
    expect(JSON.stringify(detalle)).not.toMatch(
      /privado-|password_hash|version_sesion|nombre_norm/
    )
    const historico = detalleLegible(
      entrada(
        { tienePassword: false, secret: "privado", codigo: "privado" },
        { accion: "CUENTA_DE_EQUIPO_EDITADA" }
      )
    )
    expect(historico.datos).toEqual([
      { campo: "La cuenta tiene contraseña", valor: "No" },
    ])
    expect(historico.seguridadActualizada).toBe(true)
  })

  it.each([
    undefined,
    null,
    [],
    { operacion: "UPDATE", campos: null, antes: [], despues: "incompleto" },
  ])("tolera registros incompletos sin renderizar JSON", (detalle) => {
    expect(detalleLegible(entrada(detalle)).cambios).toEqual([])
  })

  it("distingue un actor ausente de una cuenta eliminada", () => {
    expect(
      actorLegible(entrada(null, { actorId: "00000000-0000-0000-0000-000000000000" }))
    ).toBe("Sin usuario identificado")
    expect(actorLegible(entrada(null))).toBe("Una cuenta que después se eliminó")
  })
})
