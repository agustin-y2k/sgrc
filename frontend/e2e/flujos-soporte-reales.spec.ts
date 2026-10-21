import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext, type Page } from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

test.describe("soporte y avisos reales en una base efímera vacía", () => {
  test.skip(
    process.env.E2E_SOPORTE_AISLADO !== "1",
    "requiere base efímera y E2E_SOPORTE_AISLADO=1"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(90_000)
  const email = process.env.E2E_ADMIN_EMAIL!,
    password = process.env.E2E_ADMIN_PASSWORD!
  let admin = "",
    segundoAdmin = "",
    docente = "",
    otro = "",
    hilo = "",
    avisoDocente = "",
    avisoAdmin = ""
  async function api(
    r: APIRequestContext,
    method: string,
    path: string,
    data?: unknown,
    token = admin
  ) {
    const res = await r.fetch(path, {
      method,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(res.ok(), `${method} ${path}: ${res.status()}`).toBe(true)
    return (res.headers()["content-type"] ?? "").includes("application/json")
      ? res.json()
      : undefined
  }
  async function avisos(r: APIRequestContext, token: string, estado?: string) {
    return (
      await api(
        r,
        "GET",
        `/api/notifications/${estado ? `?estado=${estado}` : ""}`,
        undefined,
        token
      )
    ).data as { id: string; hiloId?: string; estado: string; tipo: string }[]
  }
  async function abrir(
    p: Page,
    esAdmin = false,
    ruta = "/notificaciones?soporte=consultas"
  ) {
    await login(p, esAdmin ? email : DOCENTE_EMAIL, esAdmin ? password : DOCENTE_PASSWORD)
    await p.goto(ruta)
  }
  const conversacion = (p: Page) =>
    p.getByRole("group", { name: "Conversación: Ayuda con las notebooks" })
  async function movil(p: Page) {
    await p.setViewportSize({ width: 320, height: 780 })
    expect(
      await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
    ).toBe(true)
    expect(
      (
        await new AxeBuilder({ page: p })
          .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
          .analyze()
      ).violations
    ).toEqual([])
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    admin = (await api(request, "POST", "/api/auth/login", { email, password }, "")).token
    for (const ruta of [
      "/api/ciclos",
      "/api/equipos",
      "/api/sugerencias",
      "/api/notifications/",
    ])
      expect((await api(request, "GET", ruta)).data).toHaveLength(0)
    expect(
      (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data
    ).toHaveLength(1)
    await api(request, "PUT", "/api/jornada", {
      tramos: [
        "LUNES",
        "MARTES",
        "MIERCOLES",
        "JUEVES",
        "VIERNES",
        "SABADO",
        "DOMINGO",
      ].map((diaSemana) => ({ diaSemana, horaInicio: "08:00", horaFin: "18:00" })),
    })
    await api(request, "POST", "/api/auth/admins", {
      nombre: "Segunda",
      apellido: "Administración",
      email: "admin2@revision.test",
      password,
    })
    segundoAdmin = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        { email: "admin2@revision.test", password },
        ""
      )
    ).token
    for (const cuenta of [DOCENTE_EMAIL, "otra@revision.test"]) {
      await api(
        request,
        "POST",
        "/api/auth/registro",
        {
          nombre: "Docente",
          apellido: "Soporte",
          email: cuenta,
          password: DOCENTE_PASSWORD,
          cargoSolicitado: "DOCENTE",
          rolSolicitado: "TITULAR",
        },
        ""
      )
      const u = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data.find(
        (u: { email: string }) => u.email === cuenta
      )
      await api(request, "PATCH", `/api/auth/usuarios/${u.id}/estado`, {
        estado: "APROBADA",
      })
      const token = (
        await api(
          request,
          "POST",
          "/api/auth/login",
          { email: cuenta, password: DOCENTE_PASSWORD },
          ""
        )
      ).token
      if (cuenta === DOCENTE_EMAIL) docente = token
      else otro = token
    }
  })

  test("el docente conserva su pedido ante fallos y navegación; llega una vez a cada admin", async ({
    page,
    request,
  }) => {
    await abrir(page, false, "/notificaciones?soporte=nuevo")
    await page.getByLabel("¿De qué se trata?").fill("Ayuda con las notebooks")
    await page
      .getByLabel("Contalo con tus palabras")
      .fill("No encuentro cómo continuar con mi clase.")
    await page.getByRole("button", { name: "Cerrar el formulario" }).click()
    await page.getByRole("button", { name: "Pedir ayuda", exact: true }).first().click()
    await expect(page.getByLabel("¿De qué se trata?")).toHaveValue(
      "Ayuda con las notebooks"
    )
    // La entrada de la barra navega a la misma página y debe conservar lo escrito.
    await page.getByRole("button", { name: "Pedir ayuda", exact: true }).first().click()
    await expect(page.getByLabel("Contalo con tus palabras")).toHaveValue(
      "No encuentro cómo continuar con mi clase."
    )
    await page.route(/\/api\/sugerencias\/?$/, async (r) => {
      if (r.request().method() !== "POST") return r.continue()
      await r.fulfill({ status: 503, json: { error: "No se pudo guardar la consulta" } })
    })
    await page.getByRole("button", { name: "Mandar", exact: true }).click()
    await expect(page.getByText("No se pudo guardar la consulta")).toBeVisible()
    await expect(page.getByLabel("¿De qué se trata?")).toHaveValue(
      "Ayuda con las notebooks"
    )
    await page.unroute(/\/api\/sugerencias\/?$/)
    const respuesta = page.waitForResponse(
      (r) =>
        /\/api\/sugerencias\/?$/.test(new URL(r.url()).pathname) &&
        r.request().method() === "POST"
    )
    await page.getByRole("button", { name: "Mandar", exact: true }).click()
    const r = await respuesta
    expect(r.status()).toBe(201)
    hilo = (await r.json()).id
    await expect(
      page.getByText("Consulta enviada. Podés seguir la conversación acá mismo.")
    ).toBeVisible()
    await expect(conversacion(page)).toBeVisible()
    for (const token of [admin, segundoAdmin]) {
      await expect
        .poll(
          async () =>
            (await avisos(request, token, "NO_LEIDA")).filter((n) => n.hiloId === hilo)
              .length
        )
        .toBe(1)
    }
    avisoAdmin = (await avisos(request, admin)).find((n) => n.hiloId === hilo)!.id
    expect((await avisos(request, otro)).filter((n) => n.hiloId === hilo)).toHaveLength(0)
    expect(
      (await api(request, "GET", "/api/mis-sugerencias", undefined, docente)).data
    ).toHaveLength(1)
    await movil(page)
  })

  test("un admin abre el aviso y responde; se cierra el pendiente para ambos administradores", async ({
    page,
    request,
  }) => {
    await abrir(page, true)
    await page.getByRole("link", { name: "Ver conversación", exact: true }).click()
    await expect(page).toHaveURL(new RegExp(`hilo=${hilo}`))
    await expect(conversacion(page)).toBeFocused()
    await conversacion(page).getByRole("textbox").fill("Te ayudamos desde el mostrador.")
    await conversacion(page)
      .getByRole("button", { name: "Responder", exact: true })
      .click()
    await expect(
      page.getByText("Respuesta enviada: Ayuda con las notebooks.")
    ).toBeVisible()
    await expect(
      conversacion(page).getByText("Respuesta enviada", { exact: true })
    ).toBeVisible()
    for (const token of [admin, segundoAdmin])
      await expect
        .poll(
          async () =>
            (await avisos(request, token, "NO_LEIDA")).filter((n) => n.hiloId === hilo)
              .length
        )
        .toBe(0)
    await expect
      .poll(
        async () =>
          (await avisos(request, docente)).filter((n) => n.hiloId === hilo).length
      )
      .toBe(1)
    avisoDocente = (await avisos(request, docente)).find((n) => n.hiloId === hilo)!.id
    expect(
      (await api(request, "GET", `/api/sugerencias/${hilo}`, undefined, docente)).estado
    ).toBe("ABIERTA")
    await movil(page)
  })

  test("los avisos se leen y borran sin cerrar ni borrar la conversación", async ({
    page,
    request,
  }) => {
    await abrir(page)
    const card = page
      .locator('[data-slot="card"]')
      .filter({ has: page.getByRole("link", { name: "Ver conversación", exact: true }) })
      .last()
    await expect(card.getByRole("button", { name: "Borrar", exact: true })).toHaveCount(0)
    const noLeida = await request.delete(`/api/notifications/${avisoDocente}`, {
      headers: { Authorization: `Bearer ${docente}` },
    })
    expect(noLeida.status()).toBe(409)
    await page.getByRole("button", { name: "Marcar todas como leídas" }).click()
    await expect(page.getByText("Avisos marcados como leídos: 1.")).toBeVisible()
    await expect(page.getByText("No tenés avisos sin leer.")).toBeVisible()
    await page.getByLabel("Mostrar también las leídas").check()
    await page.getByRole("button", { name: "Borrar todos los avisos leídos" }).click()
    await expect(page.getByText("Avisos leídos borrados: 1.")).toBeVisible()
    expect(await avisos(request, docente)).toHaveLength(0)
    expect(
      (await api(request, "GET", `/api/sugerencias/${hilo}`, undefined, docente)).mensajes
    ).toHaveLength(2)
  })

  test("el admin resuelve el hilo mediante una acción separada", async ({
    page,
    request,
  }) => {
    await abrir(page, true, `/notificaciones?soporte=consultas&hilo=${hilo}`)
    await conversacion(page).getByRole("button", { name: "Dar por resuelta" }).click()
    await expect(
      page.getByText("Conversación resuelta: Ayuda con las notebooks.")
    ).toBeVisible()
    await expect(conversacion(page).getByText("Resuelta", { exact: true })).toBeVisible()
    expect(
      (await api(request, "GET", `/api/sugerencias/${hilo}`, undefined, docente)).estado
    ).toBe("RESUELTA")
  })

  test("el docente reabre el tema resuelto y sus seguimientos no duplican pendientes", async ({
    page,
    request,
  }) => {
    await abrir(page, false, `/notificaciones?soporte=consultas&hilo=${hilo}`)
    await expect(
      page.getByText("Al enviar un mensaje se vuelve a abrir la conversación.")
    ).toBeVisible()
    await conversacion(page)
      .getByRole("textbox")
      .fill("Necesito otra indicación para avanzar.")
    await conversacion(page)
      .getByRole("button", { name: "Responder", exact: true })
      .click()
    await expect(
      page.getByText("Respuesta enviada: Ayuda con las notebooks.")
    ).toBeVisible()
    await expect(conversacion(page).getByText("Esperando respuesta")).toBeVisible()
    await api(
      request,
      "POST",
      `/api/sugerencias/${hilo}/mensajes`,
      { texto: "Sumo un detalle: https://escuela.test/" + "detalle".repeat(80) },
      docente
    )
    for (const token of [admin, segundoAdmin])
      await expect
        .poll(
          async () =>
            (await avisos(request, token, "NO_LEIDA")).filter((n) => n.hiloId === hilo)
              .length
        )
        .toBe(1)
    expect(
      (await api(request, "GET", `/api/sugerencias/${hilo}`, undefined, docente)).mensajes
    ).toHaveLength(4)
    await page.reload()
    await expect(conversacion(page)).toBeVisible()
    await movil(page)
  })

  test("otra cuenta no lee ni responde el hilo, ni modifica avisos ajenos", async ({
    page,
    request,
  }) => {
    for (const [method, path, data] of [
      ["GET", `/api/sugerencias/${hilo}`, undefined],
      ["POST", `/api/sugerencias/${hilo}/mensajes`, { texto: "No debe guardarse" }],
      ["POST", `/api/sugerencias/${hilo}/resolver`, undefined],
      ["PATCH", `/api/notifications/${avisoAdmin}/leida`, undefined],
      ["DELETE", `/api/notifications/${avisoAdmin}`, undefined],
    ] as const) {
      const r = await request.fetch(path, {
        method,
        data,
        headers: { Authorization: `Bearer ${otro}` },
      })
      expect(r.status()).toBe(403)
    }
    await login(page, "otra@revision.test", DOCENTE_PASSWORD)
    await page.goto(`/notificaciones?soporte=consultas&hilo=${hilo}`)
    await expect(
      page.getByRole("button", { name: "Reintentar conversación" })
    ).toBeVisible()
    await expect(page.getByText("Ayuda con las notebooks", { exact: true })).toHaveCount(
      0
    )
    expect(
      (await api(request, "GET", "/api/mis-sugerencias", undefined, otro)).data
    ).toHaveLength(0)
    expect(
      (await api(request, "GET", `/api/sugerencias/${hilo}`, undefined, docente)).mensajes
    ).toHaveLength(4)
  })

  test("las lecturas fallidas se recuperan conservando los borradores", async ({
    page,
  }) => {
    await abrir(page)
    await conversacion(page).getByRole("textbox").fill("Borrador ante corte de conexión")
    let fallar = true
    await page.route("**/api/mis-sugerencias*", (r) =>
      fallar
        ? r.fulfill({ status: 503, json: { error: "No se pudo consultar el buzón" } })
        : r.continue()
    )
    // Simula volver a una pestaña visible: el cliente renueva el listado.
    await page.evaluate(() => window.dispatchEvent(new Event("visibilitychange")))
    await expect(page.getByRole("button", { name: "Reintentar consultas" })).toBeVisible({
      timeout: 20_000,
    })
    fallar = false
    await page.getByRole("button", { name: "Reintentar consultas" }).click()
    await expect(conversacion(page).getByRole("textbox")).toHaveValue(
      "Borrador ante corte de conexión"
    )
  })

  test("las preferencias guardadas corresponden al rol y conservan las categorías obligatorias", async ({
    page,
    request,
  }) => {
    await abrir(page)
    await page.getByRole("button", { name: "Elegir cuáles" }).click()
    const prefs = (
      await api(
        request,
        "GET",
        "/api/notifications/preferencias-email",
        undefined,
        docente
      )
    ).data as { categoria: string; fija: boolean; activa: boolean; grupo: string }[]
    expect(prefs.some((p) => p.grupo === "ADMINISTRACION")).toBe(false)
    const opciones = page.getByRole("checkbox").filter({ visible: true })
    expect(await opciones.count()).toBeGreaterThan(1)
    const editable = page.locator('[role="checkbox"]:visible:not([disabled])').first()
    await editable.click()
    await page.getByRole("button", { name: "Guardar", exact: true }).click()
    await expect(page.getByText("Preferencias guardadas.")).toBeVisible()
    const guardadas = (
      await api(
        request,
        "GET",
        "/api/notifications/preferencias-email",
        undefined,
        docente
      )
    ).data as typeof prefs
    expect(guardadas.filter((p) => p.fija).every((p) => p.activa)).toBe(true)
    expect(guardadas.filter((p) => p.activa && !p.fija).length).toBe(
      prefs.filter((p) => p.activa && !p.fija).length +
        (prefs.find((p) => !p.fija)!.activa ? -1 : 1)
    )
    await movil(page)
  })
})
