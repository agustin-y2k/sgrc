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
