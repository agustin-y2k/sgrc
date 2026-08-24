import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import type { QueryClient } from "@tanstack/react-query"

import { EstadoBadge } from "@/components/EstadoBadge"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useAuth } from "@/features/auth/AuthContext"
import * as inventoryApi from "@/features/inventory/api"
import type {
  CuentaDeEquipo,
  Equipo,
  PrivilegioDeCuenta,
  VisibilidadDeCuenta,
} from "@/features/inventory/types"
import { getErrorMessage } from "@/lib/api-client"

/**
 * RF-03.22 — con qué cuenta se entra a cada equipo.
 *
 * Una notebook no se abre sola. Acá se ve qué cuentas tiene, cuáles son de
 * administrador y cuál es la contraseña, si a quien mira le corresponde verla.
 *
 * Quién puede ver cada contraseña lo decide el SERVIDOR, cuenta por cuenta, y
 * llega en `puedeVerLaPassword`. Esta pantalla solo dibuja: si calculara el
 * permiso por su cuenta —mirando el rol, por ejemplo— habría dos reglas que
 * mantener iguales, y la del navegador no protege nada.
 */

function claveDeCuentas(equipoId: string) {
  return ["equipo-cuentas", equipoId]
}

/**
 * Anotar o quitar la última cuenta de un equipo cambia su `tieneCuentas`, que
 * es lo que decide si a un docente le aparece el botón "Cómo entrar". El dato
 * viaja en los listados de equipos, así que hay que releerlos: si no, el botón
 * recién aparece —o desaparece— en la próxima recarga completa.
 */
async function refrescarListadosDeEquipos(queryClient: QueryClient) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["equipos"] }),
    queryClient.invalidateQueries({ queryKey: ["equipos-sueltos"] }),
  ])
}

/** Qué dice la fila sobre la contraseña, en los tres estados posibles. */
function EstadoDeLaPassword({ cuenta }: { cuenta: CuentaDeEquipo }) {
  if (!cuenta.tienePassword) {
    return <EstadoBadge tono="neutro">Entra sin contraseña</EstadoBadge>
  }
  if (!cuenta.hayPasswordParaVer) {
    // El tercer estado: pide contraseña y no la sabemos. Decirlo evita que
    // alguien la busque creyendo que está y no la encuentra.
    return <EstadoBadge tono="alerta">Contraseña no anotada</EstadoBadge>
  }
  if (!cuenta.puedeVerLaPassword) {
    // Se dice que existe pero no se muestra: sin este cartel la pantalla
    // parece rota, y esconder que la cuenta tiene contraseña no protegería
    // nada porque la cuenta ya está listada.
    return <EstadoBadge tono="neutro">Solo la ven los administradores</EstadoBadge>
  }
  return null
}

function Fila({ cuenta, esAdmin }: { cuenta: CuentaDeEquipo; esAdmin: boolean }) {
  const queryClient = useQueryClient()
  const [password, setPassword] = useState<string | null>(null)
  const [editando, setEditando] = useState(false)

  const revelar = useMutation({
    mutationFn: () => inventoryApi.revelarPasswordDeCuenta(cuenta.id),
    onSuccess: ({ password }) => setPassword(password),
  })

  const borrar = useMutation({
    mutationFn: () => inventoryApi.borrarCuentaDeEquipo(cuenta.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: claveDeCuentas(cuenta.equipoId) })
      await refrescarListadosDeEquipos(queryClient)
    },
  })

  if (editando) {
    return (
      <Formulario
        equipoId={cuenta.equipoId}
        cuenta={cuenta}
        onListo={() => setEditando(false)}
      />
    )
  }

  return (
    <div className="grid gap-2 rounded-md border p-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="font-medium">
            <span className="font-mono">{cuenta.usuario}</span>{" "}
            {/* El privilegio se muestra SIEMPRE, aunque la contraseña no:
                saber que una notebook tiene una cuenta de administrador es
                útil incluso sin poder usarla. */}
            {cuenta.privilegio === "ADMINISTRADOR" ? (
              <EstadoBadge tono="alerta">Administrador</EstadoBadge>
            ) : (
              <EstadoBadge tono="neutro">Usuario común</EstadoBadge>
            )}
          </p>
          <p className="text-muted-foreground text-sm">
            {cuenta.clase}
            {cuenta.notas && ` · ${cuenta.notas}`}
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          {cuenta.puedeVerLaPassword &&
            cuenta.hayPasswordParaVer &&
            password === null && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => revelar.mutate()}
                disabled={revelar.isPending}
              >
                Ver contraseña
              </Button>
            )}
          {esAdmin && (
            <>
              <Button variant="outline" size="sm" onClick={() => setEditando(true)}>
                Editar
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => borrar.mutate()}
                disabled={borrar.isPending}
              >
                Quitar
              </Button>
            </>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <EstadoDeLaPassword cuenta={cuenta} />
        {password !== null && (
          <span className="bg-muted rounded px-2 py-1 font-mono text-sm break-all">
            {password}
          </span>
        )}
      </div>

      {revelar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(revelar.error)}</AlertDescription>
        </Alert>
      )}
      {borrar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(borrar.error)}</AlertDescription>
        </Alert>
      )}
    </div>
  )
}

