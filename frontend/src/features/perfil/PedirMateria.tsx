import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select } from "@/components/ui/select"
import * as academicoApi from "@/features/academico/api"
import * as perfilApi from "@/features/perfil/api"
import { getErrorMessage } from "@/lib/api-client"

/** Pedir dictar una materia más. */
export function PedirMateria({ onListo }: { onListo?: () => void }) {
  const qc = useQueryClient()
  const [modo, setModo] = useState<"lista" | "nueva">("lista")
  const [materiaId, setMateriaId] = useState("")
  const [materia, setMateria] = useState("")
  const [curso, setCurso] = useState("")
  const [motivo, setMotivo] = useState("")
  const [error, setError] = useState("")
  const [listo, setListo] = useState(false)

  // Las materias de todo el ciclo activo, no solo las propias: justamente se
  // está pidiendo una que no se dicta.
  const { data: ciclos } = useQuery({
    queryKey: ["ciclos"],
    queryFn: academicoApi.listarCiclos,
  })
  const cicloActivo = ciclos?.data.find((c) => c.activo)
  const { data: cursos } = useQuery({
    queryKey: ["cursos", cicloActivo?.id],
    queryFn: () => academicoApi.listarCursos(cicloActivo!.id),
    enabled: !!cicloActivo,
  })

  const pedir = useMutation({
    mutationFn: () =>
      modo === "lista"
        ? perfilApi.pedirMateriaExistente(materiaId, motivo)
        : perfilApi.pedirMateriaNueva(materia, curso, motivo),
    onSuccess: () => {
      setError("")
      setListo(true)
      setMateriaId("")
      setMateria("")
      setCurso("")
      setMotivo("")
      qc.invalidateQueries({ queryKey: ["perfil", "mis-pedidos"] })
      onListo?.()
    },
    onError: (e) => setError(getErrorMessage(e)),
  })

  const faltante =
    modo === "lista"
      ? materiaId === ""
        ? "elegir la materia"
        : ""
      : materia.trim() === ""
        ? "escribir cómo se llama la materia"
        : ""
  const faltaMotivo = motivo.trim() === "" ? "explicar por qué la pedís" : ""
  const pendientes = [faltante, faltaMotivo].filter(Boolean)

  return (
    <div className="bg-superficie grid gap-3 rounded-xl border p-4">
      <div>
        <p className="font-medium">Pedir otra materia</p>
        <p className="text-muted-foreground text-sm">
          Lo resuelve el equipo de administración, no es automático. Si la materia ya la
          da otro docente, también le avisamos a esa persona.
        </p>
      </div>

      {listo && (
        <Alert>
          <AlertDescription>
            Listo, tu pedido quedó registrado. Te vamos a avisar acá y por correo cuando
            lo resuelvan.
          </AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant={modo === "lista" ? "default" : "outline"}
          size="sm"
          className="h-11 px-4 sm:h-9"
          onClick={() => setModo("lista")}
        >
          Está en la lista
        </Button>
        <Button
          type="button"
          variant={modo === "nueva" ? "default" : "outline"}
          size="sm"
          className="h-11 px-4 sm:h-9"
          onClick={() => setModo("nueva")}
        >
          Todavía no está cargada
        </Button>
      </div>

      {modo === "lista" ? (
        <MateriasDelCiclo
          cursos={cursos?.data ?? []}
          valor={materiaId}
          alElegir={setMateriaId}
        />
      ) : (
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="materia-nueva">¿Cómo se llama la materia?</Label>
            <Input
              id="materia-nueva"
              value={materia}
              onChange={(e) => setMateria(e.target.value)}
              placeholder="Ej.: Robótica"
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="curso-nuevo">¿De qué curso?</Label>
            <Input
              id="curso-nuevo"
              value={curso}
              onChange={(e) => setCurso(e.target.value)}
              placeholder="Ej.: 5°B"
            />
          </div>
        </div>
      )}

      <div className="grid gap-1.5">
        <Label htmlFor="motivo-pedido">¿Por qué la pedís?</Label>
        <Input
          id="motivo-pedido"
          value={motivo}
          onChange={(e) => setMotivo(e.target.value)}
          placeholder="Ej.: Me asignaron el segundo turno desde mayo"
        />
        <p className="text-muted-foreground text-sm">
          Es lo que va a leer quien lo resuelva antes de hablar con vos.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {pendientes.length > 0 && (
        <p className="text-muted-foreground text-sm">
          Para mandarlo falta {pendientes.join(" y ")}.
        </p>
      )}

      <div>
        <Button
          type="button"
          size="sm"
          className="h-11 px-4 sm:h-9"
          disabled={pendientes.length > 0 || pedir.isPending}
          onClick={() => pedir.mutate()}
        >
          {pedir.isPending ? "Mandando…" : "Mandar el pedido"}
        </Button>
      </div>
    </div>
  )
}

/** El selector de materia, agrupado por curso. */
function MateriasDelCiclo({
  cursos,
  valor,
  alElegir,
}: {
  cursos: { id: string; nombre: string }[]
  valor: string
  alElegir: (id: string) => void
}) {
  const consultas = useQuery({
    queryKey: ["materias-de-todos-los-cursos", cursos.map((c) => c.id)],
    queryFn: async () => {
      const porCurso = await Promise.all(
        cursos.map(async (c) => ({
          curso: c,
          materias: (await academicoApi.listarMaterias(c.id)).data,
        }))
      )
      return porCurso
    },
    enabled: cursos.length > 0,
  })

  return (
    <div className="grid gap-1.5">
      <Label htmlFor="materia-existente">¿Cuál materia?</Label>
      <Select
        id="materia-existente"
        value={valor}
        onChange={(e) => alElegir(e.target.value)}
      >
        <option value="">Elegí una materia…</option>
        {(consultas.data ?? []).map(({ curso, materias }) => (
          <optgroup key={curso.id} label={curso.nombre}>
            {materias.map((m) => (
              <option key={m.id} value={m.id}>
                {m.nombre}
              </option>
            ))}
          </optgroup>
        ))}
      </Select>
    </div>
  )
}
