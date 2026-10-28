import { expect, test, type APIRequestContext } from "@playwright/test"
import { login } from "./helpers"

test.describe("prioridad y reservas que ya tuvieron entrega", () => {
  test.skip(
    process.env.E2E_PRIORIDAD_CUSTODIA_AISLADA !== "1",
    "requiere cuentas ficticias y una base efímera"
  )
  test.describe.configure({ mode: "serial" })
  let admin = "",
    docente = "",
    materia = "",
    fecha = ""
  const equipos: string[] = []
  const reservas: string[] = []
  async function api(
    request: APIRequestContext,
    metodo: string,
    ruta: string,
    data?: unknown,
    token = admin
  ) {
    const r = await request.fetch(ruta, {
      method: metodo,
      headers: { Authorization: `Bearer ${token}` },
      data,
    })
    expect(r.ok(), `${metodo} ${ruta}: ${r.status()}`).toBe(true)
    return r.headers()["content-type"]?.includes("application/json")
      ? r.json()
      : undefined
  }
  test.beforeAll(async ({ request, baseURL }) => {
    expect(["localhost", "127.0.0.1"]).toContain(new URL(baseURL!).hostname)
    admin = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        {
          email: process.env.E2E_ADMIN_EMAIL,
          password: process.env.E2E_ADMIN_PASSWORD,
        },
        ""
      )
    ).token
    const titular = (
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
    docente = (
      await api(
        request,
        "POST",
        "/api/auth/login",
        {
          email: process.env.E2E_OTRO_DOCENTE_EMAIL,
          password: process.env.E2E_OTRO_DOCENTE_PASSWORD,
        },
        ""
      )
    ).token
    const ciclo = (await api(request, "GET", "/api/ciclos")).data[0]
    const curso = await api(request, "POST", `/api/ciclos/${ciclo.id}/cursos`, {
      anio: 3,
      division: "91",
    })
    materia = (
      await api(request, "POST", `/api/cursos/${curso.id}/materias`, {
        nombre: "Dibujo Técnico de custodia",
      })
    ).id
    const usuarios = (await api(request, "GET", "/api/auth/usuarios?pageSize=200")).data
    const otro = usuarios.find(
      (u: { email: string }) => u.email === process.env.E2E_OTRO_DOCENTE_EMAIL
    )
    await api(request, "POST", `/api/materias/${materia}/docentes`, {
      usuarioId: otro.id,
      rol: "TITULAR",
    })
    const propia = (await api(request, "GET", "/api/mis-materias", undefined, titular))
      .data[0]
    const inventario = (await api(request, "GET", "/api/equipos")).data
    for (const numero of [1, 2])
      equipos.push(
        inventario.find((e: { identificador: number }) => e.identificador === numero).id
      )
    await api(request, "POST", "/api/preferencias", {
      equipoIds: equipos,
      materiaNombre: "Dibujo Técnico de custodia",
      prioridad: 1,
      anio: 3,
      division: "91",
      cursoExacto: true,
    })
    fecha = new Intl.DateTimeFormat("sv-SE", {
      timeZone: "America/Argentina/Buenos_Aires",
    }).format(new Date())
    // La entrega es del día y su fin exclusivo es la medianoche siguiente.
    // La anticipación exacta y la concurrencia se comprueban con reloj fijo en Go.
    const creada = await api(
      request,
      "POST",
      "/api/reservas",
      {
        materiaId: propia.materiaId,
        fecha,
        horaInicio: "23:00",
        horaFin: "00:00",
        equipoIds: equipos,
      },
      titular
    )
    for (const equipo of equipos)
      reservas.push(
        creada.reservas.find((r: { equipoId: string }) => r.equipoId === equipo).id
      )
    const entrega = await api(request, "POST", "/api/prestamos/por-reserva", {
      reservaIds: reservas,
      equiposEsperados: Object.fromEntries(reservas.map((id, i) => [id, equipos[i]])),
    })
    expect(entrega.entregadas).toHaveLength(2)
    const devuelta = entrega.entregadas.find(
      (p: { equipoId: string }) => p.equipoId === equipos[1]
    )
    await api(request, "POST", "/api/prestamos/recibir", { prestamoIds: [devuelta.id] })
    expect((await api(request, "GET", "/api/prestamos")).data).toHaveLength(1)
  })
  for (const rol of ["ADMIN", "DOCENTE"] as const) {
    test(`${rol}: disponibilidad, formulario y API protegen préstamos abiertos y devueltos`, async ({
      page,
      request,
    }) => {
      const token = rol === "ADMIN" ? admin : docente
      const params = new URLSearchParams({
        fecha,
        horaInicio: "23:00",
        horaFin: "00:00",
        materiaId: materia,
      })
      const disponibilidad = await api(
        request,
        "GET",
        `/api/equipos-disponibles?${params}`,
        undefined,
        token
      )
      for (const equipo of equipos) {
        expect(
          disponibilidad.data.some((e: { equipoId: string }) => e.equipoId === equipo)
        ).toBe(false)
        expect(
          disponibilidad.ocupados.find((e: { equipoId: string }) => e.equipoId === equipo)
        ).toMatchObject({ puedeTomarse: false })
      }
      await login(
        page,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_EMAIL" : "E2E_OTRO_DOCENTE_EMAIL"]!,
        process.env[rol === "ADMIN" ? "E2E_ADMIN_PASSWORD" : "E2E_OTRO_DOCENTE_PASSWORD"]!
      )
      await page.goto("/reservas/nueva")
      await page.getByLabel("Materia", { exact: true }).selectOption(materia)
      await page.getByLabel("Fecha", { exact: true }).fill(fecha)
      await page.getByLabel("Hora de inicio: hora", { exact: true }).selectOption("23")
      await page.getByLabel("Hora de inicio: minutos", { exact: true }).selectOption("00")
      await page.getByLabel("Hora de fin: hora", { exact: true }).selectOption("00")
      await page.getByLabel("Hora de fin: minutos", { exact: true }).selectOption("00")
      await expect(
        page.getByText("Ya reservados en esa franja", { exact: true })
      ).toBeVisible()
      for (const numero of [1, 2])
        await expect(
          page.getByRole("checkbox", { name: `PC ${numero} (Carro 1)`, exact: true })
        ).toHaveCount(0)
      const rechazo = await request.post("/api/reservas", {
        headers: { Authorization: `Bearer ${token}` },
        data: {
          materiaId: materia,
          fecha,
          horaInicio: "23:00",
          horaFin: "00:00",
          equipoIds: equipos,
          equiposATomar: equipos,
        },
      })
      expect(rechazo.status()).toBe(409)
      for (const [i, reserva] of reservas.entries())
        expect(await api(request, "GET", `/api/reservas/${reserva}`)).toMatchObject({
          equipoId: equipos[i],
          estado: "CONFIRMADA",
        })
      expect((await api(request, "GET", "/api/prestamos")).data).toHaveLength(1)
    })
  }
})
