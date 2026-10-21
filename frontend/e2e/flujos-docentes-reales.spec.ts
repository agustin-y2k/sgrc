import AxeBuilder from "@axe-core/playwright"
import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("confirmación de asignaciones con varios titulares y suplentes", () => {
  test.skip(
    process.env.E2E_DOCENTES_AISLADOS !== "1",
    "requiere el bloque docentes con base efímera"
  )
  test.describe.configure({ mode: "serial" })
  test.setTimeout(120_000)
  const email = process.env.E2E_ADMIN_EMAIL!
  const password = process.env.E2E_ADMIN_PASSWORD!
  let token = ""
  let materiaId = ""
  let reservaId = ""
  const docentes: { id: string; nombre: string; asignacionId: string }[] = []
  const reservasAjenas: string[] = []

  async function api(
    request: APIRequestContext,
    method: string,
    ruta: string,
    data?: unknown,
    sesion = token
  ) {
    const r = await request.fetch(ruta, {
      method,
      data,
      headers: { Authorization: `Bearer ${sesion}` },
    })
    expect(r.ok(), `${method} ${ruta}: ${r.status()}`).toBeTruthy()
    return r.headers()["content-type"]?.includes("application/json")
      ? r.json()
      : undefined
  }
  async function estado(request: APIRequestContext, id: string) {
    const reservas = await api(
      request,
      "GET",
      "/api/reservas?incluirCanceladas=true&pageSize=200"
    )
    return reservas.data.find((r: { id: string }) => r.id === id)?.estado
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["127.0.0.1", "localhost"]).toContain(new URL(baseURL!).hostname)
    const ingreso = await request.post("/api/auth/login", { data: { email, password } })
    expect(ingreso.ok()).toBeTruthy()
    token = (await ingreso.json()).token
    const ciclos = await api(request, "GET", "/api/ciclos")
    expect(ciclos.data).toHaveLength(1)
    const usuarios = await api(request, "GET", "/api/auth/usuarios?pageSize=200")
    expect(usuarios.data).toHaveLength(3)
    const ciclo = ciclos.data[0]
    const curso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 3,
      division: "2",
      modalidad: "Construcciones",
    })
    const materia = await api(request, "POST", `/api/cursos/${curso.id}/materias`, {
      nombre: "Tecnología de los Materiales",
    })
    materiaId = materia.id
    for (let i = 0; i < 4; i++) {
      const correo =
        i === 0
          ? process.env.E2E_DOCENTE_EMAIL!
          : i === 1
            ? process.env.E2E_OTRO_DOCENTE_EMAIL!
            : `asignacion-${i}@revision.test`
      // Reutilizar los dos docentes del bloque evita altas innecesarias y
      // conserva el límite real de registro del backend.
      if (i >= 2)
        await api(request, "POST", "/api/auth/registro", {
          nombre: `Docente ${i}`,
          apellido: "Prueba",
          email: correo,
          password: process.env.E2E_DOCENTE_PASSWORD!,
          cargoSolicitado: "DOCENTE",
          rolSolicitado: "SUPLENTE",
        })
      const lista = await api(request, "GET", "/api/auth/usuarios?pageSize=200")
      const u = lista.data.find((u: { email: string }) => u.email === correo)
      if (i >= 2)
        await api(request, "PATCH", `/api/auth/usuarios/${u.id}/estado`, {
          estado: "APROBADA",
        })
      const asignacion = await api(
        request,
        "POST",
        `/api/materias/${materiaId}/docentes`,
        { usuarioId: u.id, rol: i < 2 ? "TITULAR" : "SUPLENTE" }
      )
      docentes.push({
        id: u.id,
        nombre: `${u.nombre} ${u.apellido}`,
        asignacionId: asignacion.id,
      })
    }
    const equipos = (await api(request, "GET", "/api/equipos")).data
    const fecha = new Intl.DateTimeFormat("sv-SE", {
      timeZone: "America/Argentina/Buenos_Aires",
    }).format(new Date(Date.now() + 7 * 86_400_000))
    const sesionDocente = await api(request, "POST", "/api/auth/login", {
      email: process.env.E2E_DOCENTE_EMAIL!,
      password: process.env.E2E_DOCENTE_PASSWORD!,
    })
    const creada = await api(
      request,
      "POST",
      "/api/reservas",
      {
        materiaId,
        equipoIds: [equipos[0].id],
        fecha,
        horaInicio: "09:00",
        horaFin: "10:00",
      },
      sesionDocente.token
    )
    reservaId = creada.reservas[0].id
    // El mismo nombre en otra división y otro año conserva su propio alcance.
    for (const [i, anio, division] of [
      [1, 3, "3"],
      [2, 4, "2"],
    ] as const) {
      const otroCurso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
        anio,
        division,
        modalidad: "Construcciones",
      })
      const otraMateria = await api(
        request,
        "POST",
        `/api/cursos/${otroCurso.id}/materias`,
        { nombre: materia.nombre }
      )
      await api(request, "POST", `/api/materias/${otraMateria.id}/docentes`, {
        usuarioId: docentes[0].id,
        rol: "TITULAR",
      })
      const r = await api(request, "POST", "/api/reservas", {
        materiaId: otraMateria.id,
        equipoIds: [equipos[i].id],
        fecha,
        horaInicio: "09:00",
        horaFin: "10:00",
      })
      reservasAjenas.push(r.reservas[0].id)
    }
  })

  async function abrirMateria(page: Parameters<typeof login>[0]) {
    await login(page, email, password)
    await page.goto("/admin/academico")
    await page.getByRole("button", { name: "Cursos", exact: true }).click()
    const fila = page.getByText("3°2 · Construcciones", { exact: true }).locator("..")
    await fila.getByRole("button", { name: "Materias", exact: true }).click()
    await page.getByRole("button", { name: "Docentes", exact: true }).click()
  }

  test("abrir y volver no escribe; un error de envío conserva las reservas y pide confirmar otra vez", async ({
    page,
    request,
  }) => {
    await abrirMateria(page)
    const fila = page.getByText(docentes[0].nombre, { exact: true }).locator("..")
    const escrituras: string[] = []
    await page.route(`**/api/materias/${materiaId}/docentes/*`, async (route) => {
      if (route.request().method() === "DELETE") {
        escrituras.push(route.request().url())
        return route.fulfill({ status: 503, body: "No se pudo quitar la asignación" })
      }
      return route.continue()
    })
    await fila.getByRole("button", { name: "Quitar", exact: true }).click()
    const panel = page.getByRole("group", { name: "Confirmar quitar docente" })
    await expect(panel).toContainText("3°2 · Construcciones")
    await expect(panel).toContainText("Quedan 3 docentes")
    expect(escrituras).toEqual([])
    await page.setViewportSize({ width: 320, height: 844 })
    const recortes = await panel.locator("button").evaluateAll((botones) =>
      botones
        .filter((b) => {
          const r = b.getBoundingClientRect()
          return r.left < 0 || r.right > innerWidth
        })
        .map((b) => b.textContent)
    )
    expect(recortes).toEqual([])
    const axe = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
      .analyze()
    expect(axe.violations.map((v) => v.id)).toEqual([])
    await panel.getByRole("button", { name: "Volver", exact: true }).click()
    await expect(fila.getByRole("button", { name: "Quitar", exact: true })).toBeFocused()
    expect(escrituras).toEqual([])
    await fila.getByRole("button", { name: "Quitar", exact: true }).click()
    await panel
      .getByRole("button", { name: "Confirmar quitar docente", exact: true })
      .click()
    await expect(
      page.getByText("No se pudo quitar la asignación", { exact: true })
    ).toBeVisible()
    expect(escrituras).toHaveLength(1)
    await expect(panel).toHaveCount(0)
    expect(
      (await api(request, "GET", `/api/materias/${materiaId}/docentes`)).data
    ).toHaveLength(4)
    expect(await estado(request, reservaId)).toBe("CONFIRMADA")
  })

  test("dos titulares y dos suplentes: sólo la cuarta baja cancela las reservas de esa materia y curso", async ({
    page,
    request,
  }) => {
    await abrirMateria(page)
    for (let i = 0; i < docentes.length; i++) {
      const fila = page.getByText(docentes[i].nombre, { exact: true }).locator("..")
      await fila.getByRole("button", { name: "Quitar", exact: true }).click()
      const panel = page.getByRole("group", { name: "Confirmar quitar docente" })
      if (i < 3) await expect(panel).toContainText("Las reservas se mantienen")
      else await expect(panel).toContainText("se cancelarán sus reservas futuras")
      const respuesta = page.waitForResponse(
        (r) =>
          r.request().method() === "DELETE" && r.url().includes(docentes[i].asignacionId)
      )
      await panel
        .getByRole("button", { name: "Confirmar quitar docente", exact: true })
        .click()
      expect((await respuesta).ok()).toBeTruthy()
      await expect(fila).toHaveCount(0)
      expect(
        (await api(request, "GET", `/api/materias/${materiaId}/docentes`)).data
      ).toHaveLength(3 - i)
      expect(await estado(request, reservaId)).toBe(i < 3 ? "CONFIRMADA" : "CANCELADA")
      for (const id of reservasAjenas)
        expect(await estado(request, id)).toBe("CONFIRMADA")
    }
    await expect(
      page.getByRole("status").filter({ hasText: "Se cancelaron 1 reserva futura" })
    ).toBeVisible()
  })
})
