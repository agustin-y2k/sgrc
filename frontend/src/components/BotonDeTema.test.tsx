import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"

import { BotonDeTema } from "@/components/BotonDeTema"
import { CLAVE_TEMA } from "@/lib/tema"

/**
 * matchMedia no existe en jsdom, así que cada test declara qué contesta el
 * sistema operativo.
 */
function mockSistema(prefiereOscuro: boolean) {
  const oyentes = new Set<() => void>()
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: prefiereOscuro,
      addEventListener: (_: string, fn: () => void) => oyentes.add(fn),
      removeEventListener: (_: string, fn: () => void) => oyentes.delete(fn),
    }))
  )
  return oyentes
}

describe("BotonDeTema", () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.classList.remove("dark")
    document.documentElement.style.colorScheme = ""
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("sin preferencia guardada, arranca con lo que dice el sistema", () => {
    mockSistema(true)
    render(<BotonDeTema />)

    // Ofrece volver a claro porque ya está en oscuro.
    expect(screen.getByRole("button", { name: /cambiar a modo claro/i })).toBeVisible()
  })

  it("alterna la clase que activa la paleta oscura", async () => {
    const user = userEvent.setup()
    mockSistema(false)
    render(<BotonDeTema />)

    await user.click(screen.getByRole("button", { name: /cambiar a modo oscuro/i }))

    expect(document.documentElement).toHaveClass("dark")
    // Sin esto, el navegador pinta las barras de scroll y los selectores de
    // fecha nativos en claro sobre un fondo oscuro.
    expect(document.documentElement.style.colorScheme).toBe("dark")
  })

  it("recuerda la elección, así que sobrevive a la recarga", async () => {
    const user = userEvent.setup()
    mockSistema(false)
    const { unmount } = render(<BotonDeTema />)

    await user.click(screen.getByRole("button", { name: /cambiar a modo oscuro/i }))
    expect(localStorage.getItem(CLAVE_TEMA)).toBe("oscuro")

    unmount()
    document.documentElement.classList.remove("dark")
    render(<BotonDeTema />)

    expect(screen.getByRole("button", { name: /cambiar a modo claro/i })).toBeVisible()
  })

  it("la elección a mano le gana al sistema", () => {
    localStorage.setItem(CLAVE_TEMA, "claro")
    mockSistema(true)
    render(<BotonDeTema />)

    expect(screen.getByRole("button", { name: /cambiar a modo oscuro/i })).toBeVisible()
  })

  it("mientras nadie eligió, sigue al sistema en vivo", async () => {
    const oyentes = mockSistema(false)
    render(<BotonDeTema />)
    expect(oyentes.size).toBe(1)
  })

  it("después de elegir, deja de escuchar al sistema", () => {
    localStorage.setItem(CLAVE_TEMA, "oscuro")
    const oyentes = mockSistema(false)
    render(<BotonDeTema />)

    // El sistema ya no manda: cambiar la preferencia del SO no debería
    // pisar lo que la persona eligió a propósito.
    expect(oyentes.size).toBe(0)
  })
})