function Formulario({
  equipoId,
  cuenta,
  onListo,
}: {
  equipoId: string
  cuenta?: CuentaDeEquipo
  onListo: () => void
}) {
  const queryClient = useQueryClient()
  const [usuario, setUsuario] = useState(cuenta?.usuario ?? "")
  const [clase, setClase] = useState(cuenta?.clase ?? "")
  const [privilegio, setPrivilegio] = useState<PrivilegioDeCuenta>(
    cuenta?.privilegio ?? "COMUN"
  )
  const [visibilidad, setVisibilidad] = useState<VisibilidadDeCuenta>(
    cuenta?.visibilidad ?? "SOLO_ADMIN"
  )
  const [tienePassword, setTienePassword] = useState(cuenta?.tienePassword ?? true)
  const [password, setPassword] = useState("")
  const [notas, setNotas] = useState(cuenta?.notas ?? "")

  // Las clases ya cargadas, para que no convivan "Microsoft" y "MICROSOFT".
  const { data: clases } = useQuery({
    queryKey: ["clases-de-cuenta"],
    queryFn: inventoryApi.listarClasesDeCuenta,
  })

  const guardar = useMutation({
    mutationFn: () => {
      const datos = {
        usuario: usuario.trim(),
        clase: clase.trim(),
        privilegio,
        visibilidad,
        tienePassword,
        notas: notas.trim(),
      }
      if (cuenta) {
        // Al editar, la contraseña solo se manda si se escribió una nueva:
        // omitirla deja la que estaba, que es el caso normal cuando lo único
        // que se cambia es quién puede verla.
        return inventoryApi.editarCuentaDeEquipo(cuenta.id, {
          ...datos,
          ...(password ? { password } : {}),
        })
      }
      return inventoryApi.crearCuentaDeEquipo(equipoId, { ...datos, password })
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: claveDeCuentas(equipoId) })
      await queryClient.invalidateQueries({ queryKey: ["clases-de-cuenta"] })
      await refrescarListadosDeEquipos(queryClient)
      onListo()
    },
  })

  return (
    <form
      className="grid gap-3 rounded-md border border-dashed p-3"
      onSubmit={(e) => {
        e.preventDefault()
        guardar.mutate()
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor={`cuenta-usuario-${cuenta?.id ?? "nueva"}`}>
            ¿Con qué usuario se entra?
          </Label>
          <Input
            id={`cuenta-usuario-${cuenta?.id ?? "nueva"}`}
            value={usuario}
            onChange={(e) => setUsuario(e.target.value)}
            placeholder="Ej.: alumno, Administrador"
            required
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor={`cuenta-clase-${cuenta?.id ?? "nueva"}`}>
            ¿De qué tipo es?
          </Label>
          <Input
            id={`cuenta-clase-${cuenta?.id ?? "nueva"}`}
            value={clase}
            onChange={(e) => setClase(e.target.value)}
            placeholder="Ej.: Local, Microsoft, Linux"
            list="clases-de-cuenta"
            required
          />
          {/* Texto libre con sugerencias, igual que el tipo de equipo: una
              escuela con RedHat tiene cuentas de Linux, y con una lista
              cerrada eso pediría tocar el sistema. */}
          {clases && clases.data.length > 0 && (
            <datalist id="clases-de-cuenta">
              {clases.data.map((c) => (
                <option key={c} value={c} />
              ))}
            </datalist>
          )}
        </div>
      </div>

      <fieldset className="grid gap-1.5">
        <legend className="text-sm font-medium">¿Qué puede hacer en la máquina?</legend>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name={`privilegio-${cuenta?.id ?? "nueva"}`}
            checked={privilegio === "COMUN"}
            onChange={() => setPrivilegio("COMUN")}
          />
          Usuario común
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name={`privilegio-${cuenta?.id ?? "nueva"}`}
            checked={privilegio === "ADMINISTRADOR"}
            onChange={() => setPrivilegio("ADMINISTRADOR")}
          />
          Administrador (puede instalar y cambiar cosas del sistema)
        </label>
      </fieldset>

      <label className="flex items-start gap-2 text-sm">
        <Checkbox
          className="mt-1"
          checked={tienePassword}
          onCheckedChange={(v) => setTienePassword(v === true)}
        />
        <span>
          Pide contraseña para entrar
          <span className="text-muted-foreground block text-xs">
            Destildalo si la cuenta se abre sola, sin escribir nada.
          </span>
        </span>
      </label>

      {tienePassword && (
        <div className="grid gap-1.5">
          <Label htmlFor={`cuenta-password-${cuenta?.id ?? "nueva"}`}>
            Contraseña{" "}
            <span className="text-muted-foreground">
              {cuenta ? "(dejala vacía para no cambiarla)" : "(si la sabés)"}
            </span>
          </Label>
          <Input
            id={`cuenta-password-${cuenta?.id ?? "nueva"}`}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="off"
          />
          {/* El tercer estado se carga solo: dejarla vacía en el alta anota que
              la cuenta pide contraseña y que no la tenemos. */}
          <p className="text-muted-foreground text-xs">
            Si la cuenta pide una contraseña que nadie anotó, dejá esto vacío: queda
            registrado que hace falta averiguarla.
          </p>
        </div>
      )}

      <fieldset className="grid gap-1.5">
        <legend className="text-sm font-medium">¿Quién puede ver la contraseña?</legend>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name={`visibilidad-${cuenta?.id ?? "nueva"}`}
            checked={visibilidad === "SOLO_ADMIN"}
            onChange={() => setVisibilidad("SOLO_ADMIN")}
          />
          Solo los administradores
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="radio"
            name={`visibilidad-${cuenta?.id ?? "nueva"}`}
            checked={visibilidad === "PUBLICA"}
            onChange={() => setVisibilidad("PUBLICA")}
          />
          Cualquier docente que la necesite
        </label>
        {/* Es independiente del privilegio a propósito: puede haber una cuenta
            de administrador que todo el mundo usa, y una común que solo
            administración debe abrir. */}
        <p className="text-muted-foreground text-xs">
          Esto no depende de si la cuenta es de administrador: una cuenta de administrador
          puede ser de uso común, y una cuenta común puede ser reservada.
        </p>
      </fieldset>

      <div className="grid gap-1.5">
        <Label htmlFor={`cuenta-notas-${cuenta?.id ?? "nueva"}`}>
          Notas <span className="text-muted-foreground">(opcional)</span>
        </Label>
        <Input
          id={`cuenta-notas-${cuenta?.id ?? "nueva"}`}
          value={notas}
          onChange={(e) => setNotas(e.target.value)}
          placeholder="Ej.: es la que usa 5°B"
        />
      </div>

      {guardar.error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(guardar.error)}</AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={guardar.isPending}>
          Guardar
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onListo}>
          Cancelar
        </Button>
      </div>
    </form>
  )
}

