import { render, screen, waitFor } from "@testing-library/react"

import { AvisoDeSpam } from "@/components/AvisoDeSpam"
import * as authApi from "@/features/auth/api"

vi.mock("@/features/auth/api")

/**
 * Lo que se prueba acá es la decisión de RF-05.8: el aviso solo aparece si
 * este despliegue manda correos, y cuando aparece nombra la dirección exacta
 * —que es lo único que de verdad saca al remitente de spam, porque deja que
 * la persona lo agregue a sus contactos una vez.
 */
describe("AvisoDeSpam", () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it("nombra la dirección desde la que salen los correos", async () => {
    vi.mocked(authApi.configPublica).mockResolvedValue({
      googleClientId: "",
      remitenteDeCorreo: "avisos@escuela.edu.ar",
    })

    render(<AvisoDeSpam />)

    expect(await screen.findByText("avisos@escuela.edu.ar")).toBeInTheDocument()
  })

  it("deja poner el texto de cada pantalla", async () => {
    vi.mocked(authApi.configPublica).mockResolvedValue({
      googleClientId: "",
      remitenteDeCorreo: "avisos@escuela.edu.ar",
    })

    render(<AvisoDeSpam>El aviso puede caer en spam.</AvisoDeSpam>)

    expect(await screen.findByText(/El aviso puede caer en spam\./)).toBeInTheDocument()
  })

  // Sin SMTP configurado no hay correos de los que hablar, y una línea que
  // diga "fijate en spam" en un despliegue que no manda nada es peor que no
  // decir nada: manda a buscar algo que no existe.
  it("sin correo configurado no dibuja nada", async () => {
    vi.mocked(authApi.configPublica).mockResolvedValue({ googleClientId: "" })

    const { container } = render(<AvisoDeSpam />)

    await waitFor(() => expect(authApi.configPublica).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })

  /**
   * Es una ayuda al margen: si la consulta falla, la pantalla que la persona
   * vino a usar tiene que seguir funcionando igual.
   */
  it("si falla la consulta, se calla", async () => {
    vi.mocked(authApi.configPublica).mockRejectedValue(new Error("sin red"))

    const { container } = render(<AvisoDeSpam />)

    await waitFor(() => expect(authApi.configPublica).toHaveBeenCalled())
    expect(container).toBeEmptyDOMElement()
  })
})
