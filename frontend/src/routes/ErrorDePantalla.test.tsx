import { render, screen } from "@testing-library/react"
import { ErrorDePantalla } from "./ErrorDePantalla"

it("ofrece una explicación y salidas accesibles sin depender de descargar otra pantalla", () => {
  render(<ErrorDePantalla />)
  expect(
    screen.getByRole("heading", { level: 1, name: "No pudimos abrir esta pantalla" })
  ).toBeVisible()
  expect(screen.getByRole("button", { name: "Recargar la página" })).toBeEnabled()
  expect(screen.getByRole("link", { name: "Volver al inicio" })).toHaveAttribute(
    "href",
    "/"
  )
  expect(
    screen.queryByText(/Unexpected Application Error|TypeError|\/assets\//)
  ).not.toBeInTheDocument()
})
