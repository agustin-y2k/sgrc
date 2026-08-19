import { datosDeLaCredencial } from "@/lib/google-identity"

/** Un ID token con el payload que interesa. */
function credencialCon(payload: Record<string, unknown>): string {
  const aBase64Url = (obj: unknown) =>
    btoa(String.fromCharCode(...new TextEncoder().encode(JSON.stringify(obj))))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "")
  return [aBase64Url({ alg: "RS256" }), aBase64Url(payload), "firma-que-no-se-mira"].join(
    "."
  )
}

describe("datosDeLaCredencial", () => {
  it("lee el email y el nombre del token", () => {
    const datos = datosDeLaCredencial(
      credencialCon({
        email: "ada@escuela.edu.ar",
        given_name: "Ada",
        family_name: "Lovelace",
      })
    )

    expect(datos).toEqual({
      email: "ada@escuela.edu.ar",
      nombre: "Ada",
      apellido: "Lovelace",
    })
  })

  // atob devuelve bytes, no caracteres: sin la conversión a UTF-8 los nombres
  // con acentos llegan rotos al formulario, que es la mitad de los apellidos
  // de una escuela argentina.
  it("no rompe los acentos", () => {
    const datos = datosDeLaCredencial(
      credencialCon({
        email: "martin@escuela.edu.ar",
        given_name: "Martín",
        family_name: "Peña",
      })
    )

    expect(datos?.nombre).toBe("Martín")
    expect(datos?.apellido).toBe("Peña")
  })

  // given_name y family_name no son claims obligatorios.
  it("tolera que falten el nombre y el apellido", () => {
    const datos = datosDeLaCredencial(credencialCon({ email: "ada@escuela.edu.ar" }))

    expect(datos).toEqual({ email: "ada@escuela.edu.ar", nombre: "", apellido: "" })
  })

  it("devuelve null si no es un JWT", () => {
    expect(datosDeLaCredencial("cualquier-cosa")).toBeNull()
    expect(datosDeLaCredencial("")).toBeNull()
    expect(datosDeLaCredencial("a.b")).toBeNull()
  })

  it("devuelve null si el payload no es JSON", () => {
    expect(datosDeLaCredencial("header.no-es-json-valido.firma")).toBeNull()
  })
})
