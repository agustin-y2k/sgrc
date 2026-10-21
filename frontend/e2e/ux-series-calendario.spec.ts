import AxeBuilder from "@axe-core/playwright"
import { expect, test } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

test("API local: comprueba una serie real sin crear reservas", async ({ request }) => {
  const ingreso = await request.post("/api/auth/login", {
    data: { email: DOCENTE_EMAIL, password: DOCENTE_PASSWORD },
  })
  expect(ingreso.ok()).toBe(true)
  const { token } = await ingreso.json()
  const headers = { Authorization: `Bearer ${token}` }
  const materias = await request.get("/api/mis-materias", { headers })
  const materiaId = (await materias.json()).data[0].materiaId
  const jornada = await request.get("/api/jornada", { headers })
  const bloque = (await jornada.json()).data[0]
  const dias = ["DOMINGO", "LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES", "SABADO"]
  const desde = new Date()
  desde.setDate(desde.getDate() + 14)
  if (bloque)
    while (dias[desde.getDay()] !== bloque.diaSemana) desde.setDate(desde.getDate() + 1)
  const hasta = new Date(desde)
  hasta.setDate(hasta.getDate() + 7)
  const iso = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  const aMinutos = (hhmm: string) => {
    const [horas, minutos] = hhmm.split(":").map(Number)
    return horas * 60 + minutos
  }
  const inicio = aMinutos(bloque?.horaInicio ?? "08:00")
  const cierre = aMinutos(bloque?.horaFin ?? "09:00")
  // La jornada puede superar las ocho horas que admite una clase o ser nocturna.
  const fin = Math.min(inicio + 60, cierre > inicio ? cierre : cierre + 24 * 60)
  const horario = {
    horaInicio: `${String(Math.floor(inicio / 60)).padStart(2, "0")}:${String(inicio % 60).padStart(2, "0")}`,
    horaFin: `${String(Math.floor(fin / 60) % 24).padStart(2, "0")}:${String(fin % 60).padStart(2, "0")}`,
  }
  const disponibles = await request.get(
    `/api/equipos-disponibles?${new URLSearchParams({ fecha: iso(desde), materiaId, ...horario })}`,
    { headers }
  )
  const equipoId = (await disponibles.json()).data[0].equipoId
  const antes = await request.get("/api/reservas?porClases=true", { headers })
  const totalAntes = (await antes.json()).meta.total
  const preview = await request.post("/api/reservas/recurrentes/previsualizar", {
    headers,
    data: {
      materiaId,
      diaSemana: dias[desde.getDay()],
      ...horario,
      fechaInicio: iso(desde),
      fechaFin: iso(hasta),
      equipoIds: [equipoId],
    },
  })
  expect(preview.status()).toBe(200)
  const resultado = await preview.json()
  expect(resultado.cantidadClases).toBe(2)
  expect(resultado.fechas.map((f: { fecha: string }) => f.fecha)).toEqual([
    iso(desde),
    iso(hasta),
  ])
  expect(resultado.puedeCrear).toBe(true)
  const despues = await request.get("/api/reservas?porClases=true", { headers })
  expect((await despues.json()).meta.total).toBe(totalAntes)
})

