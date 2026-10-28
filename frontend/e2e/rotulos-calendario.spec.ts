import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("rótulos y calendario compartido con datos aislados", () => {
  test.skip(
    process.env.E2E_ROTULOS_CALENDARIO_AISLADOS !== "1",
    "requiere la base efímera del bloque rotulos-calendario"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(90_000)
  let admin = "",
    docente = "",
    equipo = "",
    serie = "",
    reserva = ""
  const fecha = new Date()
  fecha.setDate(fecha.getDate() + 1)
  const dia = new Intl.DateTimeFormat("sv-SE", {
    timeZone: "America/Argentina/Buenos_Aires",
  }).format(fecha)

  async function api(
    request: APIRequestContext,
    metodo: string,
    ruta: string,
    data?: unknown,
    token = admin
  ) {
    const respuesta = await request.fetch(ruta, {
      method: metodo,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(respuesta.ok(), `${metodo} ${ruta}: ${respuesta.status()}`).toBe(true)
    return respuesta.headers()["content-type"]?.includes("application/json")
      ? respuesta.json()
      : undefined
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    admin = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        { email: process.env.E2E_ADMIN_EMAIL, password: process.env.E2E_ADMIN_PASSWORD },
        ""
      )
    ).token
    docente = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        {
          email: process.env.E2E_DOCENTE_EMAIL,
          password: process.env.E2E_DOCENTE_PASSWORD,
        },
        ""
      )
    ).token
    const materia = (await api(request, "GET", "/api/mis-materias", undefined, docente))
      .data[0]
    const equipos = (await api(request, "GET", "/api/equipos")).data
    const pc = equipos.find((e: { identificador: number }) => e.identificador === 4)
    equipo = pc.id
    serie = pc.numeroSerie
    const otraPc = equipos.find((e: { identificador: number }) => e.identificador === 5)
    const otraMateria = await api(
      request,
      "POST",
      `/api/cursos/${materia.cursoId}/materias`,
      { nombre: "Dibujo técnico" }
    )
    reserva = (
      await api(
        request,
        "POST",
        "/api/reservas",
        {
          materiaId: materia.materiaId,
          fecha: dia,
          horaInicio: "08:00",
          horaFin: "10:00",
          equipoIds: [equipo],
          equiposATomar: [],
        },
        docente
      )
    ).reservas[0].id
    await api(request, "POST", "/api/reservas", {
      materiaId: otraMateria.id,
      fecha: dia,
      horaInicio: "09:00",
      horaFin: "11:00",
      equipoIds: [otraPc.id],
      equiposATomar: [],
    })
    const materiaBreve = await api(
      request,
      "POST",
      `/api/cursos/${materia.cursoId}/materias`,
      { nombre: "Matemática" }
    )
    await api(request, "POST", "/api/reservas", {
      materiaId: materiaBreve.id,
      fecha: dia,
      horaInicio: "11:30",
      horaFin: "11:45",
      equipoIds: [
        equipos.find((e: { identificador: number }) => e.identificador === 6).id,
      ],
      equiposATomar: [],
    })
  })

  for (const rol of ["DOCENTE", "ADMIN"] as const) {
    test(`semana legible con clases coincidentes y breves: ${rol}`, async ({ page }) => {
      await login(
        page,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_EMAIL" : "E2E_DOCENTE_EMAIL"]!,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_PASSWORD" : "E2E_DOCENTE_PASSWORD"]!
      )
      await page.setViewportSize({ width: 1280, height: 800 })
      await page.goto("/reservas/calendario")
      await page.getByLabel("Ir a fecha").fill(dia)
      await page.getByLabel("Mostrar reservas").selectOption("todas")
      await page.getByRole("button", { name: "Semana", exact: true }).click()
      const grilla = page.getByLabel("Grilla semanal de reservas, desplazable")
      const tarjetas = grilla.getByRole("button")
      await expect(tarjetas).toHaveCount(3)
      expect(
        await grilla.evaluate((el) => el.scrollWidth <= el.clientWidth + 1),
        "la semana con dos clases coincidentes debe aprovechar el espacio de escritorio"
      ).toBe(true)
      const recortes = await tarjetas.evaluateAll((botones) =>
        botones.flatMap((boton) => {
          const caja = boton.getBoundingClientRect()
          return [...boton.children]
            .filter((hijo) => {
              const r = hijo.getBoundingClientRect()
              return (
                r.top < caja.top - 0.5 ||
                r.bottom > caja.bottom + 0.5 ||
                r.right > caja.right + 0.5
              )
            })
            .map((hijo) => hijo.textContent)
        })
      )
      expect(recortes, "los campos visibles deben caber dentro de cada tarjeta").toEqual(
        []
      )
      for (const nombre of ["Programación", "Dibujo técnico", "Matemática"]) {
        const titulo = tarjetas
          .filter({ has: page.getByText(nombre, { exact: true }) })
          .getByText(nombre, { exact: true })
        expect(
          await titulo.evaluate(
            (el) =>
              el.scrollHeight <= el.clientHeight + 1 &&
              el.scrollWidth <= el.clientWidth + 1
          ),
          nombre
        ).toBe(true)
      }
      const breve = grilla.getByRole("button", { name: /Matemática,.*11:30–11:45/ })
      await breve.click()
      const detalle = page.getByRole("dialog")
      await expect(
        detalle.getByRole("heading", { name: "Matemática", exact: true })
      ).toBeVisible()
      await expect(detalle.getByRole("link", { name: /PC 6/ })).toBeVisible()
      await detalle.getByRole("button", { name: "Cerrar detalles" }).click()
      await expect(breve).toBeFocused()
      await expect(page.getByText(/Los horarios indican la duración real/)).toBeVisible()
      await page.evaluate(() => scrollTo(0, 0))
      await page.screenshot({
        path: test.info().outputPath(`semana-legible-${rol}.png`),
        fullPage: true,
      })
    })

    test(`calendario navegable y permisos de detalle: ${rol}`, async ({ page }) => {
      await login(
        page,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_EMAIL" : "E2E_DOCENTE_EMAIL"]!,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_PASSWORD" : "E2E_DOCENTE_PASSWORD"]!
      )
      await page.goto("/reservas/calendario")
      await page.getByLabel("Ir a fecha").fill(dia)
      await expect(page.getByLabel("Mostrar reservas")).toHaveValue(
        rol === "ADMIN" ? "todas" : "mias"
      )
      await page.getByLabel("Mostrar reservas").selectOption("todas")
      const dibujo = page.getByRole("button", { name: /Dibujo técnico,.*09:00/ })
      await expect(dibujo).toHaveCount(1)
      await dibujo.click()
      const dialogo = page.getByRole("dialog")
      await expect(dialogo).toBeVisible()
      await expect(dialogo.getByRole("link", { name: "Gestionar reserva" })).toHaveCount(
        rol === "ADMIN" ? 1 : 0
      )
      await dialogo.getByRole("button", { name: "Cerrar detalles" }).click()
      await expect(dialogo).toHaveCount(0)
      await page
        .getByRole("region", { name: "Calendario de reservas", exact: true })
        .screenshot({ path: test.info().outputPath(`calendario-${rol}.png`) })
      await page.getByRole("button", { name: "Mes", exact: true }).click()
      const anterior = await page.getByLabel("Ir a fecha").inputValue()
      await page.getByRole("button", { name: "Período siguiente" }).click()
      expect(await page.getByLabel("Ir a fecha").inputValue()).not.toBe(anterior)
      await page.getByRole("button", { name: "Hoy", exact: true }).click()
      await page.getByRole("button", { name: "Agenda", exact: true }).click()
      await page.setViewportSize({ width: 390, height: 844 })
      await expect
        .poll(() =>
          page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
        )
        .toBe(true)
      const axe = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze()
      expect(
        axe.violations.map((v) => ({ id: v.id, nodos: v.nodes.map((n) => n.target) }))
      ).toEqual([])
    })
  }

  test("renumerar desde la ficha conserva serie y reserva; un número ocupado se rechaza", async ({
    page,
    request,
  }) => {
    await login(page, process.env.E2E_ADMIN_EMAIL!, process.env.E2E_ADMIN_PASSWORD!)
    await page.goto("/admin/inventario")
    await page
      .getByRole("button", { name: "Gestionar equipos", exact: true })
      .first()
      .click()
    const ficha = page.locator(`#equipo-inventario-${equipo}`)
    await ficha.locator("summary").click()
    await ficha.getByRole("button", { name: "Editar", exact: true }).click()
    await ficha.getByRole("button", { name: "Cambiar número de PC", exact: true }).click()
    await ficha.getByLabel("Nuevo número de PC").fill("5")
    await ficha
      .getByLabel("Motivo del cambio de rótulo")
      .fill("Reposición del rótulo deteriorado")
    await ficha
      .getByRole("checkbox", { name: /Confirmo que es la misma computadora/ })
      .check()
    const conflicto = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/api/equipos/${equipo}/rotulo`) &&
        r.request().method() === "PATCH"
    )
    await ficha.getByRole("button", { name: "Confirmar nuevo rótulo" }).click()
    expect((await conflicto).status()).toBe(409)
    await expect(ficha.getByRole("alert")).toContainText("ya existe")
    await ficha.getByLabel("Nuevo número de PC").fill("44")
    await ficha
      .getByRole("checkbox", { name: /Confirmo que es la misma computadora/ })
      .check()
    const cambio = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/api/equipos/${equipo}/rotulo`) &&
        r.request().method() === "PATCH"
    )
    await ficha.getByRole("button", { name: "Confirmar nuevo rótulo" }).click()
    expect((await cambio).status()).toBe(204)
    await expect(ficha.locator("summary")).toHaveAttribute(
      "aria-label",
      /Gestionar PC 44/
    )
    const pc = await api(request, "GET", `/api/equipos/${equipo}`)
    expect(pc.id).toBe(equipo)
    expect(pc.numeroSerie).toBe(serie)
    const calendario = await api(
      request,
      "GET",
      `/api/calendario/reservas?desde=${dia}&hasta=${dia}&ambito=mias`,
      undefined,
      docente
    )
    const fila = calendario.data.find((r: { id: string }) => r.id === reserva)
    expect(fila.equipoId).toBe(equipo)
    expect(fila.etiqueta).toBe("PC 44")
  })
  test("una entrega fija la PC y una selección desactualizada no entrega otro equipo", async ({
    page,
    request,
  }) => {
    const materia = (await api(request, "GET", "/api/mis-materias", undefined, docente))
      .data[0]
    const pcs = (await api(request, "GET", "/api/equipos")).data
    const segunda = pcs.find((p: { identificador: number }) => p.identificador === 5)
    const tercera = pcs.find((p: { identificador: number }) => p.identificador === 6)
    const hoy = new Intl.DateTimeFormat("sv-SE", {
      timeZone: "America/Argentina/Buenos_Aires",
    }).format(new Date())
    const hora = Number(
      new Intl.DateTimeFormat("en-GB", {
        timeZone: "America/Argentina/Buenos_Aires",
        hour: "2-digit",
        hourCycle: "h23",
      }).format(new Date())
    )
    const inicio = `${String(hora).padStart(2, "0")}:00`
    const fin = `${String((hora + 1) % 24).padStart(2, "0")}:00`
    const creada = await api(
      request,
      "POST",
      "/api/reservas",
      {
        materiaId: materia.materiaId,
        fecha: hoy,
        horaInicio: inicio,
        horaFin: fin,
        equipoIds: [equipo, segunda.id],
        equiposATomar: [],
      },
      docente
    )
    const retirada = creada.reservas.find(
      (r: { equipoId: string }) => r.equipoId === equipo
    )
    const pendiente = creada.reservas.find(
      (r: { equipoId: string }) => r.equipoId === segunda.id
    )
    const entrega = await api(request, "POST", "/api/prestamos/por-reserva", {
      reservaIds: [retirada.id],
      equiposEsperados: { [retirada.id]: equipo },
    })
    const intentar = async (ruta: string, method: string, data: unknown, token: string) =>
      request.fetch(ruta, { method, headers: { Authorization: `Bearer ${token}` }, data })
    expect(
      (
        await intentar(
          `/api/reservas/${retirada.id}/equipo`,
          "PATCH",
          { equipoId: tercera.id, soloEsta: true },
          docente
        )
      ).status()
    ).toBe(409)
    await api(request, "POST", "/api/prestamos/recibir", {
      prestamoIds: [entrega.entregadas[0].id],
    })
    expect(
      (
        await intentar(
          `/api/reservas/${retirada.id}/equipo`,
          "PATCH",
          { equipoId: tercera.id, soloEsta: true },
          docente
        )
      ).status()
    ).toBe(409)
    // Renumerar conserva el UUID y sigue permitido después de una entrega.
    await api(request, "PATCH", `/api/equipos/${equipo}/rotulo`, {
      identificadorActual: 44,
      identificador: 45,
      motivo: "Renumeración con historial de entrega",
    })
    await login(page, process.env.E2E_DOCENTE_EMAIL!, process.env.E2E_DOCENTE_PASSWORD!)
    await page.goto(`/reservas?desde=${hoy}&hasta=${hoy}`)
    await expect(
      page.getByText(/Las computadoras ya entregadas conservan su equipo/)
    ).toBeVisible()
    await page.getByRole("button", { name: "Cambiar computadora", exact: true }).click()
    await expect(page.getByLabel("¿Cuál cambiás?")).toHaveValue(pendiente.id)
    await expect(page.getByLabel("¿Cuál cambiás?").getByRole("option")).toHaveCount(1)
    await page.getByLabel("¿Por cuál?").selectOption(tercera.id)
    await page.getByRole("button", { name: "Cambiar", exact: true }).click()
    await expect(page.getByText("PC 6 · Carro 1", { exact: true })).toBeVisible()
    const obsoleta = await intentar(
      "/api/prestamos/por-reserva",
      "POST",
      {
        reservaIds: [pendiente.id],
        equiposEsperados: { [pendiente.id]: segunda.id },
        liberarNoEntregadas: true,
      },
      admin
    )
    expect(obsoleta.status()).toBe(409)
    expect((await api(request, "GET", "/api/prestamos")).data).toHaveLength(0)
    const nueva = await api(request, "POST", "/api/prestamos/por-reserva", {
      reservaIds: [pendiente.id],
      equiposEsperados: { [pendiente.id]: tercera.id },
    })
    expect(nueva.entregadas[0].equipoId).toBe(tercera.id)
    const historial = (await api(request, "GET", `/api/equipos/${equipo}/prestamos`)).data
    expect(historial[0].reservaId).toBe(retirada.id)
    expect(historial[0].equipoId).toBe(equipo)
    expect(historial[0].etiqueta).toBe("PC 45")
  })
})
