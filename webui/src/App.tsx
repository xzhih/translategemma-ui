import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
  type ButtonHTMLAttributes,
  type ReactNode,
} from "react";
import {
  ArrowLeftRight,
  ChevronDown,
  Download,
  ImageUp,
  LoaderCircle,
  Moon,
  SunMedium,
  Trash2,
  X,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { normalizeAppLocale, setAppLocale, type AppLocale } from "./i18n";
import {
  applyAppTheme,
  detectInitialAppTheme,
  readAppliedAppTheme,
  setAppTheme,
  watchSystemTheme,
  type AppTheme,
  type ResolvedAppTheme,
} from "./theme";
import "./App.css";

type ScreenMode = "text" | "image";
type DrawerKind = "model" | "history" | null;
type BadgeTone = "neutral" | "accent" | "ready" | "warning";

const maxHistoryEntries = 200;
const appLocaleOptions: Array<{ code: AppLocale; label: string }> = [
  { code: "en", label: "English" },
  { code: "zh-CN", label: "简体中文" },
  { code: "ja", label: "日本語" },
  { code: "ko", label: "한국어" },
  { code: "de-DE", label: "Deutsch" },
  { code: "fr", label: "Français" },
  { code: "es", label: "Español" },
];
const errorStatusCodes = new Set([
  "model_not_installed_locally",
  "unknown_model_selection",
  "invalid_form_payload",
  "missing_history_id",
  "invalid_history_id",
  "history_item_not_found",
  "vision_runtime_unavailable",
  "active_runtime_no_image_support",
  "invalid_multipart_payload_or_file_too_large",
  "missing_image_file",
  "unable_to_read_image_file",
  "image_file_empty",
  "image_file_exceeds_size_limit",
  "unsupported_image_format",
]);

interface LanguageOption {
  code: string;
  label: string;
  labels: Record<string, string>;
  visible?: boolean;
}

interface ModelItem {
  id: string;
  fileName: string;
  size: string;
  installed: boolean;
  active: boolean;
  selected: boolean;
  loaded: boolean;
  visionCapable: boolean;
  recommended: boolean;
}

interface HistoryEntry {
  id: number;
  source: string;
  target: string;
  input: string;
  output: string;
  when: string;
}

interface BootstrapState {
  pageTitle: string;
  activeTab: ScreenMode;
  textSourceLang: string;
  textTargetLang: string;
  textInstruction: string;
  textInput: string;
  textOutput: string;
  fileSourceLang: string;
  fileTargetLang: string;
  fileInstruction: string;
  fileOutput: string;
  status: string;
  statusCode?: string;
  activeModelId: string;
  models: ModelItem[];
  languages: LanguageOption[];
  history: HistoryEntry[];
  historyCount: number;
  runtimeStatus: string;
  runtimeStatusCode?: string;
  runtimeReady: boolean;
  needsModelSetup: boolean;
  visionEnabled: boolean;
  maxUploadMB: number;
  now: string;
}

interface StreamEvent {
  type: string;
  stage?: string;
  message?: string;
  messageCode?: string;
  percent?: number;
  downloadedBytes?: number;
  totalBytes?: number;
  speedBytesPerSecond?: number;
  delta?: string;
  output?: string;
  history?: HistoryEntry;
  count?: number;
}

interface ImageResult {
  ok: boolean;
  output?: string;
  message?: string;
  messageCode?: string;
  history?: HistoryEntry;
  count?: number;
}

interface HistoryDeleteResponse {
  ok: boolean;
  history_id?: number;
  count: number;
  status: string;
  statusCode?: string;
}

interface StatusState {
  code?: string;
  fallback?: string;
  isError: boolean;
}

interface ModelDownloadState {
  modelId: string;
  modelName: string;
  message: string;
  percent: number;
  downloadedBytes: number;
  totalBytes: number;
  speedBytesPerSecond: number;
  canceling: boolean;
}

class StreamEventError extends Error {
  code?: string;

  constructor(message: string, code?: string) {
    super(message);
    this.name = "StreamEventError";
    this.code = code;
  }
}

function clampPercent(value: number | undefined) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return 0;
  }
  return Math.max(0, Math.min(100, value));
}

