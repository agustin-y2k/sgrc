import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

// El cierre modifica todo el ciclo y el reset modifica cuentas de prueba.
// Estos escenarios sólo se activan explícitamente contra un entorno aislado.
test.describe("revisión de flujos sobre datos ficticios", () => {
  test.skip(
    process.env.E2E_REVISION_AISLADA !== "1",
    "requiere E2E_REVISION_AISLADA=1 y una base temporal"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(120_000)

  const adminEmail = process.env.E2E_ADMIN_EMAIL ?? ""
  const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? ""
  let tokenAdmin = ""
  let docenteId = ""
  let equipoId = ""
  let cicloId = ""
  let anio = 0

  async function api(
    request: APIRequestContext,
    metodo: string,
    ruta: string,
    datos?: unknown
  ) {
    const respuesta = await request.fetch(ruta, {
      method: metodo,
      headers: { Authorization: `Bearer ${tokenAdmin}` },
      data: datos,
    })
    expect(respuesta.ok(), `${metodo} ${ruta}: ${respuesta.status()}`).toBeTruthy()
    if (!(respuesta.headers()["content-type"] ?? "").includes("application/json"))
      return undefined
    return respuesta.json()
  }

  test.beforeAll(async ({ request, baseURL }) => {
    const host = new URL(baseURL!).hostname
    expect(
      ["127.0.0.1", "localhost"],
      "la revisión destructiva sólo admite localhost"
    ).toContain(host)
    const ingreso = await request.post("/api/auth/login", {
      data: { email: adminEmail, password: adminPassword },
    })
    expect(ingreso.ok()).toBeTruthy()
    tokenAdmin = (await ingreso.json()).token
    const usuarios = await api(
      request,
      "GET",
      "/api/auth/usuarios?estado=APROBADA&pageSize=200"
    )
    docenteId = usuarios.data.find((u: { email: string }) => u.email === DOCENTE_EMAIL).id
    const equipos = await api(request, "GET", "/api/equipos")
    equipoId = equipos.data[0].id
    const ciclos = await api(request, "GET", "/api/ciclos")
    const activo = ciclos.data.find((c: { activo: boolean }) => c.activo)
    cicloId = activo.id
    anio = activo.anio
  })

  test("pantallas principales y secundarias: API, accesibilidad, teclado y ancho móvil", async ({
    page,
  }) => {
    const errores: string[] = []
    page.on("pageerror", (error) => errores.push(error.message))
    const rechazos: string[] = []
    page.on("response", (response) => {
      const ruta = new URL(response.url()).pathname
      if (
        ruta.startsWith("/api/") &&
        response.status() >= 400 &&
        !(response.status() === 404 && ruta.endsWith("/foto"))
      )
        rechazos.push(`${response.status()} ${ruta}`)
    })
    await login(page, adminEmail, adminPassword)
    for (const ruta of [
      "/",
      "/perfil",
      "/reservas",
      "/reservas/nueva",
      "/inventario",
      "/disponibilidad",
      "/notificaciones",
      "/admin/aprobacion",
      "/admin/usuarios",
      "/admin/academico",
      "/admin/inventario",
      "/admin/licencias",
      "/admin/entregas",
      "/admin/reportes",
      "/admin/jornada",
      "/admin/auditoria",
      "/admin/pedidos-de-materia",
      "/admin/bloquear-equipos",
      `/inventario/equipos/${equipoId}/calendario`,
    ]) {
      await page.goto(ruta)
      await expect(page).toHaveURL(
        new RegExp(ruta.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "$")
      )
      await expect(page.locator("main")).toBeVisible()
      await page.waitForLoadState("networkidle")
      await expect(page.locator("main h1")).toBeVisible()
      const resultados = await new AxeBuilder({ page })
        .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
        .analyze()
      expect
        .soft(
          resultados.violations.map((v) => ({
            id: v.id,
            nodos: v.nodes.map((n) => n.target),
          })),
          `accesibilidad ${ruta}`
        )
        .toEqual([])
      await page.setViewportSize({ width: 390, height: 844 })
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth + 1
        ),
        `ancho ${ruta}`
      ).toBeTruthy()
      await page.setViewportSize({ width: 1280, height: 800 })
    }
    await page.goto("/login")
    await page.getByRole("button", { name: "Salir", exact: true }).click()
    await expect(page).toHaveURL(/\/login$/)
    await page.getByLabel("Email", { exact: true }).focus()
    await page.keyboard.press("Tab")
    await expect(page.getByLabel("Contraseña", { exact: true })).toBeFocused()
    expect(errores).toEqual([])
    expect(rechazos).toEqual([])
  })

  test("un docente no abre rutas administrativas aunque navegue desde una sesión admin", async ({
    page,
  }) => {
    await login(page, adminEmail, adminPassword)
    await page.goto("/admin/usuarios")
    await expect(page.getByText(adminEmail, { exact: true }).first()).toBeVisible()
    await page.getByRole("button", { name: "Salir", exact: true }).click()
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    for (const ruta of [
      "/admin/usuarios",
      "/admin/entregas",
      "/admin/academico",
      "/admin/auditoria",
    ]) {
      await page.goto(ruta)
      await expect(page).toHaveURL(/\/$/)
      await expect(page.locator("main")).toBeVisible()
    }
  })

  test("entrega y devolución guardan las observaciones y cierran el préstamo", async ({
    page,
    request,
  }) => {
    await login(page, adminEmail, adminPassword)
    await page.goto("/admin/entregas")
    await page.getByRole("button", { name: "Entregar sin reserva", exact: true }).click()
    await page.getByLabel("¿A quién?", { exact: true }).fill("Persona de revisión")
    await page
      .getByRole("checkbox", { name: /PC 1\b/ })
      .first()
      .check()
    const entregada = page.waitForResponse(
      (r) => r.url().endsWith("/api/prestamos") && r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Entregar 1 equipo", exact: true }).click()
    expect((await entregada).ok()).toBeTruthy()
    await expect(page.getByText("Persona de revisión", { exact: true })).toBeVisible()
    const prestamos = await api(request, "GET", "/api/prestamos")
    const prestamo = prestamos.data.find(
      (p: { entregadoANombre: string }) => p.entregadoANombre === "Persona de revisión"
    )
    expect(prestamo).toBeTruthy()
    await page
      .getByRole("checkbox", { name: /PC 1\b/ })
      .first()
      .check()
    await page
      .getByLabel("Observaciones (opcional)", { exact: true })
      .fill("Volvió con cargador; revisión E2E")
    const recibida = page.waitForResponse((r) =>
      r.url().endsWith("/api/prestamos/recibir")
    )
    await page
      .getByRole("button", { name: "Recibir 1 equipo seleccionado", exact: true })
      .click()
    expect((await recibida).ok()).toBeTruthy()
    await expect(page.getByText("Persona de revisión", { exact: true })).not.toBeVisible()
    const historial = await api(
      request,
      "GET",
      `/api/equipos/${prestamo.equipoId}/prestamos`
    )
    const cerrado = historial.data.find((p: { id: string }) => p.id === prestamo.id)
    expect(cerrado.devueltoEn).toBeTruthy()
    expect(cerrado.observaciones).toBe("Volvió con cargador; revisión E2E")
  })

  test("soporte: conversación privada, respuesta, cierre y reapertura", async ({
    page,
    request,
  }) => {
    const asunto = `Consulta de revisión E2E ${Date.now()}`
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await page.goto("/notificaciones")
    await page
      .getByRole("main")
      .getByRole("button", { name: "Pedir ayuda", exact: true })
      .click()
    await page.getByLabel("¿De qué se trata?", { exact: true }).fill(asunto)
    await page
      .getByLabel("Contalo con tus palabras", { exact: true })
      .fill("Necesito ayuda con la revisión")
    const alta = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname.replace(/\/$/, "") === "/api/sugerencias" &&
        r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Mandar", exact: true }).click()
    const respuestaAlta = await alta
    expect(respuestaAlta.ok()).toBeTruthy()
    const hilo = await respuestaAlta.json()
    await expect(page.getByText(asunto, { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "Salir", exact: true }).click()
    await login(page, adminEmail, adminPassword)
    await page.goto("/notificaciones")
    await page.getByRole("button", { name: "Ver conversaciones", exact: true }).click()
    const hiloAdmin = page.getByRole("group", {
      name: `Conversación: ${asunto}`,
      exact: true,
    })
    if (!(await hiloAdmin.getByRole("textbox").isVisible()))
      await hiloAdmin.getByRole("button", { name: "Ver", exact: true }).click()
    await hiloAdmin.getByRole("textbox").fill("Respuesta de revisión")
    const respondida = page.waitForResponse((r) =>
      r.url().endsWith(`/api/sugerencias/${hilo.id}/mensajes`)
    )
    await hiloAdmin.getByRole("button", { name: "Responder", exact: true }).click()
    expect((await respondida).ok()).toBeTruthy()
    const resuelta = page.waitForResponse((r) =>
      r.url().endsWith(`/api/sugerencias/${hilo.id}/resolver`)
    )
    await hiloAdmin.getByRole("button", { name: "Dar por resuelta", exact: true }).click()
    expect((await resuelta).ok()).toBeTruthy()
    await page.getByRole("button", { name: "Salir", exact: true }).click()
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    await page.goto("/notificaciones")
    await page.getByRole("button", { name: "Ver conversaciones", exact: true }).click()
    const hiloDocente = page.getByRole("group", {
      name: `Conversación: ${asunto}`,
      exact: true,
    })
    if (!(await hiloDocente.getByRole("textbox").isVisible()))
      await hiloDocente.getByRole("button", { name: "Ver", exact: true }).click()
    await expect(
      hiloDocente.getByText("Respuesta de revisión", { exact: true })
    ).toBeVisible()
    await hiloDocente.getByRole("textbox").fill("Consulta adicional")
    const seguimiento = page.waitForResponse((r) =>
      r.url().endsWith(`/api/sugerencias/${hilo.id}/mensajes`)
    )
    await hiloDocente.getByRole("button", { name: "Responder", exact: true }).click()
    expect((await seguimiento).ok()).toBeTruthy()
    const listado = await api(request, "GET", "/api/sugerencias?abiertas=true")
    expect(listado.data.find((s: { id: string }) => s.id === hilo.id).estado).toBe(
      "ABIERTA"
    )
  })

  test("licencias: alta y renovación efectiva en el equipo seleccionado", async ({
    page,
    request,
  }) => {
    const nombreSoftware = `Software revisión E2E ${Date.now()}`
    await login(page, adminEmail, adminPassword)
    await page.goto("/admin/licencias")
    await page.getByRole("button", { name: "Cargar una licencia", exact: true }).click()
    await page.getByLabel("Software", { exact: true }).fill(nombreSoftware)
    await page
      .getByRole("checkbox", { name: /PC 1\b/ })
      .first()
      .check()
    await page.getByLabel("¿Cuándo vence?", { exact: true }).selectOption("quedan-dias")
    await page.getByLabel("Días que le quedan", { exact: true }).fill("2")
    const alta = page.waitForResponse(
      (r) => r.url().endsWith("/api/licencias") && r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Cargar en 1 equipo", exact: true }).click()
    expect((await alta).ok()).toBeTruthy()
    let listado = await api(request, "GET", "/api/licencias")
    const creada = listado.data.find(
      (l: { nombre: string }) => l.nombre === nombreSoftware
    )
    expect(creada).toBeTruthy()
    await page
      .getByRole("checkbox", { name: new RegExp(`Seleccionar ${nombreSoftware}`) })
      .check()
    const renovada = page.waitForResponse((r) =>
      r.url().endsWith("/api/licencias/renovar")
    )
    await page
      .getByRole("button", { name: "Renovar las 1 seleccionadas", exact: true })
      .click()
    expect((await renovada).ok()).toBeTruthy()
    listado = await api(request, "GET", "/api/licencias")
    const nueva = listado.data.find((l: { id: string }) => l.id === creada.id)
    expect(nueva.fechaVencimiento > creada.fechaVencimiento).toBeTruthy()
    expect(nueva.estado).toBe("VIGENTE")
    expect(
      (
        await new AxeBuilder({ page })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
  })

  test("contraseña temporal bloquea los demás flujos y el cambio emite una sesión utilizable", async ({
    page,
    request,
  }) => {
    const reset = await api(
      request,
      "POST",
      `/api/auth/usuarios/${docenteId}/reset-password`
    )
    await login(page, DOCENTE_EMAIL, reset.passwordTemporal)
    await expect(page).toHaveURL(/\/cambiar-password$/)
    await page.goto("/reservas/nueva")
    await expect(page).toHaveURL(/\/cambiar-password$/)
    await page
      .getByLabel("Contraseña actual", { exact: true })
      .fill(reset.passwordTemporal)
    await page
      .getByLabel("Contraseña nueva", { exact: true })
      .fill("DocenteRevisionNueva12345")
    const cambio = page.waitForResponse((r) =>
      r.url().endsWith("/api/auth/cambiar-password")
    )
    await page.getByRole("button", { name: "Cambiar contraseña", exact: true }).click()
    expect((await cambio).ok()).toBeTruthy()
    await expect(page).toHaveURL(/\/$/)
    await page.goto("/reservas/nueva")
    await expect(
      page.getByRole("heading", { name: "Nueva reserva", exact: true })
    ).toBeVisible()
  })

  test("cierre anual desde la pantalla conserva el histórico y abre un ciclo distinto", async ({
    page,
    request,
  }) => {
    await login(page, adminEmail, adminPassword)
    await page.goto("/admin/academico")
    await page
      .getByRole("button", { name: "Cerrar el año y abrir el siguiente", exact: true })
      .click()
    await page.getByLabel(/Año del ciclo siguiente/).fill(String(anio + 1))
    const cerrada = page.waitForResponse((r) =>
      r.url().endsWith(`/api/ciclos/${cicloId}/archivar`)
    )
    await page
      .getByRole("button", { name: `Cerrar ${anio} y crear ${anio + 1}`, exact: true })
      .click()
    expect((await cerrada).ok()).toBeTruthy()
    const ciclos = await api(request, "GET", "/api/ciclos")
    expect(ciclos.data.find((c: { id: string }) => c.id === cicloId).archivado).toBe(true)
    const nuevo = ciclos.data.find((c: { activo: boolean }) => c.activo)
    expect(nuevo.anio).toBe(anio + 1)
    expect(nuevo.id).not.toBe(cicloId)
    await page.goto("/admin/reportes")
    await expect(page.locator("main")).toBeVisible()
  })
})
