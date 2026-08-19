import { useState } from "react"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { SelectorDeHora } from "@/components/SelectorDeHora"

/**
 * El control nativo `<input type="time">` decidía su formato según la
 * configuración regional del navegador: en una máquina en inglés pedía AM/PM,
 * y metía hora, minutos y AM/PM en un solo campo.
 */

function Anfitrion({ inicial = "" }: { inicial?: string }) {
  const [valor, setValor] = useState(inicial)
  return (
    <>
      <SelectorDeHora
        id="prueba"
        etiqueta="Hora de inicio"
        valor={valor}
        onCambio={setValor}
      />
      <output>{valor === "" ? "(vacío)" : valor}</output>
    </>
  )
}

describe("SelectorDeHora", () => {
  it("arma HH:MM eligiendo hora y minutos por separado", async () => {
    const user = userEvent.setup()
    render(<Anfitrion />)

    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "17")
    await user.selectOptions(screen.getByLabelText("Hora de inicio: minutos"), "30")

    expect(screen.getByText("17:30")).toBeInTheDocument()
  })

  // 24 horas: las 5 de la tarde son 17, no "5 PM". Sin AM/PM no hay nada
  // que interpretar ni un tercer segmento escondido en el mismo campo.
  it("ofrece las 24 horas del día y ninguna opción AM/PM", () => {
    render(<Anfitrion />)

    const horas = screen.getByLabelText("Hora de inicio: hora")
    const opciones = Array.from(horas.querySelectorAll("option")).map((o) => o.value)

    expect(opciones).toContain("00")
    expect(opciones).toContain("17")
    expect(opciones).toContain("23")
    expect(opciones.filter((v) => v !== "")).toHaveLength(24)
    expect(screen.queryByText(/AM|PM/)).not.toBeInTheDocument()
  })

  it("muestra un valor ya cargado repartido en los dos campos", () => {
    render(<Anfitrion inicial="08:45" />)

    expect(screen.getByLabelText("Hora de inicio: hora")).toHaveValue("08")
    expect(screen.getByLabelText("Hora de inicio: minutos")).toHaveValue("45")
  })

  // Elegir solo una de las dos partes tiene que dar una hora válida: dejar
  // el formulario a medias no le sirve a nadie.
  it("completa la parte que falta con 00", async () => {
    const user = userEvent.setup()
    render(<Anfitrion />)

    await user.selectOptions(screen.getByLabelText("Hora de inicio: hora"), "09")

    expect(screen.getByText("09:00")).toBeInTheDocument()
  })

  /**
   * Los minutos van de 5 en 5, pero el backend acepta cualquiera (los
   * horarios son libres: la escuela no tiene módulos fijos).
   */
  it("conserva un minuto que no cae en la grilla de 5", () => {
    render(<Anfitrion inicial="07:13" />)

    const minutos = screen.getByLabelText("Hora de inicio: minutos")
    expect(minutos).toHaveValue("13")
    expect(Array.from(minutos.querySelectorAll("option")).map((o) => o.value)).toContain(
      "13"
    )
  })

  it("sin elegir nada queda vacío, no en 00:00", () => {
    render(<Anfitrion />)

    expect(screen.getByText("(vacío)")).toBeInTheDocument()
    expect(screen.getByLabelText("Hora de inicio: hora")).toHaveValue("")
  })
})
