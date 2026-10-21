import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext, type Page } from "@playwright/test"
import { login } from "./helpers"

test.describe("regresiones de la revisión de vistas RV-01 a RV-09", () => {
  test.skip(process.env.E2E_REVISION_AISLADA !== "1", "requiere una base efímera")
  test.describe.configure({ mode: "default" })
  test.setTimeout(120_000)

  const email = process.env.E2E_ADMIN_EMAIL!
  const password = process.env.E2E_ADMIN_PASSWORD!
  let token = ""
  let cicloId = ""
  let equipoId = ""
  let docenteId = ""

  async function api(
    request: APIRequestContext,
    method: string,
    ruta: string,
    data?: unknown
  ) {
    const r = await request.fetch(ruta, {
      method,
      data,
      headers: { Authorization: `Bearer ${token}` },
    })
    expect(r.ok(), `${method} ${ruta}: ${r.status()}`).toBeTruthy()
    return r.headers()["content-type"]?.includes("application/json")
      ? r.json()
      : undefined
  }

  async function sinRecortes(page: Page) {
    const recortados = await page.locator("main").evaluate((main) => {
      const fallos: string[] = []
      for (const el of main.querySelectorAll<HTMLElement>("button, input, select, a")) {
        const rect = el.getBoundingClientRect()
        if (!rect.width || !rect.height || getComputedStyle(el).visibility === "hidden")
          continue
        let izquierda = 0,
          derecha = window.innerWidth
        for (let p = el.parentElement; p; p = p.parentElement) {
          if (
            ["hidden", "clip", "auto", "scroll"].includes(getComputedStyle(p).overflowX)
          ) {
            const r = p.getBoundingClientRect()
            izquierda = Math.max(izquierda, r.left)
            derecha = Math.min(derecha, r.right)
          }
        }
        if (rect.left < izquierda - 1 || rect.right > derecha + 1)
          fallos.push(
            `${el.tagName} ${el.textContent?.trim() || el.id}: ${rect.left}..${rect.right}, área ${izquierda}..${derecha}`
          )
      }
      return fallos
    })
    expect(
      recortados,
      "controles dentro del ancho visible y de sus contenedores"
    ).toEqual([])
  }

  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    const ingreso = await request.post("/api/auth/login", { data: { email, password } })
    expect(ingreso.ok()).toBeTruthy()
    token = (await ingreso.json()).token
    const ciclos = await api(request, "GET", "/api/ciclos")
    cicloId = ciclos.data.find((c: { activo: boolean }) => c.activo).id
    equipoId = (await api(request, "GET", "/api/equipos")).data[0].id
    docenteId = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data.find(
      (u: { email: string }) => u.email === process.env.E2E_OTRO_DOCENTE_EMAIL
    ).id
    const cursos = await api(request, "GET", `/api/ciclos/${cicloId}/cursos`)
    if (
      !cursos.data.some(
        (c: { anio: number; division: string }) => c.anio === 2 && c.division === "B"
      )
    )
      await api(request, "POST", `/api/ciclos/${cicloId}/cursos`, {
        anio: 2,
        division: "B",
      })
    const espacios = await api(request, "GET", `/api/ciclos/${cicloId}/espacios`)
    if (
      !espacios.data.some(
        (e: { nombre: string }) => e.nombre === "Biblioteca y apoyo escolar"
      )
    )
      await api(request, "POST", `/api/ciclos/${cicloId}/espacios`, {
        nombre: "Biblioteca y apoyo escolar",
      })
  })

  test("RV-01: una jornada desconocida no habilita reemplazos y puede recuperarse", async ({
    page,
  }) => {
    await login(page, email, password)
    let fallar = true
    const escrituras: string[] = []
    await page.route("**/api/jornada", async (route) => {
      if (route.request().method() !== "GET") {
        escrituras.push(route.request().method())
        return route.abort()
      }
      if (fallar)
        return route.fulfill({ status: 503, json: { error: "Fallo de prueba" } })
      return route.continue()
    })
    await page.goto("/admin/jornada")
    await expect(page.getByRole("button", { name: "Agregar tramo" })).toBeDisabled()
    await expect(page.getByText(/No pudimos consultar la jornada/)).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByText(/Todavía no se declaró la jornada/)).toHaveCount(0)
    expect(escrituras).toEqual([])
    fallar = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await expect(page.getByRole("button", { name: /^Editar / }).first()).toBeVisible()
    expect(escrituras).toEqual([])
  })

  test("RV-02: cursos, materias, importación y espacios no recortan controles a 320 px", async ({
    page,
  }) => {
    await login(page, email, password)
    await page.goto("/admin/academico")
    await page.getByRole("button", { name: "Cursos", exact: true }).click()
    await page.getByRole("button", { name: "Materias", exact: true }).first().click()
    await page.getByRole("button", { name: "Docentes", exact: true }).click()
    for (const width of [320, 390, 768, 1280]) {
      await page.setViewportSize({ width, height: 844 })
      await sinRecortes(page)
    }
    await page.setViewportSize({ width: 320, height: 844 })
    for (const [accion, salida] of [
      ["Renombrar", "Cancelar"],
      ["Eliminar", "Volver"],
    ]) {
      const botones = page.getByRole("button", { name: accion, exact: true })
      const cantidad = await botones.count()
      for (let i = 0; i < cantidad; i++) {
        await botones.nth(i).click()
        await sinRecortes(page)
        await page.getByRole("button", { name: salida, exact: true }).click()
      }
    }
    await page.getByLabel("Cargar desde una planilla").setInputFiles({
      name: "estructura-de-prueba.csv",
      mimeType: "text/csv",
      buffer: Buffer.from("Año;Modalidad;División;Materias\n3°;;C;Robótica"),
    })
    await expect(page.getByRole("button", { name: "Cargar en el ciclo" })).toBeVisible()
    await sinRecortes(page)
    await page
      .getByRole("button", { name: "Copiar estas materias a otros cursos" })
      .click()
    await sinRecortes(page)
    await page.waitForLoadState("networkidle")
    await page.evaluate(() =>
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
        .forEach((a) => a.finish())
    )
    await page.screenshot({
      path: test.info().outputPath("academico-320.png"),
      fullPage: true,
    })
    const axe = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
      .analyze()
    expect(
      axe.violations.map((v) => ({ id: v.id, nodos: v.nodes.map((n) => n.target) }))
    ).toEqual([])
  })

  test("RV-05: errores de lectura ofrecen reintento y no anuncian listas vacías", async ({
    page,
  }) => {
    await login(page, email, password)
    for (const caso of [
      {
        ruta: "/admin/usuarios",
        api: "/api/auth/usuarios",
        vacio: "No hay usuarios con ese filtro.",
        mensaje: "No pudimos consultar los usuarios.",
      },
      {
        ruta: "/admin/aprobacion",
        api: "/api/auth/usuarios",
        vacio: "No hay cuentas pendientes.",
        mensaje: "No pudimos consultar las cuentas pendientes.",
      },
      {
        ruta: "/admin/pedidos-de-materia",
        api: "/api/pedidos-de-materia",
        vacio: "No hay pedidos sin resolver.",
        mensaje: "No pudimos consultar los pedidos de materia.",
      },
      {
        ruta: "/disponibilidad",
        api: "/api/guardia",
        vacio: "No hay ningún Admin aprobado.",
        mensaje: "No pudimos consultar los horarios de atención.",
      },
    ]) {
      const coincide = (url: URL) => url.pathname === caso.api
      await page.route(coincide, (r) =>
        r.fulfill({ status: 503, json: { error: "Fallo de prueba" } })
      )
      await page.goto(caso.ruta)
      await expect(page.getByText(caso.mensaje, { exact: false })).toBeVisible({
        timeout: 15_000,
      })
      await expect(page.getByText(caso.vacio, { exact: true })).toHaveCount(0)
      await page.unroute(coincide)
      await page.getByRole("button", { name: "Reintentar", exact: true }).click()
      await expect(page.getByText(caso.mensaje, { exact: false })).toHaveCount(0)
    }
  })

  test("RV-07 y RV-08: lista de impacto con teclado y restauración de foco del menú", async ({
    page,
  }) => {
    await login(page, email, password)
    const disparador = page.getByRole("button", { name: "Administración", exact: true })
    await disparador.click()
    await page.getByRole("link", { name: "Académico", exact: true }).focus()
    await page.keyboard.press("Escape")
    await expect(disparador).toBeFocused()
    await expect(disparador).toHaveAttribute("aria-expanded", "false")
    await page.route("**/api/jornada", (route) => {
      if (route.request().method() === "GET") return route.continue()
      return route.fulfill({
        status: 409,
        json: {
          error: "Impacto de prueba",
          impacto: {
            reservas: Array.from({ length: 25 }, (_, i) => ({
              id: String(i),
              fecha: "2026-10-20",
              horaInicio: "09:00",
              horaFin: "10:00",
              equipo: `PC ${i}`,
              materia: "Programación",
              docente: "Ada Lovelace",
            })),
            prestamos: [],
            totalAfectadas: 25,
            clasesAfectadas: 25,
            totalDeReservas: 30,
            totalDeClases: 30,
          },
        },
      })
    })
    await page.goto("/admin/jornada")
    await page
      .getByRole("button", { name: /^Quitar / })
      .first()
      .click()
    const lista = page.getByRole("region", {
      name: "Clases afectadas por el cambio de jornada",
    })
    await lista.focus()
    await expect(lista).toBeFocused()
    await page.keyboard.press("PageDown")
    await expect.poll(() => lista.evaluate((el) => el.scrollTop)).toBeGreaterThan(0)
    const axe = await new AxeBuilder({ page })
      .withRules(["scrollable-region-focusable"])
      .analyze()
    expect(axe.violations).toEqual([])
  })

  test("RV-09: reintentar cursos conserva la respuesta y exige una selección vigente", async ({
    page,
  }) => {
    await login(page, email, password)
    await page.route("**/api/pedidos-de-materia?*", (r) =>
      r.fulfill({
        json: {
          data: [
            {
              id: "pedido-de-prueba",
              usuarioId: docenteId,
              esMateriaNueva: true,
              materiaSolicitada: "Robótica",
              cursoSolicitado: "1°A",
              motivo: "Nuevo taller",
              estado: "PENDIENTE",
              creadoEn: new Date().toISOString(),
            },
          ],
        },
      })
    )
    await page.route(`**/api/ciclos/${cicloId}/cursos`, (r) =>
      r.fulfill({ status: 503, json: { error: "Fallo de prueba" } })
    )
    await page.goto("/admin/pedidos-de-materia")
    await page.getByRole("button", { name: "Aprobar", exact: true }).click()
    await page.getByLabel("Algo para decirle (opcional)").fill("Conservar esta respuesta")
    await expect(page.getByText(/No pudimos consultar los cursos/)).toBeVisible({
      timeout: 15_000,
    })
    await expect(
      page.getByRole("button", { name: "Confirmar y habilitar" })
    ).toBeDisabled()
    await page.unroute(`**/api/ciclos/${cicloId}/cursos`)
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    const cursos = page.getByLabel("¿En qué curso se crea?")
    await expect(cursos).toBeEnabled()
    await expect(page.getByLabel("Algo para decirle (opcional)")).toHaveValue(
      "Conservar esta respuesta"
    )
    await expect(
      page.getByRole("button", { name: "Confirmar y habilitar" })
    ).toBeDisabled()
    await cursos.selectOption({ index: 1 })
    await expect(
      page.getByRole("button", { name: "Confirmar y habilitar" })
    ).toBeEnabled()
  })

  test("RV-04: quitar una cuenta de equipo permite cancelar antes de borrar", async ({
    page,
    request,
  }) => {
    const cuenta = await api(request, "POST", `/api/equipos/${equipoId}/cuentas`, {
      usuario: "cuenta-rv",
      clase: "Docente",
      privilegio: "COMUN",
      tienePassword: false,
      visibilidad: "PUBLICA",
    })
    await login(page, email, password)
    await page.goto("/inventario")
    await page.getByRole("button", { name: "Ver equipos", exact: true }).click()
    const fila = page
      .getByRole("row")
      .filter({
        has: page.locator(`a[href="/inventario/equipos/${equipoId}/calendario"]`),
      })
    await fila.getByRole("button", { name: "Cómo entrar", exact: true }).click()
    await page.getByRole("button", { name: "Quitar", exact: true }).click()
    await expect(
      page.getByText(/No se elimina la cuenta del sistema operativo/)
    ).toBeVisible()
    await page.getByRole("button", { name: "Cancelar", exact: true }).click()
    expect(
      (await api(request, "GET", `/api/equipos/${equipoId}/cuentas`)).data.some(
        (c: { id: string }) => c.id === cuenta.id
      )
    ).toBe(true)
    await page.getByRole("button", { name: "Quitar", exact: true }).click()
    const borrada = page.waitForResponse(
      (r) =>
        r.request().method() === "DELETE" && r.url().endsWith(`/api/cuentas/${cuenta.id}`)
    )
    await page.getByRole("button", { name: "Confirmar eliminación", exact: true }).click()
    expect((await borrada).ok()).toBeTruthy()
    await expect(page.getByText("cuenta-rv", { exact: true })).toHaveCount(0)
  })

  test("RV-03 y RV-06: restablecer confirma una sola vez y guía el cambio temporal", async ({
    page,
    browser,
  }) => {
    await login(page, email, password)
    await page.goto("/admin/usuarios")
    await expect(page.getByRole("link", { name: "Cambiar mi contraseña" })).toBeVisible()
    const fila = page
      .locator('[data-slot="card"]')
      .filter({ hasText: process.env.E2E_OTRO_DOCENTE_EMAIL! })
    await fila.getByRole("button", { name: "Restablecer contraseña" }).click()
    await expect(fila.getByText(/cierra sus sesiones actuales/)).toBeVisible()
    await fila.getByRole("button", { name: "Volver", exact: true }).click()
    await fila.getByRole("button", { name: "Restablecer contraseña" }).click()
    let llamadas = 0
    page.on("request", (r) => {
      if (
        r.method() === "POST" &&
        r.url().endsWith(`/api/auth/usuarios/${docenteId}/reset-password`)
      )
        llamadas++
    })
    const respuesta = page.waitForResponse((r) =>
      r.url().endsWith(`/api/auth/usuarios/${docenteId}/reset-password`)
    )
    await fila.getByRole("button", { name: "Confirmar", exact: true }).dblclick()
    const temporal = (await (await respuesta).json()).passwordTemporal
    expect(llamadas).toBe(1)
    const contexto = await browser.newContext({
      baseURL: new URL(page.url()).origin,
      locale: "es-AR",
      timezoneId: "America/Argentina/Buenos_Aires",
    })
    try {
      const docente = await contexto.newPage()
      const prohibidas: string[] = []
      docente.on("response", (r) => {
        if (r.status() === 403) prohibidas.push(new URL(r.url()).pathname)
      })
      await docente.goto("/login")
      await docente
        .getByLabel("Email", { exact: true })
        .fill(process.env.E2E_OTRO_DOCENTE_EMAIL!)
      await docente.getByLabel("Contraseña", { exact: true }).fill(temporal)
      await docente.getByRole("button", { name: /iniciar sesión/i }).click()
      await expect(docente).toHaveURL(/\/cambiar-password$/)
      await expect(docente.getByText(/Tu contraseña es temporal/)).toBeVisible()
      await expect(docente.getByRole("navigation")).toHaveCount(0)
      await expect(
        docente.getByRole("button", { name: "Cerrar sesión", exact: true })
      ).toBeVisible()
      await docente.getByLabel("Contraseña actual", { exact: true }).fill(temporal)
      await docente
        .getByLabel("Contraseña nueva", { exact: true })
        .fill("PersonalDePrueba2026!")
      await docente
        .getByRole("button", { name: "Cambiar contraseña", exact: true })
        .click()
      await expect(docente).toHaveURL(/\/$/)
      expect(prohibidas).toEqual([])
    } finally {
      await contexto.close()
    }
  })
})