function formatBinarySize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "--";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const digits = value >= 100 || unitIndex === 0 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(digits)} ${units[unitIndex]}`;
}

function formatTransferRate(bytesPerSecond: number) {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) {
    return "-- /s";
  }
  return `${formatBinarySize(bytesPerSecond)}/s`;
}

function isAbortError(error: unknown) {
  return (
    (error instanceof DOMException && error.name === "AbortError") ||
    (error instanceof Error && error.name === "AbortError")
  );
}

function localizeClientErrorMessage(
  message: string,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  switch (message.trim().toLowerCase()) {
    case "failed to load bootstrap state":
      return t("errors.failedLoadBootstrap");
    case "request failed":
      return t("errors.requestFailed");
    case "stream failed":
      return t("errors.streamFailed");
    case "image translation failed":
      return t("errors.imageTranslationFailed");
    case "model action failed":
      return t("errors.modelActionFailed");
    case "history delete failed":
      return t("errors.historyDeleteFailed");
    case "history clear failed":
      return t("errors.historyClearFailed");
    default:
      return message;
  }
}

function resolveServerMessage(
  code: string | undefined,
  fallback: string | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (code) {
    return t(`serverStatus.${code}`, { defaultValue: fallback ?? code });
  }
  if (fallback) {
    return localizeClientErrorMessage(fallback, t);
  }
  return "";
}

function localizedLanguageLabel(option: LanguageOption, locale: AppLocale) {
  return option.labels[locale] ?? option.label;
}

function cn(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(" ");
}

const screenSectionClassName =
  "flex flex-col gap-4 rounded-2xl border border-[color:var(--border)] bg-[color:var(--card)] p-6 max-md:p-5";
const editorGridClassName =
  "grid h-[clamp(720px,68vh,760px)] grid-cols-2 grid-rows-[minmax(0,1fr)_clamp(132px,18vh,176px)] gap-3 max-lg:h-auto max-lg:grid-cols-1 max-lg:grid-rows-none";
const outputPanelClassName =
  "col-start-2 row-start-1 row-span-2 flex min-h-0 h-full items-start overflow-auto max-lg:col-auto max-lg:row-auto max-lg:row-span-1";
const mutedLabelClassName = "text-[11px] font-normal text-[color:var(--muted-text)]";
const detailTextClassName = "m-0 whitespace-pre-wrap break-words text-[13px] leading-[1.5]";
const sideDrawerBackdropClassName = "absolute inset-0 border-0 bg-[color:var(--drawer-scrim)]";
const sideDrawerPanelClassName =
  "absolute top-0 right-0 flex h-dvh w-[408px] max-w-full flex-col gap-4 border-l border-[color:var(--border)] bg-[color:var(--drawer-surface)] p-6 shadow-[var(--panel-edge-shadow)]";

function Button({
  children,
  variant = "secondary",
  className = "",
  type = "button",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  children: React.ReactNode;
  variant?: "primary" | "secondary" | "danger" | "success";
  className?: string;
  type?: "button" | "submit";
}) {
  const variantClassName =
    variant === "primary"
      ? "border-transparent bg-[color:var(--primary)] text-[color:var(--primary-foreground)] hover:brightness-105"
      : variant === "danger"
        ? "border-[color:var(--danger-border)] bg-[color:var(--danger-bg)] text-[color:var(--danger-text)] hover:brightness-95"
        : variant === "success"
          ? "border-[color:var(--success-border)] bg-[color:var(--success-bg)] text-[color:var(--success-text)] hover:brightness-95"
          : "border-[color:var(--border)] bg-[color:var(--muted)] text-[color:var(--text)] hover:bg-[color:var(--muted-strong)]";
  return (
    <button
      type={type}
      className={cn(
        "inline-flex min-h-10 shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-[10px] border px-4 py-2 text-sm font-medium transition-[background-color,border-color,color,filter] duration-200",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--background)]",
        "disabled:pointer-events-none disabled:opacity-60",
        variantClassName,
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

function StatusBadge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: BadgeTone;
}) {
  const toneClassName =
    tone === "accent"
      ? "border-[color:var(--primary-soft-border)] bg-[color:var(--primary-soft-bg)] text-[color:var(--primary-soft-text)]"
      : tone === "ready"
        ? "border-[color:var(--success-border)] bg-[color:var(--success-bg)] text-[color:var(--success-text)]"
        : tone === "warning"
          ? "border-[color:var(--warning-border)] bg-[color:var(--warning-bg)] text-[color:var(--warning-text)]"
          : "border-[color:var(--border)] bg-[color:var(--muted)] text-[color:var(--text-soft)]";
  return (
    <span
      className={cn(
        "inline-flex min-h-6 items-center rounded-full border px-2 py-1 text-[11px] font-semibold leading-none tracking-[0.02em]",
        toneClassName,
      )}
    >
      {children}
    </span>
  );
}

function describeModelState(model: ModelItem | null, needsModelSetup: boolean) {
  if (needsModelSetup) {
    return {
      labelKey: "model.state.downloadRequired.label",
      tone: "warning" as BadgeTone,
      detailKey: "model.state.downloadRequired.detail",
    };
  }
  if (!model) {
    return {
      labelKey: "model.state.unavailable.label",
      tone: "warning" as BadgeTone,
      detailKey: "model.state.unavailable.detail",
    };
  }
  if (model.loaded) {
    return {
      labelKey: "model.state.loaded.label",
      tone: "ready" as BadgeTone,
      detailKey: "model.state.loaded.detail",
    };
  }
  if (model.selected && model.installed) {
    return {
      labelKey: "model.state.selected.label",
      tone: "accent" as BadgeTone,
      detailKey: "model.state.selected.detail",
    };
  }
  if (model.installed) {
    return {
      labelKey: "model.state.installed.label",
      tone: "neutral" as BadgeTone,
      detailKey: "model.state.installed.detail",
    };
  }
  return {
    labelKey: "model.state.remote.label",
    tone: "neutral" as BadgeTone,
    detailKey: "model.state.remote.detail",
  };
}

function isErrorStatusCode(code: string | undefined) {
  return !!code && errorStatusCodes.has(code);
}

function shouldShowStatusBanner(statusMessage: string, isError: boolean) {
  return isError && statusMessage.trim().length > 0;
}

function SelectField({
  value,
  options,
  onChange,
  allowAuto = true,
}: {
  value: string;
  options: LanguageOption[];
  onChange: (next: string) => void;
  allowAuto?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const currentLocale = normalizeAppLocale(i18n.resolvedLanguage) ?? "en";
  return (
    <label className="flex h-10 min-w-0 flex-1 items-center justify-between gap-3 rounded-lg border border-[color:var(--border)] bg-[color:var(--card)] px-3 text-sm font-medium text-[color:var(--text)] shadow-[var(--control-shadow)]">
      <select
        aria-label={t("accessibility.languageSelector")}
        className="w-full appearance-none border-0 bg-transparent text-inherit outline-none"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        {options
          .filter((option) => option.visible !== false)
          .filter((option) => allowAuto || option.code !== "auto")
          .map((option) => (
            <option key={option.code} value={option.code}>
              {localizedLanguageLabel(option, currentLocale)} ({option.code})
            </option>
          ))}
      </select>
      <ChevronDown size={16} strokeWidth={2} className="shrink-0 text-[color:var(--muted-text)]" />
    </label>
  );
}

function AppLocaleField({
  value,
  onChange,
}: {
  value: AppLocale;
  onChange: (next: AppLocale) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="flex h-10 min-w-[156px] flex-[0_1_220px] items-center justify-between gap-3 rounded-lg border border-[color:var(--border)] bg-[color:var(--card)] px-3 text-sm font-medium text-[color:var(--text)] shadow-[var(--control-shadow)] max-w-[min(260px,100%)]">
      <select
        aria-label={t("accessibility.uiLanguageSelector")}
        className="w-full appearance-none border-0 bg-transparent text-inherit outline-none"
        value={value}
        onChange={(event) => onChange(event.target.value as AppLocale)}
      >
        {appLocaleOptions.map((option) => (
          <option key={option.code} value={option.code}>
            {option.label}
          </option>
          ))}
      </select>
      <ChevronDown size={16} strokeWidth={2} className="shrink-0 text-[color:var(--muted-text)]" />
    </label>
  );
}

function ThemeToggle({
  resolvedTheme,
  onToggle,
}: {
  resolvedTheme: ResolvedAppTheme;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const switchLabel =
    resolvedTheme === "dark"
      ? t("header.switchToLightTheme", { defaultValue: "Switch to light mode" })
      : t("header.switchToDarkTheme", { defaultValue: "Switch to dark mode" });
  return (
    <button
      type="button"
      className={cn(
        "inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-[color:var(--border)] bg-[color:var(--muted)] text-[color:var(--text-soft)] shadow-[var(--control-shadow)] transition-[background-color,border-color,color,filter] duration-200",
        "hover:bg-[color:var(--muted-strong)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--background)]",
        "[&>svg]:block [&>svg]:shrink-0",
      )}
      aria-label={switchLabel}
      title={switchLabel}
      onClick={onToggle}
    >
      {resolvedTheme === "dark" ? <SunMedium size={19} strokeWidth={2} /> : <Moon size={19} strokeWidth={2} />}
    </button>
  );
}

function ScreenTopbar({
  activeMode,
  onSwitchText,
  onSwitchImage,
  onShowHistory,
}: {
  activeMode: ScreenMode;
  onSwitchText: () => void;
  onSwitchImage: () => void;
  onShowHistory: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-4 max-md:flex-wrap">
      <div
        className="inline-flex gap-1 rounded-xl border border-[color:var(--primary-soft-border)] bg-[color:var(--primary-soft-panel)] p-1"
        role="tablist"
        aria-label={t("accessibility.translationMode")}
      >
        <button
          type="button"
          className={cn(
            "appearance-none border-0 bg-transparent shadow-none outline-none",
            "inline-flex h-9 items-center justify-center rounded-[10px] px-4 text-sm font-bold text-[color:var(--muted-text)] transition-[background-color,color,box-shadow] duration-200",
            "hover:bg-[color:var(--card)]/70 focus-visible:ring-2 focus-visible:ring-[color:var(--primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--card)]",
            activeMode === "text" && "bg-white text-[color:var(--primary)] shadow-[var(--control-shadow)] dark:bg-[color:var(--card)]",
          )}
          onClick={onSwitchText}
        >
          {t("tabs.text")}
        </button>
        <button
          type="button"
          className={cn(
            "appearance-none border-0 bg-transparent shadow-none outline-none",
            "inline-flex h-9 items-center justify-center rounded-[10px] px-4 text-sm font-bold text-[color:var(--muted-text)] transition-[background-color,color,box-shadow] duration-200",
            "hover:bg-[color:var(--card)]/70 focus-visible:ring-2 focus-visible:ring-[color:var(--primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--card)]",
            activeMode === "image" && "bg-white text-[color:var(--primary)] shadow-[var(--control-shadow)] dark:bg-[color:var(--card)]",
          )}
          onClick={onSwitchImage}
        >
          {t("tabs.image")}
        </button>
      </div>
      <Button onClick={onShowHistory}>{t("buttons.history")}</Button>
    </div>
  );
}

function TranslationLanguageRow({
  sourceValue,
  targetValue,
  options,
  onSourceChange,
  onTargetChange,
  onSwap,
  sourceAllowsAuto,
}: {
  sourceValue: string;
  targetValue: string;
  options: LanguageOption[];
  onSourceChange: (next: string) => void;
  onTargetChange: (next: string) => void;
  onSwap: () => void;
  sourceAllowsAuto: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2.5 max-md:flex-wrap">
      <SelectField value={sourceValue} options={options} onChange={onSourceChange} allowAuto={sourceAllowsAuto} />
      <button
        type="button"
        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-[color:var(--border)] bg-[color:var(--muted)] text-[color:var(--text)] transition-colors hover:bg-[color:var(--muted-strong)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--background)]"
        aria-label={t("accessibility.swapLanguages")}
        onClick={onSwap}
      >
        <ArrowLeftRight size={16} strokeWidth={2} />
      </button>
      <SelectField value={targetValue} options={options} onChange={onTargetChange} allowAuto={false} />
    </div>
  );
}

function TranslationActionsRow({
  leftAction,
  centerAction,
  rightAction,
}: {
  leftAction?: ReactNode;
  centerAction: ReactNode;
  rightAction?: ReactNode;
}) {
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-3 max-md:grid-cols-1">
      <div className="flex min-h-10 items-center justify-start max-md:justify-stretch max-md:[&>*]:w-full">{leftAction}</div>
      <div className="flex items-center justify-center max-md:justify-stretch max-md:[&>*]:w-full">{centerAction}</div>
      <div className="flex min-h-10 items-center justify-end max-md:justify-stretch max-md:[&>*]:w-full">{rightAction}</div>
    </div>
  );
}

function ScrollView({
  ariaLabel,
  children,
}: {
  ariaLabel: string;
  children: ReactNode;
}) {
  return (
    <div className="min-h-0 flex-1 overflow-hidden" role="region" aria-label={ariaLabel}>
      <div className="scroll-view__viewport flex h-full flex-col gap-3 overflow-x-hidden overflow-y-auto pr-1 [scrollbar-gutter:stable] [overscroll-behavior:contain]">
        {children}
      </div>
    </div>
  );
}

function IconButton({
  children,
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  children: ReactNode;
  className?: string;
}) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex h-7 w-7 items-center justify-center rounded-full border-0 bg-transparent p-0 text-[color:var(--muted-text)] transition-colors hover:bg-[color:var(--muted)] hover:text-[color:var(--text)]",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--drawer-surface)]",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

function OutlineIconButton({
  children,
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  children: ReactNode;
  className?: string;
}) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex h-9 w-9 items-center justify-center rounded-full border border-[color:var(--border)] bg-[color:var(--card)] p-0 text-[color:var(--text)] shadow-[var(--control-shadow)] transition-colors hover:bg-[color:var(--muted)]",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--primary)] focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--drawer-surface)]",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

function Panel({
  children,
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement> & {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "min-h-[clamp(220px,34vh,340px)] rounded-[20px] border border-[color:var(--border)] bg-[color:var(--card)] p-3.5",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

function SectionCard({
  children,
  className,
  ...props
}: React.HTMLAttributes<HTMLElement> & {
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn(screenSectionClassName, className)} {...props}>
      {children}
    </section>
  );
}

function OutputPanel({
  value,
  placeholder,
  className,
}: {
  value: string;
  placeholder: string;
  className?: string;
}) {
  return (
    <Panel className={cn(outputPanelClassName, !value && "text-left", className)}>
      <p
        className={cn(
          "m-0 whitespace-pre-wrap break-words text-sm leading-[1.45]",
          !value && "text-[color:var(--muted-text)]",
        )}
      >
        {value || placeholder}
      </p>
    </Panel>
  );
}

function LoadingLabel({ children }: { children: ReactNode }) {
  return (
    <>
      <LoaderCircle size={16} className="spin" />
      <span>{children}</span>
    </>
  );
}

function MetaLabel({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <span className={cn(mutedLabelClassName, className)}>{children}</span>;
}

function SideDrawer({
  ariaLabel,
  children,
  closeLabel,
  onClose,
  title,
  titleClassName = "text-lg",
}: {
  ariaLabel: string;
  children: ReactNode;
  closeLabel: string;
  onClose: () => void;
  title: string;
  titleClassName?: string;
}) {
  return (
    <div className="fixed inset-0 z-20">
      <button type="button" aria-label={closeLabel} className={sideDrawerBackdropClassName} onClick={onClose} />
      <aside
        className={sideDrawerPanelClassName}
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
      >
        <div className="flex min-h-8 items-center justify-between">
          <h2 className={cn("m-0 font-semibold", titleClassName)}>{title}</h2>
          <IconButton aria-label={closeLabel} onClick={onClose}>
            <X size={18} strokeWidth={2} />
          </IconButton>
        </div>
        {children}
      </aside>
    </div>
  );
}

function InstructionField({
  ariaLabel,
  className,
  placeholder,
  value,
  onChange,
}: {
  ariaLabel: string;
  className?: string;
  placeholder: string;
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label
      className={cn(
        "flex min-w-0 min-h-0 flex-col gap-2.5 rounded-xl border border-[color:var(--border)] bg-[color:var(--muted-strong)] p-3",
        className,
      )}
    >
      <div className="flex flex-col gap-1">
        <span className="text-xs font-bold uppercase tracking-[0.04em] text-[color:var(--muted-text)]">
          {t("instruction.label")}
        </span>
        <span className="text-xs leading-[1.45] text-[color:var(--muted-text)]">{t("instruction.hint")}</span>
      </div>
      <textarea
        aria-label={ariaLabel}
        className="min-h-[72px] max-h-40 flex-1 resize-none overflow-auto border-0 bg-transparent text-[color:var(--text)] outline-none"
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function TextHomeScreen({
  languages,
  sourceLang,
  targetLang,
  instruction,
  textInput,
  textOutput,
  busy,
  modelLoading,
  onSourceChange,
  onTargetChange,
  onInstructionChange,
  onTextInputChange,
  onSwap,
  onTranslate,
  onShowHistory,
  onSwitchImage,
  onCopyInput,
  onCopyOutput,
}: {
  languages: LanguageOption[];
  sourceLang: string;
  targetLang: string;
  instruction: string;
  textInput: string;
  textOutput: string;
  busy: boolean;
  modelLoading: boolean;
  onSourceChange: (next: string) => void;
  onTargetChange: (next: string) => void;
  onInstructionChange: (next: string) => void;
  onTextInputChange: (next: string) => void;
  onSwap: () => void;
  onTranslate: () => void;
  onShowHistory: () => void;
  onSwitchImage: () => void;
  onCopyInput: () => void;
  onCopyOutput: () => void;
}) {
  const { t } = useTranslation();
  return (
    <SectionCard>
      <ScreenTopbar
        activeMode="text"
        onSwitchText={() => undefined}
        onSwitchImage={onSwitchImage}
        onShowHistory={onShowHistory}
      />

      <TranslationLanguageRow
        sourceValue={sourceLang}
        targetValue={targetLang}
        options={languages}
        onSourceChange={onSourceChange}
        onTargetChange={onTargetChange}
        onSwap={onSwap}
        sourceAllowsAuto={true}
      />

      <TranslationActionsRow
        leftAction={<Button onClick={onCopyInput}>{t("buttons.copy")}</Button>}
        centerAction={
          <Button variant="primary" onClick={onTranslate} disabled={busy || modelLoading}>
            {modelLoading ? (
              <LoadingLabel>{t("buttons.loadingModel")}</LoadingLabel>
            ) : busy ? (
              <LoadingLabel>{t("buttons.translating")}</LoadingLabel>
            ) : (
              t("buttons.translate")
            )}
          </Button>
        }
        rightAction={<Button onClick={onCopyOutput}>{t("buttons.copy")}</Button>}
      />

      <div className={editorGridClassName}>
        <Panel className="col-start-1 row-start-1 flex min-h-0 items-stretch overflow-hidden max-lg:col-auto max-lg:row-auto">
          <textarea
            aria-label={t("textScreen.sourceTextAria")}
            className="h-full min-h-0 w-full resize-none overflow-auto border-0 bg-transparent font-[var(--font-sans)] text-sm leading-[1.45] text-[color:var(--text)] outline-none"
            placeholder={t("textScreen.sourceTextPlaceholder")}
            value={textInput}
            onChange={(event) => onTextInputChange(event.target.value)}
          />
        </Panel>
        <OutputPanel value={textOutput} placeholder={t("textScreen.resultPlaceholder")} />
        <InstructionField
          className="col-start-1 row-start-2 max-lg:col-auto max-lg:row-auto"
          ariaLabel={t("textScreen.instructionAria")}
          placeholder={t("instruction.textPlaceholder")}
          value={instruction}
          onChange={onInstructionChange}
        />
      </div>
    </SectionCard>
  );
}

function ImageScreen({
  uploadInputRef,
  languages,
  sourceLang,
  targetLang,
  instruction,
  uploadedFileName,
  uploadedPreviewUrl,
  imageOutput,
  maxUploadMB,
  visionEnabled,
  needsModelSetup,
  busy,
  modelLoading,
  modelBusy,
  dragActive,
  onShowHistory,
  onSwitchText,
  onSourceChange,
  onTargetChange,
  onInstructionChange,
  onSwap,
  onUploadClick,
  onUploadChange,
  onUploadReset,
  onDragEnter,
  onDragOver,
  onDragLeave,
  onDrop,
  onTranslate,
  onCopy,
  onEnableVision,
}: {
  uploadInputRef: React.RefObject<HTMLInputElement | null>;
  languages: LanguageOption[];
  sourceLang: string;
  targetLang: string;
  instruction: string;
  uploadedFileName: string;
  uploadedPreviewUrl: string;
  imageOutput: string;
  maxUploadMB: number;
  visionEnabled: boolean;
  needsModelSetup: boolean;
  busy: boolean;
  modelLoading: boolean;
  modelBusy: boolean;
  dragActive: boolean;
  onShowHistory: () => void;
  onSwitchText: () => void;
  onSourceChange: (next: string) => void;
  onTargetChange: (next: string) => void;
  onInstructionChange: (next: string) => void;
  onSwap: () => void;
  onUploadClick: () => void;
  onUploadChange: (event: ChangeEvent<HTMLInputElement>) => void;
  onUploadReset: () => void;
  onDragEnter: (event: DragEvent<HTMLDivElement>) => void;
  onDragOver: (event: DragEvent<HTMLDivElement>) => void;
  onDragLeave: (event: DragEvent<HTMLDivElement>) => void;
  onDrop: (event: DragEvent<HTMLDivElement>) => void;
  onTranslate: () => void;
  onCopy: () => void;
  onEnableVision: () => void;
}) {
  const { t } = useTranslation();
  return (
    <SectionCard>
      <ScreenTopbar
        activeMode="image"
        onSwitchText={onSwitchText}
        onSwitchImage={() => undefined}
        onShowHistory={onShowHistory}
      />

      <TranslationLanguageRow
        sourceValue={sourceLang}
        targetValue={targetLang}
        options={languages}
        onSourceChange={onSourceChange}
        onTargetChange={onTargetChange}
        onSwap={onSwap}
        sourceAllowsAuto={true}
      />

      <TranslationActionsRow
        centerAction={
          <Button
            variant="primary"
            onClick={onTranslate}
            disabled={busy || modelLoading || (!visionEnabled && !needsModelSetup)}
          >
            {modelLoading ? (
              <LoadingLabel>{t("buttons.loadingModel")}</LoadingLabel>
            ) : busy ? (
              <LoadingLabel>{t("buttons.translating")}</LoadingLabel>
            ) : (
              t("buttons.translate")
            )}
          </Button>
        }
        rightAction={<Button onClick={onCopy}>{t("buttons.copy")}</Button>}
      />

      <div className={cn("editor-grid--image", editorGridClassName)}>
        <section
          className={cn(
            "upload-panel--image col-start-1 row-start-1 min-h-[clamp(220px,34vh,340px)] rounded-[20px] border border-[color:var(--border)] bg-[color:var(--card)] p-3.5",
            "flex min-h-0 h-auto flex-col items-center gap-3 text-center max-lg:col-auto max-lg:row-auto",
            uploadedPreviewUrl ? "justify-start" : "justify-center",
            visionEnabled && "relative border-dashed transition-[border-color,background-color,box-shadow]",
            dragActive && "border-[color:var(--primary)] bg-[color:var(--dropzone-active-bg)] shadow-[inset_0_0_0_1px_var(--dropzone-active-ring)]",
          )}
          role="region"
          aria-label={t("imageScreen.uploadAreaAria")}
          onDragEnter={visionEnabled ? onDragEnter : undefined}
          onDragOver={visionEnabled ? onDragOver : undefined}
          onDragLeave={visionEnabled ? onDragLeave : undefined}
          onDrop={visionEnabled ? onDrop : undefined}
        >
          {!visionEnabled ? (
            <div className="flex max-w-[360px] flex-col items-center gap-2.5">
              <p className="m-0 text-[15px] font-semibold">{t("imageScreen.requiresVisionTitle")}</p>
              <p className="m-0 text-[13px] text-[color:var(--muted-text)]">{t("imageScreen.requiresVisionHint")}</p>
              <Button variant="primary" onClick={onEnableVision} disabled={modelBusy}>
                {modelBusy ? (
                  <LoadingLabel>{t("buttons.switching")}</LoadingLabel>
                ) : (
                  t("buttons.switchToVision")
                )}
              </Button>
            </div>
          ) : (
            <>
              <input
                ref={uploadInputRef}
                aria-label={t("imageScreen.uploadInputAria")}
                className="visually-hidden"
                id="image-upload"
                type="file"
                accept=".png,.jpg,.jpeg,.gif"
                onChange={onUploadChange}
              />
              {uploadedPreviewUrl ? (
                <>
                  <div className="w-full max-w-[480px]">
                    <img
                      className="block w-full rounded-2xl object-contain"
                      src={uploadedPreviewUrl}
                      alt={
                        uploadedFileName
                          ? t("imageScreen.previewAlt", { name: uploadedFileName })
                          : t("imageScreen.selectedPreviewAlt")
                      }
                    />
                  </div>
                  {uploadedFileName ? <p className="m-0 text-[13px] text-[color:var(--muted-text)]">{uploadedFileName}</p> : null}
                  <p className="m-0 text-[13px] text-[color:var(--muted-text)]">{t("imageScreen.replaceHint")}</p>
                  <div className="flex flex-wrap justify-center gap-2.5">
                    <Button variant="primary" onClick={onUploadClick}>{t("buttons.chooseAnother")}</Button>
                    <Button onClick={onUploadReset}>{t("buttons.reset")}</Button>
                  </div>
                </>
              ) : (
                <>
                  <ImageUp size={30} strokeWidth={1.8} />
                  <p className="m-0 text-[15px] font-semibold">{t("imageScreen.uploadTitle")}</p>
                  <p className="m-0 text-[13px] text-[color:var(--muted-text)]">
                    {t("imageScreen.uploadHint", { maxUploadMB })}
                  </p>
                  <Button variant="primary" onClick={onUploadClick}>{t("buttons.uploadImage")}</Button>
                </>
              )}
            </>
          )}
        </section>

        <OutputPanel value={imageOutput} placeholder={t("textScreen.resultPlaceholder")} />
        <InstructionField
          className="instruction-card--inline col-start-1 row-start-2 max-lg:col-auto max-lg:row-auto"
          ariaLabel={t("imageScreen.instructionAria")}
          placeholder={t("instruction.imagePlaceholder")}
          value={instruction}
          onChange={onInstructionChange}
        />
      </div>
    </SectionCard>
  );
}

function DownloadProgressCard({
  download,
  onCancel,
  compact = false,
}: {
  download: ModelDownloadState;
  onCancel: () => void;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const progressPercent = clampPercent(download.percent);
  const totalLabel = download.totalBytes > 0 ? formatBinarySize(download.totalBytes) : "--";

  return (
    <div
      className={cn(
        "flex flex-col gap-2.5 rounded-xl border border-[color:var(--primary-soft-border)] bg-[color:var(--primary-soft-bg)] shadow-[var(--download-shadow)]",
        compact ? "p-3" : "p-3.5",
      )}
      role="status"
      aria-live="polite"
      aria-label={t("model.downloadProgressAria", { defaultValue: "Model download progress" })}
    >
      <div className="flex items-start justify-between gap-3 max-md:flex-col max-md:items-stretch">
        <div className="min-w-0 flex-1">
          <span className="text-[11px] font-bold uppercase tracking-[0.04em] text-[color:var(--primary)]">
            {t("model.downloadProgress", { defaultValue: "Downloading model" })}
          </span>
          <strong className="block overflow-wrap-anywhere text-sm font-semibold text-[color:var(--text)]">{download.modelName}</strong>
        </div>
        <Button
          variant="danger"
          className="self-start whitespace-nowrap"
          onClick={onCancel}
          disabled={download.canceling}
        >
          {download.canceling
            ? t("buttons.cancelingDownload", { defaultValue: "Canceling..." })
            : t("buttons.cancelDownload", { defaultValue: "Cancel" })}
        </Button>
      </div>
      <div className="h-2.5 overflow-hidden rounded-full bg-[color:var(--download-track)]" aria-hidden="true">
        <span
          className="block h-full rounded-[inherit] bg-[linear-gradient(90deg,var(--download-bar-start)_0%,var(--download-bar-end)_100%)] transition-[width] duration-200"
          style={{ width: `${progressPercent}%` }}
        />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2.5 text-xs font-semibold text-[color:var(--text-soft)]">
        <span>{progressPercent.toFixed(progressPercent >= 10 ? 0 : 1)}%</span>
        <span>
          {formatBinarySize(download.downloadedBytes)} / {totalLabel}
        </span>
        <span>{formatTransferRate(download.speedBytesPerSecond)}</span>
      </div>
      <p className="m-0 text-xs leading-[1.45] text-[color:var(--muted-text)]">{download.message}</p>
    </div>
  );
}

function ModelDrawer({
  open,
  models,
  onClose,
  onUseNow,
  onDelete,
  onDownload,
  onCancelDownload,
  busyModelId,
  downloadState,
}: {
  open: boolean;
  models: ModelItem[];
  onClose: () => void;
  onUseNow: (id: string) => void;
  onDelete: (id: string) => void;
  onDownload: (id: string) => void;
  onCancelDownload: () => void;
  busyModelId: string | null;
  downloadState: ModelDownloadState | null;
}) {
  const { t } = useTranslation();
  if (!open) {
    return null;
  }

  return (
    <SideDrawer
      ariaLabel={t("model.drawerTitle")}
      closeLabel={t("model.closeDrawer")}
      onClose={onClose}
      title={t("model.drawerTitle")}
    >
      <ScrollView ariaLabel={t("model.listAria")}>
        {models.map((model) => {
          const busy = busyModelId === model.id;
          const activeDownload = downloadState?.modelId === model.id ? downloadState : null;
          const showDownloadProgress = !!activeDownload;
          const modelState = describeModelState(model, false);
          const actionVariant = model.active ? "success" : model.selected ? "secondary" : "primary";
          const actionLabel = busy
            ? showDownloadProgress
              ? t("buttons.downloading", { defaultValue: "Downloading" })
              : t("buttons.loadingModel")
            : model.active
              ? t("buttons.active")
              : model.selected
                ? t("buttons.selected")
                : t("buttons.useNow");
          return (
            <div
              key={model.id}
              className={cn(
                "flex flex-col gap-2 rounded-[10px] border p-3 text-left transition-colors",
                model.active
                  ? "border-[color:var(--primary)] bg-[color:var(--muted)]"
                  : "border-[color:var(--border)] bg-[color:var(--card)]",
              )}
            >
              <div className="flex items-start justify-between gap-3">
                <span className="text-sm font-semibold leading-5 break-words text-[color:var(--text)]">{model.fileName}</span>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <StatusBadge tone={modelState.tone}>{t(modelState.labelKey)}</StatusBadge>
                <StatusBadge tone={model.visionCapable ? "accent" : "neutral"}>
                  {model.visionCapable ? t("model.capabilityVision") : t("model.capabilityText")}
                </StatusBadge>
                {model.recommended ? <StatusBadge>{t("model.recommended")}</StatusBadge> : null}
              </div>
              <span className="text-xs font-medium text-[color:var(--muted-text)]">
                {model.size} · {model.installed ? t("model.sourceInstalled") : t("model.sourceRemote")}
              </span>
              {model.installed ? (
                <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-stretch gap-2">
                  <Button
                    variant={actionVariant}
                    className="w-full"
                    onClick={() => onUseNow(model.id)}
                    disabled={busy || model.selected}
                  >
                    {busy ? <LoadingLabel>{actionLabel}</LoadingLabel> : <span>{actionLabel}</span>}
                  </Button>
                  <Button
                    variant="danger"
                    className="justify-self-end"
                    onClick={() => onDelete(model.id)}
                    disabled={busy}
                  >
                    <Trash2 size={16} strokeWidth={2} />
                    <span>{t("buttons.delete")}</span>
                  </Button>
                </div>
              ) : activeDownload ? (
                <DownloadProgressCard download={activeDownload} onCancel={onCancelDownload} compact />
              ) : (
                <Button
                  variant="primary"
                  className="w-full"
                  onClick={() => onDownload(model.id)}
                  disabled={busy}
                >
                  {busy ? (
                    <LoadingLabel>{t("buttons.download")}</LoadingLabel>
                  ) : (
                    <>
                      <Download size={16} strokeWidth={2} />
                      <span>{t("buttons.download")}</span>
                    </>
                  )}
                </Button>
              )}
            </div>
          );
        })}
      </ScrollView>
    </SideDrawer>
  );
}

function HistoryDrawer({
  open,
  entries,
  languageLabel,
  onClose,
  onDelete,
  onClear,
  onOpen,
  clearing,
}: {
  open: boolean;
  entries: HistoryEntry[];
  languageLabel: (code: string) => string;
  onClose: () => void;
  onDelete: (id: number) => void;
  onClear: () => void;
  onOpen: (id: number) => void;
  clearing: boolean;
}) {
  const { t } = useTranslation();
  if (!open) {
    return null;
  }

  return (
    <SideDrawer
      ariaLabel={t("history.drawerTitle")}
      closeLabel={t("history.closeDrawer")}
      onClose={onClose}
      title={t("history.drawerTitle")}
      titleClassName="text-xl"
    >
      <ScrollView ariaLabel={t("history.listAria")}>
        {entries.map((entry) => (
          <button
            type="button"
            key={entry.id}
            className="flex w-full flex-col gap-2.5 rounded-lg border border-[color:var(--border)] bg-[color:var(--card)] p-3 text-left transition-colors hover:bg-[color:var(--muted)]"
            onClick={() => onOpen(entry.id)}
          >
            <div className="flex items-center justify-between gap-3">
              <span className="text-[13px] font-semibold">{entry.when}</span>
              <span
                className="inline-flex min-h-6 items-center gap-1 rounded-full border border-[color:var(--danger-border)] bg-[color:var(--danger-bg)] px-2 py-1 text-xs font-medium text-[color:var(--danger-text)]"
                onClick={(event) => {
                  event.stopPropagation();
                  onDelete(entry.id);
                }}
              >
                <Trash2 size={14} strokeWidth={2} />
                <span>{t("buttons.delete")}</span>
              </span>
            </div>
            <div className="flex items-center gap-3">
              <div className="flex flex-1 flex-col gap-0.5">
                <MetaLabel>{t("history.source")}</MetaLabel>
                <span
                  className="[display:-webkit-box] overflow-hidden text-xs font-medium leading-[1.35] [-webkit-box-orient:vertical] [-webkit-line-clamp:2]"
                  title={languageLabel(entry.source)}
                >
                  {languageLabel(entry.source)}
                </span>
              </div>
              <div className="flex flex-1 flex-col gap-0.5">
                <MetaLabel>{t("history.target")}</MetaLabel>
                <span
                  className="[display:-webkit-box] overflow-hidden text-xs font-medium leading-[1.35] [-webkit-box-orient:vertical] [-webkit-line-clamp:2]"
                  title={languageLabel(entry.target)}
                >
                  {languageLabel(entry.target)}
                </span>
              </div>
            </div>
            <div className="flex flex-col gap-1">
              <MetaLabel>{t("history.sourceText")}</MetaLabel>
              <p
                className="[display:-webkit-box] overflow-hidden whitespace-normal text-xs leading-[1.45] text-[color:var(--muted-text)] [-webkit-box-orient:vertical] [-webkit-line-clamp:2]"
                title={entry.input}
              >
                {entry.input}
              </p>
            </div>
            <div className="flex flex-col gap-1">
              <MetaLabel>{t("history.targetText")}</MetaLabel>
              <p
                className="[display:-webkit-box] overflow-hidden whitespace-normal text-xs leading-[1.45] text-[color:var(--text)] [-webkit-box-orient:vertical] [-webkit-line-clamp:2]"
                title={entry.output}
              >
                {entry.output}
              </p>
            </div>
          </button>
        ))}
      </ScrollView>
      <div className="flex items-center gap-3 pt-1">
        <Button variant="danger" className="w-full" onClick={onClear} disabled={clearing || entries.length === 0}>
          {clearing ? (
            <LoadingLabel>{t("buttons.clearingHistory")}</LoadingLabel>
          ) : (
            t("buttons.clearHistory")
          )}
        </Button>
      </div>
    </SideDrawer>
  );
}

function HistoryDetailModal({
  entry,
  languageLabel,
  onClose,
  onCopy,
}: {
  entry: HistoryEntry | null;
  languageLabel: (code: string) => string;
  onClose: () => void;
  onCopy: () => void;
}) {
  const { t } = useTranslation();
  if (!entry) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-20 flex items-center justify-center overflow-auto p-6">
      <div className="absolute inset-0 bg-[color:var(--overlay-solid)]" onClick={onClose} />
      <div
        className="relative z-10 m-0 max-h-[calc(100vh-48px)] w-[min(985px,calc(100vw-48px))] overflow-y-auto rounded-lg border border-[color:var(--border)] bg-[color:var(--drawer-surface)] p-6 shadow-[var(--panel-shadow)] max-md:p-4"
        role="dialog"
        aria-modal="true"
        aria-label={t("history.detailTitle")}
      >
        <div className="mb-3 flex items-start justify-between gap-3 max-md:flex-col max-md:items-stretch">
          <div className="flex w-[220px] flex-col gap-1.5 max-md:w-full">
            <MetaLabel>{t("history.translationTime")}</MetaLabel>
            <strong>{entry.when}</strong>
          </div>
          <OutlineIconButton aria-label={t("history.closeDetail")} onClick={onClose}>
            <X size={16} strokeWidth={2} />
          </OutlineIconButton>
        </div>

        <div className="mb-3 flex items-center gap-3 max-md:flex-col max-md:items-stretch">
          <div className="flex flex-1 flex-col gap-1 rounded-md bg-[color:var(--muted)] p-3">
            <MetaLabel>{t("history.source")}</MetaLabel>
            <strong className="text-sm font-normal">{languageLabel(entry.source)}</strong>
          </div>
          <div className="flex flex-1 flex-col gap-1 rounded-md bg-[color:var(--muted)] p-3">
            <MetaLabel>{t("history.target")}</MetaLabel>
            <strong className="text-sm font-normal">{languageLabel(entry.target)}</strong>
          </div>
        </div>

        <div className="mb-3 flex flex-col gap-2">
          <MetaLabel>{t("history.sourceText")}</MetaLabel>
          <div className="max-h-[32vh] overflow-auto rounded-md bg-[color:var(--muted)] p-3">
            <p className={detailTextClassName}>{entry.input}</p>
          </div>
        </div>

        <div className="mb-3 flex flex-col gap-2">
          <MetaLabel>{t("history.targetText")}</MetaLabel>
          <div className="max-h-[32vh] overflow-auto rounded-md bg-[color:var(--muted)] p-3">
            <p className={detailTextClassName}>{entry.output}</p>
          </div>
        </div>

        <div className="flex justify-end pt-1">
          <Button variant="primary" onClick={onCopy}>{t("buttons.copyResult")}</Button>
        </div>
      </div>
    </div>
  );
}

async function fetchBootstrap(): Promise<BootstrapState> {
  const response = await fetch("/api/bootstrap");
  if (!response.ok) {
    throw new Error(await response.text() || "failed to load bootstrap state");
  }
  return (await response.json()) as BootstrapState;
}

async function streamJsonLines(
  url: string,
  body: URLSearchParams | undefined,
  onEvent: (event: StreamEvent) => void,
  signal?: AbortSignal,
) {
  const response = await fetch(url, {
    method: "POST",
    body,
    signal,
  });
  if (!response.ok || !response.body) {
    throw new Error((await response.text()) || "request failed");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const chunk = await reader.read();
    if (chunk.done) {
      break;
    }
    buffer += decoder.decode(chunk.value, { stream: true });
    let boundary = buffer.indexOf("\n");
    while (boundary >= 0) {
      const line = buffer.slice(0, boundary).trim();
      buffer = buffer.slice(boundary + 1);
      if (line) {
        onEvent(JSON.parse(line) as StreamEvent);
      }
      boundary = buffer.indexOf("\n");
    }
  }

  const tail = buffer.trim();
  if (tail) {
    onEvent(JSON.parse(tail) as StreamEvent);
  }
}

function App() {
  const { t, i18n } = useTranslation();
  const uploadRef = useRef<HTMLInputElement | null>(null);
  const modelActionAbortRef = useRef<AbortController | null>(null);
  const [screenMode, setScreenMode] = useState<ScreenMode>("text");
  const [drawer, setDrawer] = useState<DrawerKind>(null);
  const [models, setModels] = useState<ModelItem[]>([]);
  const [languages, setLanguages] = useState<LanguageOption[]>([]);
  const [historyEntries, setHistoryEntries] = useState<HistoryEntry[]>([]);
  const [, setHistoryCount] = useState(0);
  const [selectedHistoryId, setSelectedHistoryId] = useState<number | null>(null);
  const [textSourceLang, setTextSourceLang] = useState("auto");
  const [textTargetLang, setTextTargetLang] = useState("zh-CN");
  const [textInstruction, setTextInstruction] = useState("");
  const [textInput, setTextInput] = useState("");
  const [textOutput, setTextOutput] = useState("");
  const [fileSourceLang, setFileSourceLang] = useState("auto");
  const [fileTargetLang, setFileTargetLang] = useState("zh-CN");
  const [fileInstruction, setFileInstruction] = useState("");
  const [fileOutput, setFileOutput] = useState("");
  const [uploadedFile, setUploadedFile] = useState<File | null>(null);
  const [uploadedPreviewUrl, setUploadedPreviewUrl] = useState("");
  const [statusState, setStatusState] = useState<StatusState | null>(null);
  const [needsModelSetup, setNeedsModelSetup] = useState(false);
  const [visionEnabled, setVisionEnabled] = useState(false);
  const [maxUploadMB, setMaxUploadMB] = useState(10);
  const [loading, setLoading] = useState(true);
  const [busyModelId, setBusyModelId] = useState<string | null>(null);
  const [pendingModelAction, setPendingModelAction] = useState<string | null>(null);
  const [downloadState, setDownloadState] = useState<ModelDownloadState | null>(null);
  const [busyText, setBusyText] = useState(false);
  const [busyImage, setBusyImage] = useState(false);
  const [imageDragActive, setImageDragActive] = useState(false);
  const [translationModelLoading, setTranslationModelLoading] = useState(false);
  const [clearingHistory, setClearingHistory] = useState(false);
  const [themePreference, setThemePreference] = useState<AppTheme>(() => detectInitialAppTheme());
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedAppTheme>(() => readAppliedAppTheme());

  function applyServerStatus(code?: string, fallback?: string, isErrorFallback = false) {
    setStatusState({
      code,
      fallback,
      isError: code ? isErrorStatusCode(code) : isErrorFallback,
    });
  }

  function applyClientError(message: string) {
    setStatusState({ fallback: message, isError: true });
  }

  async function refreshState(preserveDrafts = true) {
    const next = await fetchBootstrap();
    setModels(next.models);
    setLanguages(next.languages);
    setHistoryEntries(next.history);
    setHistoryCount(next.historyCount);
    applyServerStatus(next.statusCode, next.status);
    setNeedsModelSetup(next.needsModelSetup);
    setVisionEnabled(next.visionEnabled);
    setMaxUploadMB(next.maxUploadMB);
    if (!preserveDrafts) {
      setScreenMode(next.activeTab);
      setTextSourceLang(next.textSourceLang);
      setTextTargetLang(next.textTargetLang);
      setTextInstruction(next.textInstruction);
      setTextInput(next.textInput);
      setTextOutput(next.textOutput);
      setFileSourceLang(next.fileSourceLang);
      setFileTargetLang(next.fileTargetLang);
      setFileInstruction(next.fileInstruction);
      setFileOutput(next.fileOutput);
    }
  }

  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const next = await fetchBootstrap();
        if (cancelled) {
          return;
        }
        setModels(next.models);
        setLanguages(next.languages);
        setHistoryEntries(next.history);
        setHistoryCount(next.historyCount);
        setScreenMode(next.activeTab);
        setTextSourceLang(next.textSourceLang);
        setTextTargetLang(next.textTargetLang);
        setTextInstruction(next.textInstruction);
        setTextInput(next.textInput);
        setTextOutput(next.textOutput);
        setFileSourceLang(next.fileSourceLang);
        setFileTargetLang(next.fileTargetLang);
        setFileInstruction(next.fileInstruction);
        setFileOutput(next.fileOutput);
        applyServerStatus(next.statusCode, next.status);
        setNeedsModelSetup(next.needsModelSetup);
        setVisionEnabled(next.visionEnabled);
        setMaxUploadMB(next.maxUploadMB);
      } catch (error) {
        if (!cancelled) {
          applyClientError(
            error instanceof Error ? error.message : "failed to load bootstrap state",
          );
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [i18n]);

  useEffect(() => {
    if (!uploadedFile) {
      setUploadedPreviewUrl("");
      return;
    }
    if (typeof URL === "undefined" || typeof URL.createObjectURL !== "function") {
      setUploadedPreviewUrl("");
      return;
    }

    const nextUrl = URL.createObjectURL(uploadedFile);
    setUploadedPreviewUrl(nextUrl);

    return () => {
      if (typeof URL.revokeObjectURL === "function") {
        URL.revokeObjectURL(nextUrl);
      }
    };
  }, [uploadedFile]);

  const selectedHistory = historyEntries.find((entry) => entry.id === selectedHistoryId) ?? null;
  const statusMessage = statusState ? resolveServerMessage(statusState.code, statusState.fallback, t) : "";
  const showStatusBanner = shouldShowStatusBanner(statusMessage, statusState?.isError === true);
  const selectedModelNeedsLoad = models.some((item) => item.selected && !item.loaded);
  const actionModelLoading = pendingModelAction === "/api/models/activate" || pendingModelAction === "/api/models/enable-vision";
  const modelLoading = actionModelLoading || translationModelLoading;
  const visionSwitchBusy = pendingModelAction === "/api/models/enable-vision";
  const currentLocale = normalizeAppLocale(i18n.resolvedLanguage) ?? "en";

  useEffect(() => () => {
    modelActionAbortRef.current?.abort();
  }, []);

  useEffect(() => {
    document.title = t("app.title");
  }, [i18n.resolvedLanguage, t]);

  useEffect(() => {
    setResolvedTheme(applyAppTheme(themePreference));
  }, [themePreference]);

  useEffect(() => {
    if (themePreference !== "system") {
      return () => undefined;
    }
    return watchSystemTheme(() => {
      setResolvedTheme(applyAppTheme("system"));
    });
  }, [themePreference]);

  function languageLabel(code: string) {
    const option = languages.find((language) => language.code === code);
    if (!option) {
      return code;
    }
    return localizedLanguageLabel(option, currentLocale);
  }

  function upsertHistory(entry?: HistoryEntry, count?: number) {
    if (!entry) {
      return;
    }
    setHistoryEntries((current) => {
      const filtered = current.filter((item) => item.id !== entry.id);
      return [entry, ...filtered].slice(0, maxHistoryEntries);
    });
    if (typeof count === "number") {
      setHistoryCount(count);
    }
  }

  function markSelectedModelLoaded() {
    setModels((current) =>
      current.map((item) => ({
        ...item,
        active: item.selected,
        loaded: item.selected,
      })),
    );
  }

  function findModelForAction(url: string, modelId?: string) {
    const resolvedModelId = modelId ?? (url === "/api/models/enable-vision" ? "q8_0_vision" : "");
    if (!resolvedModelId) {
      return null;
    }
    return models.find((item) => item.id === resolvedModelId) ?? null;
  }

  function shouldTrackDownload(url: string, modelId?: string) {
    const model = findModelForAction(url, modelId);
    if (!model) {
      return false;
    }
    if (url === "/api/models/install") {
      return true;
    }
    if (url === "/api/models/enable-vision") {
      return !model.installed;
    }
    return false;
  }

  function startDownloadTracking(url: string, modelId?: string) {
    const model = findModelForAction(url, modelId);
    if (!model) {
      return;
    }
    setDownloadState({
      modelId: model.id,
      modelName: model.fileName,
      message:
        url === "/api/models/enable-vision"
          ? t("serverStatus.downloading_vision_runtime")
          : t("serverStatus.preparing_model_install"),
      percent: 0,
      downloadedBytes: 0,
      totalBytes: 0,
      speedBytesPerSecond: 0,
      canceling: false,
    });
  }

  function updateDownloadTracking(url: string, modelId: string | undefined, event: StreamEvent) {
    const model = findModelForAction(url, modelId);
    if (!model) {
      return;
    }
    const isDownloadStage = event.stage === "download";
    const message = resolveServerMessage(event.messageCode, event.message, t);
    setDownloadState((current) => ({
      modelId: model.id,
      modelName: model.fileName,
      message: message || current?.message || "",
      percent: isDownloadStage
        ? clampPercent(event.percent ?? current?.percent ?? 0)
        : current?.percent ?? clampPercent(event.percent ?? 0),
      downloadedBytes: isDownloadStage
        ? event.downloadedBytes ?? current?.downloadedBytes ?? 0
        : current?.downloadedBytes ?? 0,
      totalBytes: isDownloadStage ? event.totalBytes ?? current?.totalBytes ?? 0 : current?.totalBytes ?? 0,
      speedBytesPerSecond: isDownloadStage ? event.speedBytesPerSecond ?? current?.speedBytesPerSecond ?? 0 : 0,
      canceling: current?.canceling ?? false,
    }));
  }

  async function handleCopy(value: string) {
    if (!value.trim()) {
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      return;
    }
  }

  async function handleTextTranslate() {
    if (needsModelSetup) {
      setDrawer("model");
      return;
    }
    if (!textInput.trim() || busyText || modelLoading) {
      return;
    }

    const shouldLoadModel = selectedModelNeedsLoad;
    let modelMarkedLoaded = !shouldLoadModel;
    setBusyText(true);
    setTextOutput("");
    if (shouldLoadModel) {
      setTranslationModelLoading(true);
    }

    try {
      await streamJsonLines(
        "/api/translate/stream",
        new URLSearchParams({
          source_lang: textSourceLang,
          target_lang: textTargetLang,
          translation_instruction: textInstruction,
          input_text: textInput,
        }),
        (event) => {
          const resolveModelLoad = () => {
            if (modelMarkedLoaded) {
              return;
            }
            modelMarkedLoaded = true;
            setTranslationModelLoading(false);
            markSelectedModelLoaded();
          };
          if ((event.type === "status" || event.type === "progress") && event.stage !== "load") {
            resolveModelLoad();
          }
          if (event.type === "delta" && event.delta) {
            resolveModelLoad();
            setTextOutput((current) => current + event.delta);
            return;
          }
          if (event.type === "done") {
            resolveModelLoad();
            if (event.message) {
              applyServerStatus(event.messageCode, event.message);
            }
            if (event.output) {
              setTextOutput(event.output);
            }
            upsertHistory(event.history, event.count);
          }
          if (event.type === "error") {
            throw new Error(event.message || t("errors.streamFailed"));
          }
        },
      );
    } catch (error) {
      applyClientError(error instanceof Error ? error.message : "stream failed");
    } finally {
      setTranslationModelLoading(false);
      setBusyText(false);
    }
  }

  async function handleImageTranslate() {
    if (needsModelSetup) {
      setDrawer("model");
      return;
    }
    if (!uploadedFile || busyImage || modelLoading || !visionEnabled) {
      return;
    }

    const shouldLoadModel = selectedModelNeedsLoad;
    setBusyImage(true);
    setFileOutput("");
    if (shouldLoadModel) {
      setTranslationModelLoading(true);
    }

    const payload = new FormData();
    payload.append("source_lang", fileSourceLang);
    payload.append("target_lang", fileTargetLang);
    payload.append("translation_instruction", fileInstruction);
    payload.append("image_file", uploadedFile);

    try {
      const response = await fetch("/api/translate/image", {
        method: "POST",
        body: payload,
      });
      const result = (await response.json()) as ImageResult;
      if (!response.ok || !result.ok) {
        throw new Error(result.message || t("errors.imageTranslationFailed"));
      }
      if (shouldLoadModel) {
        markSelectedModelLoaded();
      }
      setFileOutput(result.output || "");
      applyServerStatus(result.messageCode, result.message || "File translation completed");
      upsertHistory(result.history, result.count);
    } catch (error) {
      applyClientError(error instanceof Error ? error.message : "image translation failed");
    } finally {
      setTranslationModelLoading(false);
      setBusyImage(false);
    }
  }

  function applyUploadedFile(file: File | null) {
    if (!file) {
      return;
    }
    setUploadedFile(file);
    setFileOutput("");
    setImageDragActive(false);
  }

  function resetUploadedFile() {
    setUploadedFile(null);
    setUploadedPreviewUrl("");
    setFileOutput("");
    setImageDragActive(false);
    if (uploadRef.current) {
      uploadRef.current.value = "";
    }
  }

  function openUploadPicker() {
    if (!uploadRef.current) {
      return;
    }
    uploadRef.current.value = "";
    uploadRef.current.click();
  }

  async function runModelAction(url: string, modelId?: string) {
    if (busyModelId) {
      return;
    }
    const tracksDownload = shouldTrackDownload(url, modelId);
    const abortController = tracksDownload ? new AbortController() : null;
    modelActionAbortRef.current = abortController;
    if (url === "/api/models/enable-vision" && tracksDownload) {
      setDrawer("model");
    }
    if (url === "/api/models/activate" && modelId) {
      setModels((current) =>
        current.map((item) => ({
          ...item,
          active: false,
          selected: item.id === modelId,
          loaded: false,
        })),
      );
    }
    if (url === "/api/models/enable-vision") {
      setVisionEnabled(true);
      setModels((current) =>
        current.map((item) => ({
          ...item,
          active: false,
          selected: item.id === "q8_0_vision",
          loaded: false,
        })),
      );
    }
    if (tracksDownload) {
      startDownloadTracking(url, modelId);
    }
    setBusyModelId(modelId || "__global__");
    setPendingModelAction(url);
    try {
      await streamJsonLines(
        url,
        modelId ? new URLSearchParams({ model_id: modelId }) : undefined,
        (event) => {
          if (event.type === "error") {
            throw new StreamEventError(event.message || t("errors.modelActionFailed"), event.messageCode);
          }
          if (tracksDownload) {
            updateDownloadTracking(url, modelId, event);
          }
          if (event.type === "done") {
            setDownloadState(null);
          }
        },
        abortController?.signal,
      );
      await refreshState(true);
      if (url === "/api/models/enable-vision") {
        setScreenMode("image");
      }
    } catch (error) {
      setDownloadState(null);
      await refreshState(true).catch(() => undefined);
      if (isAbortError(error)) {
        return;
      }
      if (error instanceof StreamEventError && error.code) {
        applyServerStatus(error.code, error.message, true);
        return;
      }
      applyClientError(error instanceof Error ? error.message : "model action failed");
    } finally {
      if (modelActionAbortRef.current === abortController) {
        modelActionAbortRef.current = null;
      }
      setPendingModelAction(null);
      setBusyModelId(null);
    }
  }

  async function handleDeleteHistory(id: number) {
    const response = await fetch("/api/history/delete", {
      method: "POST",
      body: new URLSearchParams({ history_id: String(id) }),
    });
    const payload = (await response.json()) as HistoryDeleteResponse;
    if (!response.ok || !payload.ok) {
      applyServerStatus(
        payload.statusCode,
        payload.status || "history delete failed",
        true,
      );
      return;
    }
    applyServerStatus(payload.statusCode, payload.status);
    setHistoryEntries((current) => current.filter((item) => item.id !== id));
    setHistoryCount(payload.count);
    if (selectedHistoryId === id) {
      setSelectedHistoryId(null);
    }
  }

  async function handleClearHistory() {
    if (clearingHistory) {
      return;
    }
    setClearingHistory(true);
    try {
      const response = await fetch("/api/history/clear", {
        method: "POST",
      });
      const payload = (await response.json()) as HistoryDeleteResponse;
      if (!response.ok || !payload.ok) {
        applyServerStatus(
          payload.statusCode,
          payload.status || "history clear failed",
          true,
        );
        return;
      }
      applyServerStatus(payload.statusCode, payload.status);
      setHistoryEntries([]);
      setHistoryCount(payload.count);
      setSelectedHistoryId(null);
    } finally {
      setClearingHistory(false);
    }
  }

  function handleCancelDownload() {
    if (!downloadState || !modelActionAbortRef.current) {
      return;
    }
    setDownloadState((current) => (current ? { ...current, canceling: true } : current));
    modelActionAbortRef.current.abort();
  }

  function handleToggleTheme() {
    const nextTheme = resolvedTheme === "dark" ? "light" : "dark";
    setThemePreference(nextTheme);
    setResolvedTheme(setAppTheme(nextTheme));
  }

  if (loading) {
    return (
      <div className="app-shell min-h-screen bg-background text-foreground">
        <div className="inline-flex items-center gap-2.5 rounded-xl border border-[color:var(--border)] bg-[color:var(--muted)] px-3.5 py-3 text-[13px] leading-[1.45] text-[color:var(--muted-text)]">
          <LoaderCircle size={18} className="spin" />
          <span>{t("app.loading")}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="app-shell min-h-screen bg-background text-foreground">
      <header className="mx-auto mb-4 grid min-h-14 w-full max-w-[1200px] grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-3 max-md:flex max-md:flex-col max-md:items-start">
        <button
          type="button"
          className="min-w-0 border-0 bg-transparent p-0 text-left text-[22px] font-bold tracking-[-0.02em] text-[color:var(--text)]"
          onClick={() => setScreenMode("text")}
          aria-label={t("header.goToTextScreen")}
        >
          TranslateGemmaUI
        </button>

        <div className="flex min-w-0 max-w-[min(100%,420px)] items-center justify-end gap-3 max-md:w-full max-md:max-w-full max-md:flex-wrap max-md:justify-start">
          <AppLocaleField value={currentLocale} onChange={(nextLocale) => void setAppLocale(nextLocale)} />
          <ThemeToggle resolvedTheme={resolvedTheme} onToggle={handleToggleTheme} />
          <Button onClick={() => setDrawer("model")}>{t("header.model")}</Button>
        </div>
      </header>

      {showStatusBanner ? (
        <div className="mx-auto mb-3 w-full max-w-[1200px] rounded-xl border border-[color:var(--danger-border)] bg-[color:var(--danger-bg)] px-3.5 py-3 text-[13px] leading-[1.45] text-[color:var(--danger-text)]">
          {statusMessage}
        </div>
      ) : null}

      <main className="mx-auto w-full max-w-[1200px]">
        {screenMode === "text" ? (
          <TextHomeScreen
            languages={languages}
            sourceLang={textSourceLang}
            targetLang={textTargetLang}
            instruction={textInstruction}
            textInput={textInput}
            textOutput={textOutput}
            busy={busyText}
            modelLoading={modelLoading}
            onSourceChange={setTextSourceLang}
            onTargetChange={setTextTargetLang}
            onInstructionChange={setTextInstruction}
            onTextInputChange={setTextInput}
            onSwap={() => {
              setTextSourceLang(textTargetLang);
              setTextTargetLang(textSourceLang === "auto" ? "en" : textSourceLang);
            }}
            onTranslate={() => void handleTextTranslate()}
            onShowHistory={() => setDrawer("history")}
            onSwitchImage={() => setScreenMode("image")}
            onCopyInput={() => void handleCopy(textInput)}
            onCopyOutput={() => void handleCopy(textOutput)}
          />
        ) : (
          <ImageScreen
            uploadInputRef={uploadRef}
            languages={languages}
            sourceLang={fileSourceLang}
            targetLang={fileTargetLang}
            instruction={fileInstruction}
            uploadedFileName={uploadedFile?.name ?? ""}
            uploadedPreviewUrl={uploadedPreviewUrl}
            imageOutput={fileOutput}
            maxUploadMB={maxUploadMB}
            visionEnabled={visionEnabled}
            needsModelSetup={needsModelSetup}
            busy={busyImage}
            modelLoading={modelLoading}
            modelBusy={visionSwitchBusy}
            dragActive={imageDragActive}
            onShowHistory={() => setDrawer("history")}
            onSwitchText={() => setScreenMode("text")}
            onSourceChange={setFileSourceLang}
            onTargetChange={setFileTargetLang}
            onInstructionChange={setFileInstruction}
            onSwap={() => {
              setFileSourceLang(fileTargetLang);
              setFileTargetLang(fileSourceLang === "auto" ? "en" : fileSourceLang);
            }}
            onUploadClick={openUploadPicker}
            onUploadChange={(event) => {
              applyUploadedFile(event.target.files?.[0] ?? null);
            }}
            onUploadReset={resetUploadedFile}
            onDragEnter={(event) => {
              event.preventDefault();
              setImageDragActive(true);
            }}
            onDragOver={(event) => {
              event.preventDefault();
              event.dataTransfer.dropEffect = "copy";
              setImageDragActive(true);
            }}
            onDragLeave={(event) => {
              event.preventDefault();
              const nextTarget = event.relatedTarget;
              if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) {
                return;
              }
              setImageDragActive(false);
            }}
            onDrop={(event) => {
              event.preventDefault();
              const file = event.dataTransfer.files?.[0] ?? null;
              applyUploadedFile(file);
            }}
            onTranslate={() => void handleImageTranslate()}
            onCopy={() => void handleCopy(fileOutput)}
            onEnableVision={() => void runModelAction("/api/models/enable-vision")}
          />
        )}
      </main>

      <ModelDrawer
        open={drawer === "model"}
        models={models}
        onClose={() => setDrawer(null)}
        onUseNow={(id) => void runModelAction("/api/models/activate", id)}
        onDelete={(id) => void runModelAction("/api/models/delete", id)}
        onDownload={(id) => void runModelAction("/api/models/install", id)}
        onCancelDownload={handleCancelDownload}
        busyModelId={busyModelId}
        downloadState={downloadState}
      />

      <HistoryDrawer
        open={drawer === "history"}
        entries={historyEntries}
        languageLabel={languageLabel}
        onClose={() => setDrawer(null)}
        onDelete={(id) => void handleDeleteHistory(id)}
        onClear={() => void handleClearHistory()}
        onOpen={(id) => setSelectedHistoryId(id)}
        clearing={clearingHistory}
      />

      <HistoryDetailModal
        entry={selectedHistory}
        languageLabel={languageLabel}
        onClose={() => setSelectedHistoryId(null)}
        onCopy={() => void handleCopy(selectedHistory?.output ?? "")}
      />
    </div>
  );
}

export default App;
