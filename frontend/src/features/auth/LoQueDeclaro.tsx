import { Link } from "react-router"

import type { Usuario } from "@/features/auth/types"

/**
 * Lo que la persona declaró al registrarse (RF-01.3).
 *
 * El cargo NO otorga permisos: quien se registró como administrador de
 * sistema llega acá igual que cualquier otro, DOCENTE y PENDIENTE. Por eso la
 * ficha lo dice explícitamente — si "Aprobar" significara una cosa para unos
 * y otra para otros, el botón dejaría de ser confiable.
 */
export function LoQueDeclaro({ usuario: u }: { usuario: Usuario }) {
  const comoRol = u.rolSolicitado
    ? ` · como ${u.rolSolicitado === "TITULAR" ? "titular" : "suplente"}`
    : ""

  if (u.cargoSolicitado === "ADMIN_SISTEMA") {
    return (
      <div className="bg-muted/40 mb-3 rounded-md border p-3 text-sm">
        <p className="mb-1 font-medium">Se registró como</p>
        <p>Administrador de Sistema{comoRol}</p>
        <p className="text-muted-foreground mt-1 text-xs">
          Es lo que declaró, y no le da ningún permiso: la cuenta se aprueba igual que
          cualquier otra. Si además tiene que administrar el sistema, se lo promovés
          después desde{" "}
          <Link to="/admin/usuarios" className="underline">
            Usuarios
          </Link>
          .
        </p>
      </div>
    )
  }

  return (
    <div className="bg-muted/40 mb-3 rounded-md border p-3 text-sm">
      <p className="mb-1 font-medium">Pidió dictar</p>
      <p>
        {u.materiaSolicitada || "—"}
        {u.cursoSolicitado ? ` · ${u.cursoSolicitado}` : ""}
        {comoRol}
      </p>
      <p className="text-muted-foreground mt-1 text-xs">
        Es lo que escribió al registrarse, no una referencia: puede que el curso o la
        materia todavía no existan. Se asigna desde{" "}
        <Link to="/admin/academico" className="underline">
          Académico
        </Link>{" "}
        después de aprobarlo.
      </p>
    </div>
  )
}
