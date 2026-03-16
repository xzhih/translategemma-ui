import "@testing-library/jest-dom/vitest";

Object.defineProperty(window.navigator, "clipboard", {
  configurable: true,
  value: {
    writeText: async () => undefined,
  },
});

class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

Object.defineProperty(globalThis, "ResizeObserver", {
  configurable: true,
  writable: true,
  value: MockResizeObserver,
});
