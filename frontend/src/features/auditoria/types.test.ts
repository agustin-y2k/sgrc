import {
  accionLegible,
  entidadEnFrase,
  entidadLegible,
} from "@/features/auditoria/types"

describe("cómo se nombra lo que el registro anota", () => {
  it("arma el nombre de la acción a partir de lo guardado", () => {
    expect(accionLegible("CUENTA_APROBADA")).toBe("Cuenta aprobada")
  })

  // Lo guardado no lleva tildes y no siempre se lee bien palabra por palabra:
  // `jornada_institucion` se muestra como «Jornada de la escuela», que es como
  // la llama la interfaz.
  it("le pone acentos y nombre real a las entidades conocidas", () => {
    expect(entidadLegible("jornada_institucion")).toBe("Jornada de la escuela")
    expect(entidadLegible("equipo_cuenta")).toBe("Cuenta de equipo")
  })

  /**
   * La regla, no los dos textos: NINGUNA entidad conocida puede producir una
   * frase con el artículo equivocado.
   *
   * Salió de una captura de la guía, que mostraba «Ver todo lo de esta
   * usuario»: el botón concatenaba un «esta» fijo con un sustantivo de
   * cualquier género. Comprobar sólo los dos casos que estaban mal dejaría
   * pasar el tercero que alguien agregue.
   */
  it.each([
    ["usuario", "este usuario"],
    ["carro", "este carro"],
    ["curso", "este curso"],
    ["ciclo_lectivo", "este ciclo lectivo"],
    ["equipo", "este equipo"],
    ["pc", "este equipo"],
    ["pedido_de_materia", "este pedido de materia"],
    ["equipo_cuenta", "esta cuenta de equipo"],
    ["materia", "esta materia"],
    ["licencia", "esta licencia"],
    ["reserva", "esta reserva"],
    ["docente_materia", "esta asignación de docente"],
    ["jornada_institucion", "esta jornada de la escuela"],
  ])("%s se nombra «%s»", (entidad, frase) => {
    expect(entidadEnFrase(entidad)).toBe(frase)
  })

  // Una entidad que el backend empiece a auditar mañana no tiene que esperar a
  // que alguien la agregue a la tabla para leerse bien.
  it("cae en algo neutro y bien escrito si la entidad es desconocida", () => {
    expect(entidadEnFrase("cosa_nueva_que_no_existe")).toBe("esta ficha")
    expect(entidadLegible("cosa_nueva_que_no_existe")).toBe(
      "Cosa nueva que no existe"
    )
  })
})
