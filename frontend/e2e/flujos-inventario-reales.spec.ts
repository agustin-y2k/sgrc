import AxeBuilder from "@axe-core/playwright"
import {
  expect,
  test,
  type APIRequestContext,
  type Page,
  type Locator,
} from "@playwright/test"
import { DOCENTE_EMAIL, DOCENTE_PASSWORD, login } from "./helpers"

test.describe("inventario y licencias reales en una base efímera vacía", () => {
  test.skip(
    process.env.E2E_INVENTARIO_AISLADO !== "1",
    "requiere E2E_INVENTARIO_AISLADO=1 y base vacía"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(90_000)
  const email = process.env.E2E_ADMIN_EMAIL!,
    password = process.env.E2E_ADMIN_PASSWORD!
  let admin = "",
    docente = "",
    materia = "",
    origen = "",
    destino = "",
    vacio = "",
    pc = "",
    otro = "",
    suelto = ""
  const hoy = new Date(),
    iso = (d: Date) =>
      `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  const manana = new Date(hoy)
  manana.setDate(manana.getDate() + 1)
  const despues = new Date(manana)
  despues.setDate(despues.getDate() + 7)
  const reservas: string[] = []
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
  async function abrir(p: Page, ruta = "/admin/inventario") {
    await login(p, email, password)
    await p.goto(ruta)
  }
  function carro(p: Page, nombre: string) {
    return p.locator('[data-slot="card"]').filter({
      has: p.locator('[data-slot="card-title"]').getByText(nombre, { exact: true }),
    })
  }
  async function equipo(p: Page, id: string, nombreCarro: string) {
    const panel = carro(p, nombreCarro)
    const boton = panel.getByRole("button", { name: "Gestionar equipos", exact: true })
    await boton.click()
    const ficha = p.locator(`#equipo-inventario-${id}`)
    await expect(ficha).toBeVisible()
    await ficha.locator("summary").click()
    return ficha
  }
  async function operacion(
    p: Page,
    scope: Page | Locator,
    nombre: string,
    path: string,
    method: string
  ) {
    const res = p.waitForResponse(
      (r) => new URL(r.url()).pathname === path && r.request().method() === method
    )
    await scope.getByRole("button", { name: nombre, exact: true }).click()
    return res
  }
  async function movil(p: Page) {
    await p.setViewportSize({ width: 320, height: 780 })
    expect(
      await p.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)
    ).toBe(true)
    for (const el of await p
      .locator("main button:visible, main select:visible, main input:visible")
      .all()) {
      const b = await el.boundingBox()
      if (b) {
        expect(b.x).toBeGreaterThanOrEqual(0)
        expect(b.x + b.width).toBeLessThanOrEqual(320)
      }
    }
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
      "/api/prestamos",
      "/api/licencias",
    ])
      expect((await api(request, "GET", ruta)).data).toHaveLength(0)
    await api(request, "PUT", "/api/jornada", {
      tramos: [
        "DOMINGO",
        "LUNES",
        "MARTES",
        "MIERCOLES",
        "JUEVES",
        "VIERNES",
        "SABADO",
      ].flatMap((diaSemana) => [
        { diaSemana, horaInicio: "00:00", horaFin: "12:00" },
        { diaSemana, horaInicio: "12:00", horaFin: "00:00" },
      ]),
    })
    const ciclo = await api(request, "POST", "/api/ciclos", { anio: hoy.getFullYear() })
    const curso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 4,
      division: "A",
    })
    materia = (
      await api(request, "POST", `/api/cursos/${curso.id}/materias`, {
        nombre: "Dibujo Técnico",
      })
    ).id
    await api(
      request,
      "POST",
      "/api/auth/registro",
      {
        nombre: "Docente",
        apellido: "Inventario",
        email: DOCENTE_EMAIL,
        password: DOCENTE_PASSWORD,
        cargoSolicitado: "DOCENTE",
        rolSolicitado: "TITULAR",
      },
      ""
    )
    const u = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data.find(
      (u: { email: string }) => u.email === DOCENTE_EMAIL
    )
    await api(request, "PATCH", `/api/auth/usuarios/${u.id}/estado`, {
      estado: "APROBADA",
    })
    await api(request, "POST", `/api/materias/${materia}/docentes`, {
      usuarioId: u.id,
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
    origen = (await api(request, "POST", "/api/carros", { nombre: "Origen de revisión" }))
      .id
    destino = (
      await api(request, "POST", "/api/carros", { nombre: "Destino de revisión" })
    ).id
    pc = (
      await api(request, "POST", `/api/carros/${origen}/equipos`, {
        identificador: 1,
        numeroSerie: "INV-PRUEBA-1",
      })
    ).id
    suelto = (
      await api(request, "POST", "/api/equipos", {
        tipo: "Proyector",
        nombre: "Proyector de revisión",
        reservable: true,
        esComputadora: false,
      })
    ).id
  })

  test("alta de carro y computadora desde la interfaz", async ({ page, request }) => {
    await abrir(page)
    await page.getByRole("button", { name: "Nuevo carro", exact: true }).click()
    await page.getByLabel("Nombre", { exact: true }).fill("Carro vacío de revisión")
    expect(
      (await operacion(page, page, "Crear carro", "/api/carros", "POST")).status()
    ).toBe(201)
    vacio = (await api(request, "GET", "/api/carros")).data.find(
      (c: { nombre: string }) => c.nombre === "Carro vacío de revisión"
    ).id
    const panel = carro(page, "Origen de revisión")
    await panel.getByRole("button", { name: "Gestionar equipos", exact: true }).click()
    await panel
      .getByRole("button", { name: "Agregar un equipo al carro", exact: true })
      .click()
    await panel.getByLabel("Número de máquina", { exact: true }).fill("9")
    await panel.getByLabel("Número de serie", { exact: true }).fill("INV-PRUEBA-9")
    expect(
      (
        await operacion(
          page,
          panel,
          "Agregar al carro",
          `/api/carros/${origen}/equipos`,
          "POST"
        )
      ).status()
    ).toBe(201)
    otro = (await api(request, "GET", `/api/carros/${origen}/equipos`)).data.find(
      (e: { identificador: number }) => e.identificador === 9
    ).id
  })

  test("la prioridad masiva alcanza únicamente a los dispositivos elegidos", async ({
    page,
    request,
  }) => {
    await abrir(page)
    const panel = carro(page, "Origen de revisión")
    await panel
      .getByRole("button", { name: "Preferencia del carro", exact: true })
      .click()
    await panel.getByRole("checkbox", { name: "PC 9", exact: true }).uncheck()
    await panel.getByLabel("Alcance", { exact: true }).selectOption("GENERAL")
    await panel.getByLabel("Materia", { exact: true }).selectOption("Dibujo Técnico")
    await movil(page)
    expect(
      (await operacion(page, panel, "Marcar", "/api/preferencias", "POST")).ok()
    ).toBe(true)
    expect(
      (await api(request, "GET", `/api/equipos/${pc}/preferencias`)).data
    ).toHaveLength(1)
    expect(
      (await api(request, "GET", `/api/equipos/${otro}/preferencias`)).data
    ).toHaveLength(0)
  })

  test("la prioridad de un curso concreto respeta su materia y la API rechaza cruces inventados", async ({
    page,
    request,
  }) => {
    const ciclo = (await api(request, "GET", "/api/ciclos")).data.find(
      (c: { activo: boolean }) => c.activo
    )
    const curso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 3,
      division: "2",
      modalidad: "Construcción",
    })
    await api(request, "POST", `/api/cursos/${curso.id}/materias`, {
      nombre: "Tecnología de los Materiales",
    })
    const otroCurso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 3,
      division: "1",
      modalidad: "Electromecánica",
    })
    await api(request, "POST", `/api/cursos/${otroCurso.id}/materias`, { nombre: "TIC" })
    await abrir(page)
    const panel = carro(page, "Origen de revisión")
    await panel
      .getByRole("button", { name: "Preferencia del carro", exact: true })
      .click()
    await expect(panel.getByLabel("Materia", { exact: true })).toBeDisabled()
    await panel.getByLabel("Modalidad", { exact: true }).selectOption("Electromecánica")
    await panel.getByLabel("Año", { exact: true }).selectOption("3")
    await panel.getByLabel("División", { exact: true }).selectOption("1")
    await expect(
      panel.getByLabel("Materia", { exact: true }).locator("option")
    ).toHaveText(["Elegí una materia…", "TIC"])
    await panel.getByLabel("Modalidad", { exact: true }).selectOption("Construcción")
    await expect(panel.getByLabel("Año", { exact: true })).toHaveValue("")
    await panel.getByLabel("Año", { exact: true }).selectOption("3")
    await panel.getByLabel("División", { exact: true }).selectOption("2")
    await panel
      .getByLabel("Materia", { exact: true })
      .selectOption("Tecnología de los Materiales")
    await movil(page)
    expect(
      (await operacion(page, panel, "Marcar", "/api/preferencias", "POST")).status()
    ).toBe(201)
    const validas = (await api(request, "GET", `/api/equipos/${pc}/preferencias`)).data
    const marca = validas.find((p: { cursoExacto: boolean }) => p.cursoExacto)
    expect(marca).toMatchObject({
      materiaNombre: "Tecnología de los Materiales",
      modalidad: "Construcción",
      anio: 3,
      division: "2",
      cursoExacto: true,
    })
    for (const data of [
      { modalidad: "Electromecánica", anio: 3, division: "1" },
      { modalidad: "Construcción", anio: 4, division: "2" },
      { modalidad: "Construcción", anio: 3, division: "9" },
    ]) {
      const respuesta = await request.post("/api/preferencias", {
        headers: { Authorization: `Bearer ${admin}` },
        data: {
          equipoIds: [pc, otro],
          materiaNombre: "Tecnología de los Materiales",
          cursoExacto: true,
          ...data,
        },
      })
      expect(respuesta.status()).toBe(400)
    }
    const respuesta = await request.patch(`/api/preferencias/${marca.id}`, {
      headers: { Authorization: `Bearer ${admin}` },
      data: { anio: 3, modalidad: "Electromecánica", division: "1", cursoExacto: true },
    })
    expect(respuesta.status()).toBe(400)
    expect((await api(request, "GET", `/api/equipos/${pc}/preferencias`)).data).toEqual(
      validas
    )
    const ficha = await equipo(page, pc, "Origen de revisión")
    await ficha.getByRole("button", { name: "Preferencias", exact: true }).click()
    const filaMarca = ficha.getByRole("listitem").filter({
      hasText: "Tecnología de los Materiales de 3°2",
    })
    await filaMarca.getByRole("button", { name: "Editar", exact: true }).click()
    await expect(ficha.getByLabel("Alcance", { exact: true })).toHaveValue("CURSO")
    await expect(ficha.getByLabel("Materia", { exact: true })).toHaveValue(
      "Tecnología de los Materiales"
    )
    await ficha.getByLabel("Prioridad", { exact: true }).selectOption("2")
    expect(
      (
        await operacion(page, ficha, "Guardar", `/api/preferencias/${marca.id}`, "PATCH")
      ).ok()
    ).toBe(true)
    expect(
      (await api(request, "GET", `/api/equipos/${pc}/preferencias`)).data.find(
        (p: { id: string }) => p.id === marca.id
      )
    ).toMatchObject({ ...marca, prioridad: 2 })
    for (const equipoId of [pc, otro]) {
      const marcas = (await api(request, "GET", `/api/equipos/${equipoId}/preferencias`))
        .data
      for (const p of marcas.filter((p: { cursoExacto: boolean }) => p.cursoExacto))
        await api(request, "DELETE", `/api/preferencias/${p.id}`)
    }
  })

  test("licencias en una computadora y un equipo suelto: renovación masiva y edición conservan la fecha", async ({
    page,
    request,
  }) => {
    await abrir(page, "/admin/licencias")
    await page.getByRole("button", { name: "Cargar una licencia", exact: true }).click()
    await page.getByLabel("Software", { exact: true }).fill("Software de revisión")
    await page.getByRole("checkbox", { name: /^PC 1\b/ }).check()
    await page
      .getByRole("checkbox", { name: "Proyector de revisión", exact: true })
      .check()
    await page.getByLabel("¿Cuándo vence?", { exact: true }).selectOption("vence-el")
    await expect(
      page.getByRole("button", { name: "Cargar en 2 equipos", exact: true })
    ).toBeDisabled()
    await page.getByLabel("Fecha de vencimiento", { exact: true }).fill(iso(hoy))
    expect(
      (await operacion(page, page, "Cargar en 2 equipos", "/api/licencias", "POST")).ok()
    ).toBe(true)
    await page.getByRole("button", { name: "Cerrar", exact: true }).click()
    await expect(page.getByText("Vence hoy").first()).toBeVisible()
    for (const c of await page
      .getByRole("checkbox", { name: /^Seleccionar Software de revisión/ })
      .all())
      await c.check()
    await movil(page)
    expect(
      (
        await operacion(
          page,
          page,
          "Renovar las 2 seleccionadas",
          "/api/licencias/renovar",
          "POST"
        )
      ).ok()
    ).toBe(true)
    await expect(
      page.getByText("Se renovaron 2 licencias.", { exact: true })
    ).toBeVisible()
    const lista = (await api(request, "GET", "/api/licencias")).data
    expect(lista).toHaveLength(2)
    const licencia = lista.find((l: { equipoId: string }) => l.equipoId === pc)
    const fila = page.locator("div.rounded-md.border.p-3").filter({
      has: page.getByRole("checkbox", {
        name: /^Seleccionar Software de revisión de PC 1/,
      }),
    })
    await fila.getByRole("button", { name: "Editar", exact: true }).click()
    await expect(fila.getByLabel("Fecha de vencimiento", { exact: true })).toHaveValue(
      licencia.fechaVencimiento
    )
    await fila.getByLabel("Días que dura cada renovación", { exact: true }).fill("60")
    expect(
      (
        await operacion(page, fila, "Guardar", `/api/licencias/${licencia.id}`, "PATCH")
      ).ok()
    ).toBe(true)
    expect(await api(request, "GET", `/api/licencias/${licencia.id}`)).toMatchObject({
      diasDuracion: 60,
      fechaVencimiento: licencia.fechaVencimiento,
      ultimaRenovacion: licencia.ultimaRenovacion,
    })
  })

  test("mover un equipo conserva su préstamo, reservas, licencia y prioridad", async ({
    page,
    request,
  }) => {
    await api(
      request,
      "POST",
      "/api/reservas/recurrentes",
      {
        materiaId: materia,
        equipoIds: [pc],
        diaSemana: [
          "DOMINGO",
          "LUNES",
          "MARTES",
          "MIERCOLES",
          "JUEVES",
          "VIERNES",
          "SABADO",
        ][manana.getDay()],
        fechaInicio: iso(manana),
        fechaFin: iso(despues),
        horaInicio: "10:00",
        horaFin: "11:00",
      },
      docente
    )
    for (const r of (await api(request, "GET", "/api/reservas?pageSize=200")).data)
      reservas.push(r.id)
    expect(reservas).toHaveLength(2)
    const p = (
      await api(request, "POST", "/api/prestamos", {
        equipoIds: [pc],
        nombre: "Persona de revisión",
      })
    ).entregadas[0]
    await abrir(page)
    const ficha = await equipo(page, pc, "Origen de revisión")
    await ficha.getByRole("button", { name: "Editar", exact: true }).click()
    await ficha.getByLabel("Carro", { exact: true }).selectOption(destino)
    expect(
      (
        await operacion(page, ficha, "Guardar cambios", `/api/equipos/${pc}`, "PATCH")
      ).status()
    ).toBe(200)
    expect((await api(request, "GET", "/api/prestamos")).data).toContainEqual(
      expect.objectContaining({ id: p.id, equipoId: pc, abierto: true })
    )
    await api(request, "POST", "/api/prestamos/recibir", { prestamoIds: [p.id] })
    expect((await api(request, "GET", `/api/equipos/${pc}`)).carroId).toBe(destino)
    expect(
      (await api(request, "GET", `/api/equipos/${pc}/preferencias`)).data
    ).toHaveLength(1)
    const licencias = (await api(request, "GET", "/api/licencias")).data
    expect(
      licencias.find((l: { equipoId: string }) => l.equipoId === pc).carroNombre
    ).toBe("Destino de revisión")
    const actuales = (await api(request, "GET", "/api/reservas?pageSize=200")).data
    expect(actuales.map((r: { id: string }) => r.id).sort()).toEqual([...reservas].sort())
    expect(actuales.every((r: { estado: string }) => r.estado === "CONFIRMADA")).toBe(
      true
    )
  })

  test("mantenimiento cancela todas las fechas futuras y volver a disponible no las restaura", async ({
    page,
    request,
  }) => {
    await abrir(page)
    const ficha = await equipo(page, pc, "Destino de revisión")
    await ficha.getByRole("button", { name: "→ En mantenimiento", exact: true }).click()
    await ficha
      .getByLabel("Motivo (opcional)", { exact: true })
      .fill("Revisión de teclado")
    const res = await operacion(
      page,
      ficha,
      "Confirmar cambio",
      `/api/equipos/${pc}/estado`,
      "PATCH"
    )
    expect(await res.json()).toMatchObject({
      reservasCanceladas: 2,
      docentesNotificados: 1,
    })
    const actuales = (await api(request, "GET", "/api/reservas?pageSize=200")).data
    expect(actuales.every((r: { estado: string }) => r.estado === "CANCELADA")).toBe(true)
    await ficha.locator("summary").click()
    await ficha.getByRole("button", { name: "→ Disponible", exact: true }).click()
    expect(
      (
        await operacion(
          page,
          ficha,
          "Confirmar cambio",
          `/api/equipos/${pc}/estado`,
          "PATCH"
        )
      ).ok()
    ).toBe(true)
    expect(
      (await api(request, "GET", "/api/reservas?pageSize=200")).data.every(
        (r: { estado: string }) => r.estado === "CANCELADA"
      )
    ).toBe(true)
  })

  test("baja y reactivación respetan custodia y detectan un número de máquina reutilizado", async ({
    page,
    request,
  }) => {
    const p = (
      await api(request, "POST", "/api/prestamos", {
        equipoIds: [otro],
        nombre: "Otra persona de revisión",
      })
    ).entregadas[0]
    await abrir(page)
    const ficha = await equipo(page, otro, "Origen de revisión")
    await ficha.getByRole("button", { name: "Dar de baja", exact: true }).click()
    expect(
      (
        await operacion(page, ficha, "Confirmar baja", `/api/equipos/${otro}`, "DELETE")
      ).status()
    ).toBe(409)
    await api(request, "POST", "/api/prestamos/recibir", { prestamoIds: [p.id] })
    expect(
      (
        await operacion(page, ficha, "Confirmar baja", `/api/equipos/${otro}`, "DELETE")
      ).ok()
    ).toBe(true)
    const reemplazo = await api(request, "POST", `/api/carros/${origen}/equipos`, {
      identificador: 9,
      numeroSerie: "INV-REEMPLAZO-9",
    })
    const panel = carro(page, "Origen de revisión")
    await panel.getByRole("button", { name: /^Ver los 1 equipos dados de baja$/ }).click()
    expect(
      (
        await operacion(
          page,
          panel,
          "Devolver al inventario",
          `/api/equipos/${otro}/reactivar`,
          "POST"
        )
      ).status()
    ).toBe(409)
    await api(request, "DELETE", `/api/equipos/${reemplazo.id}`)
    expect(
      (
        await operacion(
          page,
          panel,
          "Devolver al inventario",
          `/api/equipos/${otro}/reactivar`,
          "POST"
        )
      ).ok()
    ).toBe(true)
    expect((await api(request, "GET", `/api/equipos/${otro}`)).dadoDeBaja).toBe(false)
  })

  test("quitar una licencia no cambia el equipo ni la otra licencia", async ({
    page,
    request,
  }) => {
    const lista = (await api(request, "GET", "/api/licencias")).data
    const quitar = lista.find((l: { equipoId: string }) => l.equipoId === suelto)
    await abrir(page, "/admin/licencias")
    const fila = page.locator("div.rounded-md.border.p-3").filter({
      has: page.getByRole("checkbox", {
        name: /^Seleccionar Software de revisión de Proyector/,
      }),
    })
    await fila.getByRole("button", { name: "Quitar", exact: true }).click()
    expect(
      (
        await operacion(page, fila, "Quitar", `/api/licencias/${quitar.id}`, "DELETE")
      ).ok()
    ).toBe(true)
    expect((await api(request, "GET", "/api/licencias")).data).toHaveLength(1)
    expect((await api(request, "GET", `/api/equipos/${suelto}`)).dadoDeBaja).toBe(false)
  })

  test("el docente puede consultar equipos y no puede administrar inventario, prioridades ni licencias", async ({
    page,
    request,
  }) => {
    await login(page, DOCENTE_EMAIL, DOCENTE_PASSWORD)
    for (const ruta of ["/admin/inventario", "/admin/licencias"]) {
      await page.goto(ruta)
      await expect(page).not.toHaveURL(new RegExp(`${ruta}$`))
    }
    for (const [method, ruta, data] of [
      ["PATCH", `/api/equipos/${pc}/estado`, { estado: "EN_MANTENIMIENTO" }],
      [
        "POST",
        "/api/licencias",
        { equipoIds: [pc], nombre: "Intento", diasDuracion: 30, diasAviso: 1 },
      ],
      [
        "POST",
        "/api/preferencias",
        { equipoIds: [pc], materiaNombre: "Dibujo Técnico", prioridad: 1 },
      ],
    ] as const) {
      expect(
        (
          await request.fetch(ruta, {
            method,
            headers: { Authorization: `Bearer ${docente}` },
            data,
          })
        ).status()
      ).toBe(403)
    }
    expect(
      (await api(request, "GET", "/api/equipos", undefined, docente)).data.length
    ).toBeGreaterThan(0)
    expect((await api(request, "GET", "/api/licencias")).data).toHaveLength(1)
  })

  test("el alta de licencias recupera un inventario inaccesible conservando el formulario", async ({
    page,
  }) => {
    await abrir(page, "/admin/licencias")
    let falla = true
    await page.route("**/api/equipos", (r) =>
      falla
        ? r.fulfill({ status: 503, json: { error: "Inventario de prueba inaccesible" } })
        : r.continue()
    )
    await page.getByRole("button", { name: "Cargar una licencia", exact: true }).click()
    await page.getByLabel("Software", { exact: true }).fill("Formulario conservado")
    await expect(page.getByText(/No pudimos verificar el inventario/)).toBeVisible({
      timeout: 15_000,
    })
    await expect(page.getByText(/No hay equipos cargados/)).toHaveCount(0)
    falla = false
    await page.getByRole("button", { name: "Reintentar", exact: true }).click()
    await page.getByRole("checkbox", { name: /^PC 1\b/ }).check()
    await expect(page.getByLabel("Software", { exact: true })).toHaveValue(
      "Formulario conservado"
    )
    await expect(
      page.getByRole("button", { name: "Cargar en 1 equipo", exact: true })
    ).toBeEnabled()
    await movil(page)
  })

  test("un carro ocupado no se retira y uno vacío puede retirarse y reactivarse", async ({
    page,
    request,
  }) => {
    await abrir(page)
    const ocupado = carro(page, "Destino de revisión")
    await ocupado
      .getByRole("button", { name: "Dar de baja el carro", exact: true })
      .click()
    expect(
      (
        await operacion(
          page,
          ocupado,
          "Confirmar la baja del carro",
          `/api/carros/${destino}`,
          "DELETE"
        )
      ).status()
    ).toBe(409)
    const libre = carro(page, "Carro vacío de revisión")
    await libre.getByRole("button", { name: "Dar de baja el carro", exact: true }).click()
    expect(
      (
        await operacion(
          page,
          libre,
          "Confirmar la baja del carro",
          `/api/carros/${vacio}`,
          "DELETE"
        )
      ).ok()
    ).toBe(true)
    expect((await api(request, "GET", `/api/carros/${vacio}`)).dadoDeBaja).toBe(true)
    await api(request, "POST", `/api/carros/${vacio}/reactivar`)
    expect((await api(request, "GET", `/api/carros/${vacio}`)).dadoDeBaja).toBe(false)
  })
})