test("serie: un conflicto posterior exige consentimiento y los cambios invalidan la comprobación", async ({
  page,
}) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  await page.route("**/api/mis-materias", (r) =>
    r.fulfill({
      json: {
        data: [
          {
            materiaId: "m-ux",
            materiaNombre: "TIC",
            cursoNombre: "3°1",
            cicloAnio: 2026,
          },
        ],
      },
    })
  )
  await page.route("**/api/jornada", (r) => r.fulfill({ json: { data: [] } }))
  await page.route("**/api/equipos-disponibles?*", (r) =>
    r.fulfill({
      json: { data: [{ equipoId: "pc-ux", etiqueta: "PC de prueba", tramo: "NEUTRAL" }] },
    })
  )
  let bloqueo = false
  await page.route("**/api/reservas/recurrentes/previsualizar", (r) =>
    r.fulfill({
      json: {
        cantidadClases: 3,
        puedeCrear: !bloqueo,
        equiposATomar: bloqueo ? [] : ["pc-ux"],
        fechas: ["2026-11-02", "2026-11-09", "2026-11-16"].map((fecha, i) => ({
          fecha,
          equipos: [
            {
              equipoId: "pc-ux",
              etiqueta: "PC de prueba",
              estado: i === 0 ? "LIBRE" : bloqueo ? "NO_DISPONIBLE" : "POR_PRIORIDAD",
              motivo:
                i === 0
                  ? undefined
                  : bloqueo
                    ? "Bloqueado por administración."
                    : "Se desplazarán reservas en esta fecha.",
            },
          ],
        })),
      },
    })
  )
  let escrituras = 0
  await page.route("**/api/reservas/recurrentes", (r) => {
    escrituras++
    return r.fulfill({
      status: 409,
      body: "La disponibilidad cambió. Volvé a comprobar.",
    })
  })
  await page.goto("/reservas/nueva")
  await page.getByRole("button", { name: "Se repite todas las semanas" }).click()
  await page.getByLabel("Materia", { exact: true }).selectOption("m-ux")
  await page.getByLabel("Desde", { exact: true }).fill("2026-11-02")
  await page.getByLabel("Hasta", { exact: true }).fill("2026-11-16")
  await page.getByLabel("Hora de inicio: hora").selectOption("08")
  await page.getByLabel("Hora de fin: hora").selectOption("09")
  await page.getByRole("checkbox", { name: /PC de prueba/ }).check()
  const confirmar = page.getByRole("button", { name: "Confirmar reserva", exact: true })
  await expect(confirmar).toBeDisabled()
  await page.getByRole("button", { name: "Comprobar serie" }).click()
  await expect(page.getByText(/3 clases comprobadas/)).toBeVisible()
  await expect(confirmar).toBeDisabled()
  await page.getByRole("checkbox", { name: /Autorizo tomar/ }).check()
  await expect(confirmar).toBeEnabled()
  await confirmar.click()
  await expect(
    page.getByText("La disponibilidad cambió. Volvé a comprobar.", { exact: true })
  ).toBeVisible()
  await expect(confirmar).toBeDisabled()
  bloqueo = true
  await page.getByRole("button", { name: "Comprobar serie" }).click()
  await expect(page.getByText(/Hay equipos no disponibles/)).toBeVisible()
  await expect(confirmar).toBeDisabled()
  await page.setViewportSize({ width: 320, height: 780 })
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
      .violations
  ).toEqual([])
  expect(escrituras).toBe(1)
})

test("calendario: teclado, cierre con Escape y lista móvil muestran todos los detalles", async ({
  page,
}) => {
  await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
  await page.route("**/api/jornada", (r) => r.fulfill({ json: { data: [] } }))
  await page.route("**/api/equipos/pc-ux/calendario?*", (r) => {
    const desde = new URL(r.request().url()).searchParams.get("desde")!
    return r.fulfill({
      json: {
        equipoId: "pc-ux",
        etiqueta: "PC de prueba",
        desde,
        hasta: desde,
        bloques: [
          {
            reservaId: "r-ux",
            fecha: desde,
            horaInicio: "23:00",
            horaFin: "01:00",
            estado: "CONFIRMADA",
            tipo: "NORMAL",
            materiaNombre: "Dibujo Técnico",
            cursoNombre: "3°1",
            docente: "Docente de prueba",
          },
          {
            reservaId: "b-ux",
            fecha: desde,
            horaInicio: "08:00",
            horaFin: "10:00",
            estado: "CONFIRMADA",
            tipo: "BLOQUEO",
            motivoBloqueo: "Revisión de los equipos",
            docente: "",
          },
        ],
      },
    })
  })
  await page.goto("/inventario/equipos/pc-ux/calendario")
  const fragmento = page
    .getByRole("button", { name: /Dibujo Técnico.*Docente de prueba/ })
    .last()
  await fragmento.focus()
  await page.keyboard.press("Enter")
  const dialogo = page.getByRole("dialog", { name: "Detalle de la reserva" })
  await expect(dialogo).toContainText("23:00–01:00 (termina al día siguiente)")
  await expect(page.getByRole("button", { name: "Cerrar detalles" })).toBeFocused()
  await page.keyboard.press("Escape")
  await expect(dialogo).toHaveCount(0)
  await expect(fragmento).toBeFocused()
  await page.setViewportSize({ width: 320, height: 780 })
  await expect(
    page.getByRole("button", { name: "Lista de reservas", exact: true })
  ).toHaveAttribute("aria-pressed", "true")
  await page
    .getByRole("button", { name: /Revisión de los equipos.*Ver detalles/ })
    .click()
  await expect(page.getByRole("dialog", { name: "Bloqueo del equipo" })).toContainText(
    "Revisión de los equipos"
  )
  expect(
    (await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze())
      .violations
  ).toEqual([])
  await page.screenshot({
    path: test.info().outputPath("calendario-detalle-movil.png"),
    fullPage: true,
  })
})
