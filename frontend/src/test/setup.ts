import "@testing-library/jest-dom/vitest"
import { configure } from "@testing-library/react"

// El default de findBy*/waitFor es 1s.
configure({ asyncUtilTimeout: 5000 })

// jsdom no implementa ResizeObserver, pero varios primitivos de Radix
// (Checkbox, Select, …) lo usan para medirse en un layout effect.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

globalThis.ResizeObserver ??= ResizeObserverStub as unknown as typeof ResizeObserver

// jsdom no implementa matchMedia, y useEsAngosto lo consulta para decidir si
// dibuja tarjetas o tabla. Sin esto el hook se cae; con esto, los tests corren
// en "pantalla ancha", que es la estructura que casi todos esperan. Un test
// que quiera la angosta lo sobrescribe.
globalThis.matchMedia ??= ((consulta: string) => ({
  matches: false,
  media: consulta,
  onchange: null,
  addEventListener: () => {},
  removeEventListener: () => {},
  addListener: () => {},
  removeListener: () => {},
  dispatchEvent: () => false,
})) as unknown as typeof window.matchMedia
