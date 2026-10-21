import { render, screen } from "@testing-library/react"

import { PieDeAutoria } from "@/components/PieDeAutoria"

describe("PieDeAutoria", () => {
  it("muestra la licencia del producto", () => {
    render(<PieDeAutoria />)

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
