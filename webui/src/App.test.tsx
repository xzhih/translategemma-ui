import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import i18n, { appLocaleStorageKey } from "./i18n";

const originalFetch = globalThis.fetch;
const originalCreateObjectURL = globalThis.URL.createObjectURL;
const originalRevokeObjectURL = globalThis.URL.revokeObjectURL;
const originalLocalStorage = Object.getOwnPropertyDescriptor(window, "localStorage");

function createMemoryStorage() {
  const store = new Map<string, string>();
  return {
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    setItem(key: string, value: string) {
      store.set(key, value);
    },
    removeItem(key: string) {
      store.delete(key);
    },
    clear() {
      store.clear();
    },
  };
}

function makeBootstrapState() {
  return {
    pageTitle: "TranslateGemmaUI",
    activeTab: "text",
    textSourceLang: "auto",
    textTargetLang: "zh-CN",
    textInstruction: "",
    textInput: "",
    textOutput: "",
    fileSourceLang: "auto",
    fileTargetLang: "zh-CN",
    fileInstruction: "",
    fileOutput: "",
    status: "Ready",
    statusCode: "ready",
    activeModelId: "q4_k_m",
    models: [
      {
        id: "q4_k_m",
        fileName: "translategemma-4b-it.Q4_K_M.llamafile",
        size: "3.4 GB",
        installed: true,
        active: true,
        selected: true,
        loaded: true,
        visionCapable: false,
        recommended: true,
      },
      {
        id: "q8_0_vision",
        fileName: "translategemma-4b-it.Q8_0.mmproj-Q8_0.llamafile",
        size: "8.9 GB",
        installed: true,
        active: false,
        selected: false,
        loaded: false,
        visionCapable: true,
        recommended: true,
      },
    ],
    languages: [
      {
        code: "auto",
        label: "Auto Detect",
        labels: {
          en: "Auto Detect",
          "zh-CN": "自动检测",
          ja: "自動検出",
          ko: "자동 감지",
          "de-DE": "Automatisch erkennen",
          fr: "Détection automatique",
          es: "Detección automática",
        },
      },
      {
        code: "en",
        label: "English",
        labels: {
          en: "English",
          "zh-CN": "英语",
          ja: "英語",
          ko: "영어",
          "de-DE": "Englisch",
          fr: "anglais",
          es: "inglés",
        },
      },
      {
        code: "en-AE",
        label: "English",
        labels: {
          en: "English",
          "zh-CN": "英语",
          ja: "英語",
          ko: "영어",
          "de-DE": "Englisch",
          fr: "anglais",
          es: "inglés",
        },
        visible: false,
      },
      {
        code: "zh-CN",
        label: "Simplified Chinese",
        labels: {
          en: "Simplified Chinese",
          "zh-CN": "简体中文",
          ja: "簡体中国語",
          ko: "중국어(간체)",
          "de-DE": "Vereinfachtes Chinesisch",
          fr: "chinois simplifié",
          es: "chino simplificado",
        },
      },
    ],
    history: [
      {
        id: 1,
        source: "en",
        target: "zh-CN",
        input: "Hello world",
        output: "你好，世界",
        when: "2026-03-09 08:00",
      },
    ],
    historyCount: 1,
    runtimeStatus: "Runtime ready",
    runtimeStatusCode: "runtime_ready",
    runtimeReady: true,
    needsModelSetup: false,
    visionEnabled: false,
    maxUploadMB: 10,
    now: "2026-03-09 08:00",
  };
}

