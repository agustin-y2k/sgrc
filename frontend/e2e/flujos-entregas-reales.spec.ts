import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext, type Page } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

test.describe("entregas reales sobre una base vacía y aislada", () => {
  test.skip(
    process.env.E2E_ENTREGAS_AISLADAS !== "1",
    "requiere base efímera vacía y E2E_ENTREGAS_AISLADAS=1"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(90_000)
  let admin = "",
    docente = "",
    docenteId = "",
    materia = ""
  const equipos: string[] = []
  const dias = ["DOMINGO", "LUNES", "MARTES", "MIERCOLES", "JUEVES", "VIERNES", "SABADO"]
  const ahora = new Date()
  const iso = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  const hoy = iso(ahora)
  const siguiente = new Date(ahora)
  siguiente.setDate(siguiente.getDate() + 7)
  const minuto = Math.min(
    23 * 60,
    ahora.getHours() * 60 + Math.floor(ahora.getMinutes() / 5) * 5
  )
  const hhmm = (min: number) =>
    `${String(Math.floor((min % 1440) / 60)).padStart(2, "0")}:${String(min % 60).padStart(2, "0")}`
  const horario = { horaInicio: hhmm(minuto), horaFin: hhmm(minuto + 60) }
  const adminEmail = process.env.E2E_ADMIN_EMAIL!,
    adminPassword = process.env.E2E_ADMIN_PASSWORD!
  async function api(
    request: APIRequestContext,
    method: string,
    ruta: string,
    data?: unknown,
    token = admin
  ) {
    const res = await request.fetch(ruta, {
      method,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(res.ok(), `${method} ${ruta}: ${res.status()}`).toBe(true)
    return (res.headers()["content-type"] ?? "").includes("application/json")
      ? res.json()
      : undefined
  }
  async function abrir(page: Page) {
    await login(page, adminEmail, adminPassword)
    await page.goto("/admin/entregas")
  }
  async function operacion(page: Page, ruta: string, nombre: string) {
    const respuesta = page.waitForResponse(
      (r) => new URL(r.url()).pathname === ruta && r.request().method() === "POST"
    )
    await page.getByRole("button", { name: nombre, exact: true }).click()
    const res = await respuesta
    expect(res.ok()).toBe(true)
    return res.json()
  }
  async function abiertos(request: APIRequestContext) {
    return (await api(request, "GET", "/api/prestamos")).data
  }
  async function revisarEnMovil(page: Page) {
    await page.setViewportSize({ width: 320, height: 780 })
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
    ).toBe(true)
    for (const elemento of await page
      .locator('main button:visible, main input:visible, main [role="alert"]:visible')
      .all()) {
      const limites = await elemento.boundingBox()
      if (limites) {
        expect(limites.x).toBeGreaterThanOrEqual(0)
        expect(limites.x + limites.width).toBeLessThanOrEqual(320)
      }
    }
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
  }
  const historia = async (request: APIRequestContext, eq: number) =>
    (await api(request, "GET", `/api/equipos/${equipos[eq]}/prestamos`)).data

  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    admin = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        { email: adminEmail, password: adminPassword },
        ""
      )
    ).token
    expect((await api(request, "GET", "/api/ciclos")).data).toHaveLength(0)
    expect((await api(request, "GET", "/api/equipos")).data).toHaveLength(0)
    expect(await abiertos(request)).toHaveLength(0)
    await api(request, "PUT", "/api/jornada", {
      tramos: dias.flatMap((diaSemana) => [
        { diaSemana, horaInicio: "00:00", horaFin: "12:00" },
        { diaSemana, horaInicio: "12:00", horaFin: "00:00" },
      ]),
    })
    const ciclo = await api(request, "POST", "/api/ciclos", { anio: ahora.getFullYear() })
    const curso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 1,
      division: "A",
    })
    materia = (
      await api(request, "POST", `/api/cursos/${curso.id}/materias`, {
        nombre: "Taller de revisión",
      })
    ).id
    await api(
      request,
      "POST",
      "/api/auth/registro",
      {
        nombre: "Docente",
        apellido: "Revisión",
        email: DOCENTE_EMAIL,
        password: DOCENTE_PASSWORD,
        cargoSolicitado: "DOCENTE",
        rolSolicitado: "TITULAR",
      },
      ""
    )
    docenteId = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data.find(
      (u: { email: string }) => u.email === DOCENTE_EMAIL
    ).id
    await api(request, "PATCH", `/api/auth/usuarios/${docenteId}/estado`, {
      estado: "APROBADA",
    })
    await api(request, "POST", `/api/materias/${materia}/docentes`, {
      usuarioId: docenteId,
      rol: "TITULAR",
    })
    docente = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        { email: DOCENTE_EMAIL, password: DOCENTE_PASSWORD },
        ""
      )
    ).token
    const carro = await api(request, "POST", "/api/carros", {
      nombre: "Carro de revisión",
    })
    for (let i = 1; i <= 9; i++)
      equipos.push(
        (
          await api(request, "POST", `/api/carros/${carro.id}/equipos`, {
            identificador: i,
            numeroSerie: `REVISION-${i}`,
          })
        ).id
      )
  })

  test("entrega parcial conserva responsable y retirante, libera el resto y respeta la fecha siguiente", async ({
    page,
    request,
  }) => {
    await api(
      request,
      "POST",
      "/api/reservas/recurrentes",
      {
        materiaId: materia,
        diaSemana: dias[ahora.getDay()],
        fechaInicio: hoy,
        fechaFin: iso(siguiente),
        ...horario,
        equipoIds: equipos.slice(0, 3),
      },
      docente
    )
    await abrir(page)
    const panel = page
      .locator('[data-slot="card"]')
      .filter({ hasText: "Entregar las de una reserva" })
    await panel.getByRole("checkbox", { name: /^PC 1\b/ }).check()
    await panel.getByRole("checkbox", { name: /^PC 2\b/ }).check()
    await panel.getByLabel("¿Quién las retira? (opcional)").fill("Estudiante de prueba")
    await panel.getByRole("checkbox", { name: /Es todo lo que se llevó/ }).check()
    await revisarEnMovil(page)
    const resultado = await operacion(
      page,
      "/api/prestamos/por-reserva",
      "Entregar 2 equipos y liberar el resto"
    )
    expect(resultado.entregadas).toHaveLength(2)
    expect(resultado.liberadas).toHaveLength(1)
    for (const p of resultado.entregadas)
      expect(p).toMatchObject({
        entregadoAUsuarioId: docenteId,
        entregadoANombre: "Docente Revisión",
        retiradoPor: "Estudiante de prueba",
        abierto: true,
      })
    await expect(page.getByText(/Quedó libre 1 equipo/)).toBeVisible()
    const actuales = (
      await api(request, "GET", `/api/reservas?desde=${hoy}&hasta=${hoy}&pageSize=200`)
    ).data
    expect(
      actuales.find((r: { equipoId: string }) => r.equipoId === equipos[2]).estado
    ).toBe("NO_RETIRADA")
    const futuras = (
      await api(
        request,
        "GET",
        `/api/reservas?desde=${iso(siguiente)}&hasta=${iso(siguiente)}&pageSize=200`
      )
    ).data
    expect(futuras).toHaveLength(3)
    expect(futuras.every((r: { estado: string }) => r.estado === "CONFIRMADA")).toBe(true)
    await panel.getByRole("checkbox", { name: /^PC 3\b/ }).check()
    await expect(
      panel.getByRole("checkbox", { name: /Es todo lo que se llevó/ })
    ).not.toBeChecked()
  })

  test("devolución parcial guarda la observación sólo en el equipo recibido", async ({
    page,
    request,
  }) => {
    await abrir(page)
    await page.getByRole("checkbox", { name: /^Seleccionar PC 1\b/ }).check()
    await page.getByLabel("Observaciones (opcional)").fill("Volvió sin cargador")
    await revisarEnMovil(page)
    const resultado = await operacion(
      page,
      "/api/prestamos/recibir",
      "Recibir 1 equipo seleccionado"
    )
    expect(resultado.recibidos).toHaveLength(1)
    expect(
      (await abiertos(request)).map((p: { equipoId: string }) => p.equipoId)
    ).toEqual([equipos[1]])
    expect((await historia(request, 0))[0]).toMatchObject({
      abierto: false,
      observaciones: "Volvió sin cargador",
      retiradoPor: "Estudiante de prueba",
    })
    await operacion(
      page,
      "/api/prestamos/recibir",
      "Recibir 1 equipo de Docente Revisión"
    )
    expect(await abiertos(request)).toHaveLength(0)
    expect((await historia(request, 1))[0].observaciones).toBeUndefined()
  })

  test("entrega sin reserva a una persona sin cuenta conserva destino y omite una devolución no pactada", async ({
    page,
    request,
  }) => {
    await abrir(page)
    await page.getByRole("button", { name: "Entregar sin reserva", exact: true }).click()
    await page.getByLabel("¿A quién?", { exact: true }).fill("Personal sin cuenta")
    await page.getByLabel(/A dónde va/).fill("Biblioteca")
    for (const n of [4, 5])
      await page.getByRole("checkbox", { name: new RegExp(`^PC ${n}\\b`) }).check()
    const resultado = await operacion(page, "/api/prestamos", "Entregar 2 equipos")
    expect(resultado.entregadas).toHaveLength(2)
    for (const p of resultado.entregadas) {
      expect(p).toMatchObject({
        entregadoANombre: "Personal sin cuenta",
        destino: "Biblioteca",
      })
      expect(p.entregadoAUsuarioId).toBeUndefined()
      expect(p.devolucionEstimada).toBeUndefined()
    }
    await expect(page.getByLabel("¿A quién?", { exact: true })).toHaveCount(0)
    expect(
      (await abiertos(request)).filter(
        (p: { entregadoANombre: string }) => p.entregadoANombre === "Personal sin cuenta"
      )
    ).toHaveLength(2)
  })

  test("si otro mostrador se adelanta y no sale ningún equipo se conserva el formulario", async ({
    page,
    request,
  }) => {
    await abrir(page)
    await page.getByRole("button", { name: "Entregar sin reserva", exact: true }).click()
    await page.getByLabel("¿A quién?", { exact: true }).fill("Persona que espera")
    await page.getByRole("checkbox", { name: /^PC 6\b/ }).check()
    await api(request, "POST", "/api/prestamos", {
      equipoIds: [equipos[5]],
      nombre: "Otro mostrador",
    })
    const resultado = await operacion(page, "/api/prestamos", "Entregar 1 equipo")
    expect(resultado.entregadas).toHaveLength(0)
    expect(resultado.noEntregadas).toHaveLength(1)
    await expect(page.getByLabel("¿A quién?", { exact: true })).toHaveValue(
      "Persona que espera"
    )
    await expect(
      page.getByRole("button", { name: "Entregar 1 equipo", exact: true })
    ).toBeDisabled()
    expect(
      (await abiertos(request)).filter(
        (p: { equipoId: string }) => p.equipoId === equipos[5]
      )
    ).toHaveLength(1)
  })

  test("un lote con un equipo ya prestado entrega el otro sin duplicar custodia", async ({
    page,
    request,
  }) => {
    await abrir(page)
    await page.getByRole("button", { name: "Entregar sin reserva", exact: true }).click()
    await page.getByLabel("¿A quién?", { exact: true }).fill("Retiro por tandas")
    for (const n of [7, 8])
      await page.getByRole("checkbox", { name: new RegExp(`^PC ${n}\\b`) }).check()
    await api(request, "POST", "/api/prestamos", {
      equipoIds: [equipos[6]],
      nombre: "Otra tanda",
    })
    const resultado = await operacion(page, "/api/prestamos", "Entregar 2 equipos")
    expect(resultado.entregadas).toHaveLength(1)
    expect(resultado.noEntregadas).toHaveLength(1)
    expect(resultado.entregadas[0].equipoId).toBe(equipos[7])
    const lista = await abiertos(request)
    for (const eq of equipos.slice(6, 8))
      expect(lista.filter((p: { equipoId: string }) => p.equipoId === eq)).toHaveLength(1)
    await expect(page.getByLabel("¿A quién?", { exact: true })).toHaveCount(0)
  })

  test("reparación exige destino y la devolución no cambia automáticamente el estado de mantenimiento", async ({
    page,
    request,
  }) => {
    await api(request, "PATCH", `/api/equipos/${equipos[8]}/estado`, {
      estado: "EN_MANTENIMIENTO",
    })
    await abrir(page)
    await page.getByRole("button", { name: "Entregar sin reserva", exact: true }).click()
    await expect(page.getByRole("checkbox", { name: /^PC 9\b/ })).toHaveCount(0)
    await page.getByRole("button", { name: "Cerrar", exact: true }).click()
    await page
      .getByRole("button", { name: "Sacar un equipo a reparación", exact: true })
      .click()
    await page.getByLabel("¿Quién se lo lleva?").fill("Service de revisión")
    await page.getByRole("checkbox", { name: /^PC 9\b/ }).check()
    await expect(
      page.getByRole("button", { name: "Registrar la salida de 1 equipo" })
    ).toBeDisabled()
    await page.getByLabel("¿A dónde va y por qué?").fill("Service: revisar pantalla")
    await revisarEnMovil(page)
    const resultado = await operacion(
      page,
      "/api/prestamos",
      "Registrar la salida de 1 equipo"
    )
    expect(resultado.entregadas[0]).toMatchObject({
      equipoId: equipos[8],
      destino: "Service: revisar pantalla",
    })
    await operacion(
      page,
      "/api/prestamos/recibir",
      "Recibir 1 equipo de Service de revisión"
    )
    expect((await api(request, "GET", `/api/equipos/${equipos[8]}`)).estado).toBe(
      "EN_MANTENIMIENTO"
    )
    expect((await historia(request, 8))[0].abierto).toBe(false)
  })

  test("docente no puede entregar ni recibir mediante interfaz o API", async ({
    page,
    request,
  }) => {
    const antes = await abiertos(request)
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await page.goto("/admin/entregas")
    await expect(page).not.toHaveURL(/\/admin\/entregas$/)
    for (const [ruta, datos] of [
      ["/api/prestamos", { equipoIds: [equipos[0]], nombre: "Intento docente" }],
      ["/api/prestamos/recibir", { prestamoIds: [antes[0].id] }],
    ] as const) {
      const res = await request.post(ruta, {
        headers: { Authorization: `Bearer ${docente}` },
        data: datos,
      })
      expect(res.status()).toBe(403)
    }
    expect(await abiertos(request)).toHaveLength(antes.length)
  })

  test("inventario inaccesible no habilita entregas ni borra el nombre al reintentar", async ({
    page,
  }) => {
    await abrir(page)
    let falla = true
    await page.route("**/api/equipos", (r) =>
      falla ? r.fulfill({ status: 503, body: "Inventario inaccesible" }) : r.continue()
    )
    await page.getByRole("button", { name: "Entregar sin reserva", exact: true }).click()
    await page.getByLabel("¿A quién?", { exact: true }).fill("Nombre conservado")
    await expect(
      page.getByText(/No pudimos verificar qué equipos se pueden entregar/)
    ).toBeVisible({ timeout: 15000 })
    await expect(
      page.getByRole("button", { name: "Entregar", exact: true })
    ).toBeDisabled()
    await expect(page.getByText(/No hay equipos disponibles para entregar/)).toHaveCount(
      0
    )
    falla = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await page.getByRole("checkbox", { name: /^PC 3\b/ }).check()
    await expect(page.getByLabel("¿A quién?", { exact: true })).toHaveValue(
      "Nombre conservado"
    )
    await expect(
      page.getByRole("button", { name: "Entregar 1 equipo", exact: true })
    ).toBeEnabled()
  })
})
