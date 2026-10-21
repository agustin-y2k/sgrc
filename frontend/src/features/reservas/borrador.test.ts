import {
  guardarBorradorReserva,
  leerBorradorReserva,
  type BorradorReserva,
} from "./borrador"
import {
  BORRADOR_RESERVA_KEY,
  borrarBorradorReserva,
  descartarBorradorDeOtraCuenta,
} from "@/lib/borradores"

const datos: BorradorReserva = {
  modo: "simple",
  materiaId: "m1",
  busquedaMateria: "",
  volverA: "/reservas",
  fecha: "2026-11-02",
  horaInicio: "08:00",
  horaFin: "09:00",
  diaSemana: "LUNES",
  fechaInicio: "",
  fechaFin: "",
  seleccion: { clave: "", ids: [], aTomar: [] },
}

describe("borrador de reserva por cuenta", () => {
  beforeEach(() => sessionStorage.clear())
  afterEach(() => vi.restoreAllMocks())
  it("recupera los datos de la misma cuenta", () => {
    expect(guardarBorradorReserva("u1", datos)).toBe(true)
    expect(leerBorradorReserva("u1")).toEqual(datos)
  })
  it("descarta datos de otra cuenta", () => {
    guardarBorradorReserva("u1", datos)
    expect(leerBorradorReserva("u2")).toBeNull()
    expect(sessionStorage.getItem(BORRADOR_RESERVA_KEY)).toBeNull()
  })
  it.each([
    "no es json",
    JSON.stringify({ version: 2, usuarioId: "u1", datos }),
    JSON.stringify({
      version: 1,
      usuarioId: "u1",
      datos: { ...datos, modo: "invalido" },
    }),
  ])("descarta un borrador incompatible", (valor) => {
    sessionStorage.setItem(BORRADOR_RESERVA_KEY, valor)
    expect(leerBorradorReserva("u1")).toBeNull()
    expect(sessionStorage.getItem(BORRADOR_RESERVA_KEY)).toBeNull()
  })
  it("no permite autorizar tomas ajenas a la selección", () => {
    sessionStorage.setItem(
      BORRADOR_RESERVA_KEY,
      JSON.stringify({
        version: 1,
        usuarioId: "u1",
        datos: { ...datos, seleccion: { clave: "", ids: ["pc1"], aTomar: ["pc2"] } },
      })
    )
    expect(leerBorradorReserva("u1")).toBeNull()
  })
  it("informa un fallo real de guardado sin lanzar una excepción", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("sin espacio")
    })
    expect(guardarBorradorReserva("u1", datos)).toBe(false)
  })
  it("limpia al descartar o al entrar otra cuenta", () => {
    guardarBorradorReserva("u1", datos)
    descartarBorradorDeOtraCuenta("u2")
    expect(sessionStorage.getItem(BORRADOR_RESERVA_KEY)).toBeNull()
    guardarBorradorReserva("u1", datos)
    borrarBorradorReserva()
    expect(sessionStorage.getItem(BORRADOR_RESERVA_KEY)).toBeNull()
  })
})