function jsonResponse(payload: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(payload), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function streamResponse(events: Array<Record<string, unknown>>) {
  const lines = `${events.map((item) => JSON.stringify(item)).join("\n")}\n`;
  return Promise.resolve(
    new Response(lines, {
      status: 200,
      headers: { "Content-Type": "application/x-ndjson" },
    }),
  );
}

function timedStreamingDownloadResponse(
  signal: AbortSignal | undefined,
  steps: Array<{ event: Record<string, unknown>; delayMs?: number }>,
) {
  const encoder = new TextEncoder();
  return Promise.resolve(
    new Response(
      new ReadableStream<Uint8Array>({
        start(controller) {
          let closed = false;
          const timers: Array<ReturnType<typeof setTimeout>> = [];
          const cleanup = () => {
            closed = true;
            for (const timer of timers) {
              clearTimeout(timer);
            }
          };

          let elapsed = 0;
          for (const step of steps) {
            elapsed += step.delayMs ?? 0;
            timers.push(
              setTimeout(() => {
                if (closed) {
                  return;
                }
                controller.enqueue(encoder.encode(`${JSON.stringify(step.event)}\n`));
              }, elapsed),
            );
          }

          signal?.addEventListener(
            "abort",
            () => {
              cleanup();
              controller.error(new DOMException("The operation was aborted.", "AbortError"));
            },
            { once: true },
          );
        },
      }),
      {
        status: 200,
        headers: { "Content-Type": "application/x-ndjson" },
      },
    ),
  );
}

describe("TranslateGemmaUI webui", () => {
  let bootstrapState = makeBootstrapState();

  beforeEach(async () => {
    bootstrapState = makeBootstrapState();
    vi.restoreAllMocks();
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      value: createMemoryStorage(),
    });
    window.history.replaceState({}, "", "/");
    window.localStorage.removeItem(appLocaleStorageKey);
    await i18n.changeLanguage("en");
    Object.defineProperty(globalThis.URL, "createObjectURL", {
      configurable: true,
      value: vi.fn(() => "blob:preview-image"),
    });
    Object.defineProperty(globalThis.URL, "revokeObjectURL", {
      configurable: true,
      value: vi.fn(),
    });
    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";

      if (url === "/api/bootstrap" && method === "GET") {
        return jsonResponse(bootstrapState);
      }
      if (url === "/api/translate/stream" && method === "POST") {
        return streamResponse([
          { type: "status", message: "Streaming translation", messageCode: "streaming_translation" },
          { type: "progress", stage: "translate", message: "Translating part 1/1", percent: 0 },
          { type: "delta", delta: "你好" },
          { type: "delta", delta: "，世界" },
          {
            type: "done",
            message: "Translation completed",
            messageCode: "translation_completed",
            output: "你好，世界",
            history: bootstrapState.history[0],
            count: 2,
          },
        ]);
      }
      if (url === "/api/translate/image" && method === "POST") {
        return jsonResponse({
          ok: true,
          message: "File translation completed",
          messageCode: "file_translation_completed",
          output: "安静区",
          history: {
            id: 2,
            source: "auto",
            target: "zh-CN",
            input: "[image] sign.jpg",
            output: "安静区",
            when: "2026-03-09 08:01",
          },
          count: 2,
        });
      }
      if (url === "/api/models/enable-vision" && method === "POST") {
        bootstrapState.status = "Vision runtime active";
        bootstrapState.statusCode = "vision_runtime_active";
        bootstrapState.visionEnabled = true;
        bootstrapState.activeModelId = "q8_0_vision";
        bootstrapState.models = bootstrapState.models.map((item) => ({
          ...item,
          active: item.id === "q8_0_vision",
          selected: item.id === "q8_0_vision",
          loaded: item.id === "q8_0_vision",
        }));
        return streamResponse([
          { type: "progress", stage: "load", message: "Loading model into runtime", percent: 55 },
          { type: "done", message: "Vision runtime active", messageCode: "vision_runtime_active" },
        ]);
      }
      if (url === "/api/models/activate" && method === "POST") {
        bootstrapState.status = "Model active";
        bootstrapState.statusCode = "model_active";
        bootstrapState.activeModelId = "q8_0_vision";
        bootstrapState.visionEnabled = true;
        bootstrapState.models = bootstrapState.models.map((item) => ({
          ...item,
          active: item.id === "q8_0_vision",
          selected: item.id === "q8_0_vision",
          loaded: item.id === "q8_0_vision",
        }));
        return streamResponse([
          { type: "progress", stage: "load", message: "Loading model into runtime", percent: 55 },
          { type: "done", message: "Model active", messageCode: "model_active" },
        ]);
      }
      if (url === "/api/history/delete" && method === "POST") {
        return jsonResponse({
          ok: true,
          history_id: 1,
          count: 0,
          status: "History item deleted",
          statusCode: "history_item_deleted",
        });
      }
      if (url === "/api/history/clear" && method === "POST") {
        bootstrapState.history = [];
        bootstrapState.historyCount = 0;
        bootstrapState.status = "History cleared";
        bootstrapState.statusCode = "history_cleared";
        return jsonResponse({ ok: true, count: 0, status: "History cleared", statusCode: "history_cleared" });
      }

      throw new Error(`Unhandled fetch: ${method} ${url}`);
      }),
    });
  });

  afterEach(() => {
    window.history.replaceState({}, "", "/");
    window.localStorage.removeItem(appLocaleStorageKey);
    if (originalLocalStorage) {
      Object.defineProperty(window, "localStorage", originalLocalStorage);
    }
    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: originalFetch,
    });
    Object.defineProperty(globalThis.URL, "createObjectURL", {
      configurable: true,
      value: originalCreateObjectURL,
    });
    Object.defineProperty(globalThis.URL, "revokeObjectURL", {
      configurable: true,
      value: originalRevokeObjectURL,
    });
  });

  it("loads bootstrap state and streams a text translation", async () => {
    const user = userEvent.setup();

    render(<App />);

    expect(await screen.findByRole("button", { name: "Go to text screen" })).toHaveTextContent("TranslateGemmaUI");
    expect(screen.queryByText("Active model")).not.toBeInTheDocument();
    expect(document.querySelector(".runtime-pill")).toBeNull();
    expect(screen.getByText("Optional guidance")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Type or paste text to translate...")).toBeInTheDocument();
    expect(screen.getByText("Result...")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("For example: Keep product names and technical terms unchanged.")).toBeInTheDocument();
    expect(screen.getAllByRole("combobox", { name: "Language selector" })[0]).toHaveValue("auto");

    await user.type(screen.getByLabelText("Source text"), "Hello world");
    await user.type(screen.getByLabelText("Translation instruction"), "Use Mainland China UI wording.");
    await user.click(screen.getByRole("button", { name: "Translate" }));

    expect(await screen.findByText("你好，世界")).toBeInTheDocument();
    expect(screen.queryByText("Translation completed")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Model" }));
    expect(screen.queryByText("Translation completed")).not.toBeInTheDocument();
  });

  it("switches the UI locale to zh-CN and persists it", async () => {
    const user = userEvent.setup();
    bootstrapState.languages[1].labels["zh-CN"] = "英语-后端";

    render(<App />);

    await screen.findByRole("button", { name: "Go to text screen" });
    const localeSelect = screen.getByRole("combobox", { name: "UI language" });
    const sourceLanguageSelect = screen.getAllByRole("combobox", { name: "Language selector" })[0];
    expect(within(localeSelect).getByRole("option", { name: "日本語" })).toBeInTheDocument();
    expect(within(localeSelect).getByRole("option", { name: "한국어" })).toBeInTheDocument();
    expect(within(localeSelect).getByRole("option", { name: "Deutsch" })).toBeInTheDocument();
    expect(within(localeSelect).getByRole("option", { name: "Français" })).toBeInTheDocument();
    expect(within(localeSelect).getByRole("option", { name: "Español" })).toBeInTheDocument();
    expect(within(sourceLanguageSelect).queryByRole("option", { name: "English (en-AE)" })).not.toBeInTheDocument();

    await user.selectOptions(localeSelect, "zh-CN");

    expect(await screen.findByRole("button", { name: "切换到文本翻译页" })).toHaveTextContent("TranslateGemmaUI");
    expect(screen.getByRole("button", { name: "模型" })).toBeInTheDocument();
    expect(screen.getByText("附加说明")).toBeInTheDocument();
    expect(within(sourceLanguageSelect).getByRole("option", { name: "英语-后端 (en)" })).toBeInTheDocument();
    expect(window.localStorage.getItem(appLocaleStorageKey)).toBe("zh-CN");
  });

  it("opens the model drawer when translate is clicked without any local model", async () => {
    const user = userEvent.setup();

    bootstrapState.needsModelSetup = true;
    bootstrapState.runtimeReady = false;
    bootstrapState.runtimeStatus = "No local model installed";
    bootstrapState.runtimeStatusCode = "no_local_model_select_download";
    bootstrapState.models = bootstrapState.models.map((item) => ({
      ...item,
      installed: false,
      active: false,
      selected: false,
      loaded: false,
    }));

    render(<App />);

    await screen.findByRole("button", { name: "Go to text screen" });
    expect(screen.queryByText("No local runtime is installed yet. Open the model drawer and download q4_k_m or q8_0_vision first.")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Translate" }));

    expect(screen.getByRole("dialog", { name: "Model List" })).toBeInTheDocument();
  });

  it("shows Loading model on the first translation while the selected runtime loads", async () => {
    const user = userEvent.setup();
    const baseFetch = globalThis.fetch as typeof fetch;
    let resolveTranslate: (() => void) | undefined;

    bootstrapState.runtimeReady = false;
    bootstrapState.runtimeStatus = "Runtime idle";
    bootstrapState.runtimeStatusCode = "runtime_idle_load_on_first_translation";
    bootstrapState.models = bootstrapState.models.map((item) =>
      item.id === "q4_k_m"
        ? { ...item, active: false, selected: true, loaded: false }
        : { ...item, active: false, selected: false, loaded: false },
    );

    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        const method = init?.method ?? "GET";
        if (url === "/api/translate/stream" && method === "POST") {
          return new Promise<Response>((resolve) => {
            resolveTranslate = () => {
              bootstrapState.status = "Translation completed";
              bootstrapState.statusCode = "translation_completed";
              bootstrapState.runtimeReady = true;
              bootstrapState.runtimeStatus = "Runtime ready";
              bootstrapState.runtimeStatusCode = "runtime_ready";
              bootstrapState.models = bootstrapState.models.map((item) =>
                item.id === "q4_k_m"
                  ? { ...item, active: true, selected: true, loaded: true }
                  : { ...item, active: false, selected: false, loaded: false },
              );
              void streamResponse([
                { type: "progress", stage: "load", message: "Loading model into runtime", percent: 35 },
                { type: "status", message: "Streaming translation", messageCode: "streaming_translation" },
                { type: "delta", delta: "你好" },
                { type: "delta", delta: "，世界" },
                {
                  type: "done",
                  message: "Translation completed",
                  messageCode: "translation_completed",
                  output: "你好，世界",
                  history: bootstrapState.history[0],
                  count: 2,
                },
              ]).then(resolve);
            };
          });
        }
        return baseFetch(input, init);
      }),
    });

    render(<App />);

    await screen.findByRole("button", { name: "Go to text screen" });
    await user.type(screen.getByLabelText("Source text"), "Hello world");
    await user.click(screen.getByRole("button", { name: "Translate" }));

    expect(screen.getByRole("button", { name: "Loading model" })).toBeDisabled();

    resolveTranslate?.();

    expect(await screen.findByText("你好，世界")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Model" }));
    expect(screen.getByRole("button", { name: "ACTIVE" })).toBeDisabled();
  });

  it("localizes coded server errors when the UI locale is zh-CN", async () => {
    const user = userEvent.setup();
    const baseFetch = globalThis.fetch as typeof fetch;

    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        const method = init?.method ?? "GET";
        if (url === "/api/history/delete" && method === "POST") {
          return jsonResponse(
            {
              ok: false,
              count: 1,
              status: "History item not found",
              statusCode: "history_item_not_found",
            },
            404,
          );
        }
        return baseFetch(input, init);
      }),
    });

    render(<App />);

    await screen.findByRole("button", { name: "Go to text screen" });
    await user.selectOptions(screen.getByRole("combobox", { name: "UI language" }), "zh-CN");
    await user.click(screen.getByRole("button", { name: "历史" }));
    await user.click(screen.getByText("删除"));

    expect(await screen.findByText("未找到历史记录")).toBeInTheDocument();
  });

  it("auto grows the source textarea to fit long content", async () => {
    const user = userEvent.setup();
    vi.spyOn(HTMLTextAreaElement.prototype, "scrollHeight", "get").mockReturnValue(320);

    render(<App />);

    const input = (await screen.findByLabelText("Source text")) as HTMLTextAreaElement;
    await user.type(input, "A longer block of text that should grow the editor.");

    expect(input.style.height).toBe("320px");
  });

  it("opens history and shows detail", async () => {
    const user = userEvent.setup();

    render(<App />);

    await screen.findByRole("button", { name: "History" });
    await user.click(screen.getByRole("button", { name: "History" }));
    await user.click(screen.getByText("Hello world"));

    const dialog = screen.getByRole("dialog", { name: "History Detail" });
    expect(dialog).toBeInTheDocument();
    expect(dialog).toHaveTextContent("你好，世界");
  });

  it("clears history from the drawer", async () => {
    const user = userEvent.setup();

    render(<App />);

    await screen.findByRole("button", { name: "History" });
    await user.click(screen.getByRole("button", { name: "History" }));
    await user.click(screen.getByRole("button", { name: "Clear history" }));

    await waitFor(() => {
      expect(screen.queryByText("Hello world")).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Clear history" })).toBeDisabled();
  });

  it("can switch to vision runtime and translate an image", async () => {
    const user = userEvent.setup();

    render(<App />);

    await screen.findByRole("button", { name: "Image" });
    await user.click(screen.getByRole("button", { name: "Image" }));
    await user.click(screen.getByRole("button", { name: "Switch to q8_0_vision" }));

    expect(await screen.findByRole("button", { name: "Upload image" })).toBeInTheDocument();
    expect(screen.getByText("Result...")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("For example: Keep brand names unchanged and preserve line breaks.")).toBeInTheDocument();
    expect(screen.queryByText("Vision runtime enabled")).not.toBeInTheDocument();

    const input = screen.getByLabelText("Upload image");
    await user.upload(input, new File(["img"], "sign.jpg", { type: "image/jpeg" }));
    expect(screen.getByAltText("Preview of sign.jpg")).toBeInTheDocument();
    expect(screen.queryByText("Upload an image")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Translate" }));

    expect(await screen.findByText("安静区")).toBeInTheDocument();
    expect(screen.queryByText("File translation completed")).not.toBeInTheDocument();
  });

  it("shows download progress, speed, and allows canceling a model download", async () => {
    const user = userEvent.setup();
    const baseFetch = globalThis.fetch as typeof fetch;

    bootstrapState.models = [
      bootstrapState.models[0],
      {
        id: "q6_k",
        fileName: "translategemma-4b-it.Q6_K.llamafile",
        size: "3.0 GB",
        installed: false,
        active: false,
        selected: false,
        loaded: false,
        visionCapable: false,
        recommended: false,
      },
      bootstrapState.models[1],
    ];

    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        const method = init?.method ?? "GET";
        if (url === "/api/models/install" && method === "POST") {
          return timedStreamingDownloadResponse(init?.signal ?? undefined, [
            {
              event: {
                type: "status",
                stage: "check",
                message: "Preparing model install",
                messageCode: "preparing_model_install",
                percent: 0,
              },
            },
            {
              event: {
                type: "progress",
                stage: "download",
                message: "Downloading artifact",
                percent: 37.5,
                downloadedBytes: 805306368,
                totalBytes: 2147483648,
                speedBytesPerSecond: 67108864,
              },
              delayMs: 40,
            },
          ]);
        }
        return baseFetch(input, init);
      }),
    });

    render(<App />);

    await screen.findByRole("button", { name: "Model" });
    await user.click(screen.getByRole("button", { name: "Model" }));
    await user.click(screen.getAllByRole("button", { name: "Download" })[0]);

    expect(await screen.findByText("Preparing model install")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Download" })).not.toBeInTheDocument();
    expect(await screen.findByText("768 MB / 2.00 GB")).toBeInTheDocument();
    expect(screen.getByText("64.0 MB/s")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Download" })).toBeEnabled();
    });
    expect(screen.queryByText("768 MB / 2.00 GB")).not.toBeInTheDocument();
  });

  it("shows Selected while a chosen model is loading and disables translate actions", async () => {
    const user = userEvent.setup();
    const baseFetch = globalThis.fetch as typeof fetch;
    let resolveActivate: (() => void) | undefined;

    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        const method = init?.method ?? "GET";
        if (url === "/api/models/activate" && method === "POST") {
          return new Promise<Response>((resolve) => {
            resolveActivate = () => {
              bootstrapState.status = "Model active";
              bootstrapState.statusCode = "model_active";
              bootstrapState.activeModelId = "q8_0_vision";
              bootstrapState.visionEnabled = true;
              bootstrapState.models = bootstrapState.models.map((item) => ({
                ...item,
                active: item.id === "q8_0_vision",
                selected: item.id === "q8_0_vision",
                loaded: item.id === "q8_0_vision",
              }));
              void streamResponse([
                { type: "progress", stage: "load", message: "Loading model into runtime", percent: 55 },
                { type: "done", message: "Model active", messageCode: "model_active" },
              ]).then(resolve);
            };
          });
        }
        return baseFetch(input, init);
      }),
    });

    render(<App />);

    await screen.findByRole("button", { name: "Model" });
    await user.click(screen.getByRole("button", { name: "Model" }));
    await user.click(screen.getByRole("button", { name: "Use now" }));

    const dialog = screen.getByRole("dialog", { name: "Model List" });
    expect(screen.getByText("Selected")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "ACTIVE" })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Loading model" })).toBeDisabled();

    resolveActivate?.();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "ACTIVE" })).toBeDisabled();
    });
  });

  it("can reset the selected image", async () => {
    const user = userEvent.setup();

    render(<App />);

    await screen.findByRole("button", { name: "Image" });
    await user.click(screen.getByRole("button", { name: "Image" }));
    await user.click(screen.getByRole("button", { name: "Switch to q8_0_vision" }));

    const input = screen.getByLabelText("Upload image");
    await user.upload(input, new File(["img"], "sign.jpg", { type: "image/jpeg" }));
    expect(screen.getByAltText("Preview of sign.jpg")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Reset" }));

    expect(screen.queryByAltText("Preview of sign.jpg")).not.toBeInTheDocument();
    expect(screen.getByText("Upload an image")).toBeInTheDocument();
  });

  it("keeps the hidden file input available for choose another", async () => {
    const user = userEvent.setup();
    const clickSpy = vi.spyOn(HTMLInputElement.prototype, "click");

    render(<App />);

    await screen.findByRole("button", { name: "Image" });
    await user.click(screen.getByRole("button", { name: "Image" }));
    await user.click(screen.getByRole("button", { name: "Switch to q8_0_vision" }));

    const input = screen.getByLabelText("Upload image");
    await user.upload(input, new File(["img"], "first.jpg", { type: "image/jpeg" }));
    expect(screen.getByAltText("Preview of first.jpg")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Choose another" }));
    expect(clickSpy).toHaveBeenCalled();

    await user.upload(screen.getByLabelText("Upload image"), new File(["img"], "second.jpg", { type: "image/jpeg" }));
    expect(screen.getByAltText("Preview of second.jpg")).toBeInTheDocument();
  });

  it("accepts drag and drop image selection", async () => {
    const user = userEvent.setup();

    render(<App />);

    await screen.findByRole("button", { name: "Image" });
    await user.click(screen.getByRole("button", { name: "Image" }));
    await user.click(screen.getByRole("button", { name: "Switch to q8_0_vision" }));

    const dropzone = screen.getByRole("region", { name: "Image upload area" });
    const file = new File(["img"], "drop-sign.jpg", { type: "image/jpeg" });
    const dataTransfer = {
      files: [file],
      items: [],
      types: ["Files"],
      dropEffect: "copy",
    };

    fireEvent.dragEnter(dropzone, { dataTransfer });
    fireEvent.dragOver(dropzone, { dataTransfer });
    fireEvent.drop(dropzone, { dataTransfer });

    expect(screen.getByAltText("Preview of drop-sign.jpg")).toBeInTheDocument();
  });

  it("opens the model drawer and activates another installed runtime", async () => {
    const user = userEvent.setup();

    render(<App />);

    await screen.findByRole("button", { name: "Model" });
    await user.click(screen.getByRole("button", { name: "Model" }));
    expect(screen.getAllByText("Recommended").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Vision").length).toBeGreaterThan(0);
    await user.click(screen.getByRole("button", { name: "Use now" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "ACTIVE" })).toBeDisabled();
    });
    expect(screen.getAllByText("ACTIVE")).toHaveLength(1);
  });

  it("keeps passive model and runtime status hidden from the main screen", async () => {
    bootstrapState.runtimeReady = false;
    bootstrapState.runtimeStatus = "No local model installed";

    render(<App />);

    await screen.findByRole("button", { name: "Go to text screen" });
    expect(document.querySelector(".runtime-pill")).toBeNull();
    expect(document.querySelector(".status-banner")).toBeNull();
    expect(screen.queryByText("Active model")).not.toBeInTheDocument();
  });
});
