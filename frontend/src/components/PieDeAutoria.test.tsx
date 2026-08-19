import { render, screen } from "@testing-library/react"

import { PieDeAutoria } from "@/components/PieDeAutoria"

describe("PieDeAutoria", () => {
  // La licencia MIT solo pide una cosa a cambio de todo lo que permite: que
  // el aviso de autoría viaje con el software.
  it("muestra la autoría y la licencia", () => {
    render(<PieDeAutoria />)

    expect(screen.getByText(/Ramiro Agustin Pintos De Nucci/)).toBeInTheDocument()
    expect(screen.getByRole("link", { name: "MIT" })).toHaveAttribute(
      "href",
      "https://opensource.org/licenses/MIT"
    )
  })

  // La versión la inyecta Vite desde package.json (ver vite.config.ts).
  it("muestra un número de versión, no el marcador sin sustituir", () => {
    render(<PieDeAutoria />)

    expect(screen.getByText(/SGRC v\d+\.\d+\.\d+/)).toBeInTheDocument()
    expect(screen.queryByText(/__VERSION__/)).not.toBeInTheDocument()
  })

  it("es un landmark de pie de página", () => {
    render(<PieDeAutoria />)

    expect(screen.getByRole("contentinfo")).toBeInTheDocument()
  })
})