export function CuentasDeEquipo({ equipo }: { equipo: Equipo }) {
  const { user } = useAuth()
  const esAdmin = user?.rol === "ADMIN"
  const [agregando, setAgregando] = useState(false)

  const { data, isPending, error } = useQuery({
    queryKey: claveDeCuentas(equipo.id),
    queryFn: () => inventoryApi.listarCuentasDeEquipo(equipo.id),
  })

  const cuentas = data?.data ?? []

  return (
    <div className="grid gap-3 rounded-md border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="font-medium">Cómo entrar a {equipo.etiqueta}</p>
        {esAdmin && !agregando && (
          <Button variant="outline" size="sm" onClick={() => setAgregando(true)}>
            Agregar cuenta
          </Button>
        )}
      </div>

      {isPending && <p className="text-muted-foreground text-sm">Buscando…</p>}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{getErrorMessage(error)}</AlertDescription>
        </Alert>
      )}

      {agregando && (
        <Formulario equipoId={equipo.id} onListo={() => setAgregando(false)} />
      )}

      {!isPending && cuentas.length === 0 && !agregando && (
        // Cargar cuentas es opcional: no tener ninguna es un estado normal,
        // no un equipo mal cargado, y el texto lo dice sin alarmar.
        <p className="text-muted-foreground text-sm">
          No hay ninguna cuenta anotada para este equipo.
          {esAdmin && " Si sabés con qué usuario se entra, cargalo acá."}
        </p>
      )}

      {cuentas.map((cuenta) => (
        <Fila key={cuenta.id} cuenta={cuenta} esAdmin={esAdmin} />
      ))}
    </div>
  )
}
