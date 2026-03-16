import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
  type ButtonHTMLAttributes,
  type ReactNode,
  type RefObject,
  type TextareaHTMLAttributes,
} from "react";
import {
  ArrowLeftRight,
  ChevronDown,
  Download,
  ImageUp,
  LoaderCircle,
  Trash2,
  X,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { normalizeAppLocale, setAppLocale, type AppLocale } from "./i18n";
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
  return (
    <button
      type={type}
      className={`button button--${variant} ${className}`.trim()}
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
  return <span className={`status-badge status-badge--${tone}`.trim()}>{children}</span>;
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
    <label className="field-button">
      <select
        aria-label={t("accessibility.languageSelector")}
        className="field-select"
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
      <ChevronDown size={16} strokeWidth={2} />
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
    <label className="field-button field-button--compact">
      <select
        aria-label={t("accessibility.uiLanguageSelector")}
        className="field-select"
        value={value}
        onChange={(event) => onChange(event.target.value as AppLocale)}
      >
        {appLocaleOptions.map((option) => (
          <option key={option.code} value={option.code}>
            {option.label}
          </option>
        ))}
      </select>
      <ChevronDown size={16} strokeWidth={2} />
    </label>
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
    <div className="screen-card__topbar">
      <div className="compact-tabs" role="tablist" aria-label={t("accessibility.translationMode")}>
        <button
          type="button"
          className={`compact-tabs__item ${activeMode === "text" ? "compact-tabs__item--active" : ""}`.trim()}
          onClick={onSwitchText}
        >
          {t("tabs.text")}
        </button>
        <button
          type="button"
          className={`compact-tabs__item ${activeMode === "image" ? "compact-tabs__item--active" : ""}`.trim()}
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
    <div className="language-row">
      <SelectField value={sourceValue} options={options} onChange={onSourceChange} allowAuto={sourceAllowsAuto} />
      <button type="button" className="swap-button" aria-label={t("accessibility.swapLanguages")} onClick={onSwap}>
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
    <div className="actions-row">
      <div className="actions-row__side actions-row__side--start">{leftAction}</div>
      <div className="actions-row__center">{centerAction}</div>
      <div className="actions-row__side actions-row__side--end">{rightAction}</div>
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
    <div className="scroll-view" role="region" aria-label={ariaLabel}>
      <div className="scroll-view__viewport">{children}</div>
    </div>
  );
}

function useAutoGrowTextarea(ref: RefObject<HTMLTextAreaElement | null>, value: string) {
  useLayoutEffect(() => {
    const node = ref.current;
    if (!node) {
      return;
    }

    const resize = () => {
      const manualHeight = Number(node.dataset.manualHeight || "0");
      node.style.height = "0px";
      const parent = node.parentElement;
      const parentHeight = parent ? parent.clientHeight : 0;
      node.style.height = `${Math.max(node.scrollHeight, manualHeight, parentHeight)}px`;
    };

    resize();
    window.addEventListener("resize", resize);
    return () => {
      window.removeEventListener("resize", resize);
    };
  }, [ref, value]);
}

function rememberManualTextareaHeight(event: ReactPointerEvent<HTMLTextAreaElement>) {
  event.currentTarget.dataset.manualHeight = String(event.currentTarget.offsetHeight);
}

function AutoGrowTextarea({
  value,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement> & {
  value: string;
}) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  useAutoGrowTextarea(textareaRef, value);
  return <textarea ref={textareaRef} value={value} {...props} />;
}

function InstructionField({
  ariaLabel,
  placeholder,
  value,
  onChange,
}: {
  ariaLabel: string;
  placeholder: string;
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="instruction-card">
      <div className="instruction-card__copy">
        <span className="instruction-card__label">{t("instruction.label")}</span>
        <span className="instruction-card__hint">{t("instruction.hint")}</span>
      </div>
      <AutoGrowTextarea
        aria-label={ariaLabel}
        className="instruction-textarea"
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onPointerUp={rememberManualTextareaHeight}
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
    <section className="screen-card">
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
              <>
                <LoaderCircle size={16} className="spin" />
                <span>{t("buttons.loadingModel")}</span>
              </>
            ) : busy ? (
              <>
                <LoaderCircle size={16} className="spin" />
                <span>{t("buttons.translating")}</span>
              </>
            ) : (
              t("buttons.translate")
            )}
          </Button>
        }
        rightAction={<Button onClick={onCopyOutput}>{t("buttons.copy")}</Button>}
      />

      <div className="editor-grid">
        <div className="editor-panel">
          <AutoGrowTextarea
            aria-label={t("textScreen.sourceTextAria")}
            className="editor-textarea"
            placeholder={t("textScreen.sourceTextPlaceholder")}
            value={textInput}
            onChange={(event) => onTextInputChange(event.target.value)}
            onPointerUp={rememberManualTextareaHeight}
          />
        </div>
        <div className={`editor-panel editor-panel--output ${textOutput ? "" : "editor-panel--placeholder"}`.trim()}>
          <p>{textOutput || t("textScreen.resultPlaceholder")}</p>
        </div>
      </div>

      <InstructionField
        ariaLabel={t("textScreen.instructionAria")}
        placeholder={t("instruction.textPlaceholder")}
        value={instruction}
        onChange={onInstructionChange}
      />
    </section>
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
    <section className="screen-card screen-card--image">
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
          <Button variant="primary" onClick={onTranslate} disabled={busy || modelLoading || !visionEnabled}>
            {modelLoading ? (
              <>
                <LoaderCircle size={16} className="spin" />
                <span>{t("buttons.loadingModel")}</span>
              </>
            ) : busy ? (
              <>
                <LoaderCircle size={16} className="spin" />
                <span>{t("buttons.translating")}</span>
              </>
            ) : (
              t("buttons.translate")
            )}
          </Button>
        }
        rightAction={<Button onClick={onCopy}>{t("buttons.copy")}</Button>}
      />

      <div className="editor-grid">
        <div
          className={`upload-panel ${visionEnabled ? "upload-panel--dropzone" : ""} ${uploadedPreviewUrl ? "upload-panel--filled" : "upload-panel--empty"} ${dragActive ? "upload-panel--drag-active" : ""}`.trim()}
          role="region"
          aria-label={t("imageScreen.uploadAreaAria")}
          onDragEnter={visionEnabled ? onDragEnter : undefined}
          onDragOver={visionEnabled ? onDragOver : undefined}
          onDragLeave={visionEnabled ? onDragLeave : undefined}
          onDrop={visionEnabled ? onDrop : undefined}
        >
          {!visionEnabled ? (
            <div className="upload-panel__notice">
              <p className="upload-panel__title">{t("imageScreen.requiresVisionTitle")}</p>
              <p className="upload-panel__hint">{t("imageScreen.requiresVisionHint")}</p>
              <Button variant="primary" onClick={onEnableVision} disabled={modelBusy}>
                {modelBusy ? (
                  <>
                    <LoaderCircle size={16} className="spin" />
                    <span>{t("buttons.switching")}</span>
                  </>
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
                  <div className="upload-preview upload-preview--large">
                    <img
                      className="upload-preview__image"
                      src={uploadedPreviewUrl}
                      alt={
                        uploadedFileName
                          ? t("imageScreen.previewAlt", { name: uploadedFileName })
                          : t("imageScreen.selectedPreviewAlt")
                      }
                    />
                  </div>
                  {uploadedFileName ? <p className="upload-panel__filename">{uploadedFileName}</p> : null}
                  <p className="upload-panel__hint">{t("imageScreen.replaceHint")}</p>
                  <div className="upload-panel__actions">
                    <Button variant="primary" onClick={onUploadClick}>{t("buttons.chooseAnother")}</Button>
                    <Button onClick={onUploadReset}>{t("buttons.reset")}</Button>
                  </div>
                </>
              ) : (
                <>
                  <ImageUp size={30} strokeWidth={1.8} />
                  <p className="upload-panel__title">{t("imageScreen.uploadTitle")}</p>
                  <p className="upload-panel__hint">{t("imageScreen.uploadHint", { maxUploadMB })}</p>
                  <Button variant="primary" onClick={onUploadClick}>{t("buttons.uploadImage")}</Button>
                </>
              )}
            </>
          )}
        </div>

        <div className={`editor-panel editor-panel--output ${imageOutput ? "" : "editor-panel--placeholder"}`.trim()}>
          {imageOutput ? <p>{imageOutput}</p> : <p>{t("textScreen.resultPlaceholder")}</p>}
        </div>
      </div>

      <InstructionField
        ariaLabel={t("imageScreen.instructionAria")}
        placeholder={t("instruction.imagePlaceholder")}
        value={instruction}
        onChange={onInstructionChange}
      />
    </section>
  );
}

function ModelDrawer({
  open,
  models,
  onClose,
  onUseNow,
  onDelete,
  onDownload,
  busyModelId,
}: {
  open: boolean;
  models: ModelItem[];
  onClose: () => void;
  onUseNow: (id: string) => void;
  onDelete: (id: string) => void;
  onDownload: (id: string) => void;
  busyModelId: string | null;
}) {
  const { t } = useTranslation();
  if (!open) {
    return null;
  }

  return (
    <div className="overlay overlay--model">
      <button type="button" aria-label={t("model.closeDrawer")} className="overlay__scrim" onClick={onClose} />
      <aside className="drawer drawer--model" role="dialog" aria-modal="true" aria-label={t("model.drawerTitle")}>
        <div className="drawer__header">
          <h2>{t("model.drawerTitle")}</h2>
          <button type="button" className="icon-button" aria-label={t("model.closeList")} onClick={onClose}>
            <X size={18} strokeWidth={2} />
          </button>
        </div>
        <ScrollView ariaLabel={t("model.listAria")}>
          {models.map((model) => {
            const busy = busyModelId === model.id;
            const modelState = describeModelState(model, false);
            const actionVariant = model.active ? "success" : model.selected ? "secondary" : "primary";
            const actionLabel = busy
              ? t("buttons.loadingModel")
              : model.active
                ? t("buttons.active")
                : model.selected
                  ? t("buttons.selected")
                  : t("buttons.useNow");
            return (
              <div key={model.id} className={`model-item ${model.active ? "model-item--active" : ""}`.trim()}>
                <div className="model-item__top">
                  <span className="model-item__name">{model.fileName}</span>
                </div>
                <div className="model-item__badges">
                  <StatusBadge tone={modelState.tone}>{t(modelState.labelKey)}</StatusBadge>
                  <StatusBadge tone={model.visionCapable ? "accent" : "neutral"}>
                    {model.visionCapable ? t("model.capabilityVision") : t("model.capabilityText")}
                  </StatusBadge>
                  {model.recommended ? <StatusBadge>{t("model.recommended")}</StatusBadge> : null}
                </div>
                <span className="model-item__speed">
                  {model.size} · {model.installed ? t("model.sourceInstalled") : t("model.sourceRemote")}
                </span>
                {model.installed ? (
                  <div className="model-item__actions">
                    <Button
                      variant={actionVariant}
                      className="button--full"
                      onClick={() => onUseNow(model.id)}
                      disabled={busy || model.selected}
                    >
                      {busy ? <LoaderCircle size={16} className="spin" /> : null}
                      <span>{actionLabel}</span>
                    </Button>
                    <Button
                      variant="danger"
                      className="button--full"
                      onClick={() => onDelete(model.id)}
                      disabled={busy}
                    >
                      <Trash2 size={16} strokeWidth={2} />
                      <span>{t("buttons.delete")}</span>
                    </Button>
                  </div>
                ) : (
                  <Button
                    variant="primary"
                    className="button--full"
                    onClick={() => onDownload(model.id)}
                    disabled={busy}
                  >
                    {busy ? <LoaderCircle size={16} className="spin" /> : <Download size={16} strokeWidth={2} />}
                    <span>{t("buttons.download")}</span>
                  </Button>
                )}
              </div>
            );
          })}
        </ScrollView>
      </aside>
    </div>
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
    <div className="overlay overlay--history">
      <button type="button" aria-label={t("history.closeDrawer")} className="overlay__scrim" onClick={onClose} />
      <aside className="drawer drawer--history" role="dialog" aria-modal="true" aria-label={t("history.drawerTitle")}>
        <div className="drawer__header drawer__header--history">
          <h2>{t("history.drawerTitle")}</h2>
          <button type="button" className="icon-button" aria-label={t("history.closeList")} onClick={onClose}>
            <X size={20} strokeWidth={2} />
          </button>
        </div>
        <ScrollView ariaLabel={t("history.listAria")}>
          {entries.map((entry) => (
            <button type="button" key={entry.id} className="history-card" onClick={() => onOpen(entry.id)}>
              <div className="history-card__meta">
                <span className="history-card__time">{entry.when}</span>
                <span
                  className="history-card__delete"
                  onClick={(event) => {
                    event.stopPropagation();
                    onDelete(entry.id);
                  }}
                >
                  <Trash2 size={14} strokeWidth={2} />
                  <span>{t("buttons.delete")}</span>
                </span>
              </div>
              <div className="history-card__languages">
                <div>
                  <span className="history-card__label">{t("history.source")}</span>
                  <span className="history-card__value" title={languageLabel(entry.source)}>
                    {languageLabel(entry.source)}
                  </span>
                </div>
                <div>
                  <span className="history-card__label">{t("history.target")}</span>
                  <span className="history-card__value" title={languageLabel(entry.target)}>
                    {languageLabel(entry.target)}
                  </span>
                </div>
              </div>
              <div className="history-card__block">
                <span className="history-card__label">{t("history.sourceText")}</span>
                <p className="history-card__preview" title={entry.input}>
                  {entry.input}
                </p>
              </div>
              <div className="history-card__block">
                <span className="history-card__label">{t("history.targetText")}</span>
                <p className="history-card__preview history-card__target" title={entry.output}>
                  {entry.output}
                </p>
              </div>
            </button>
          ))}
        </ScrollView>
        <div className="drawer__footer drawer__footer--history">
          <Button variant="danger" className="button--full" onClick={onClear} disabled={clearing || entries.length === 0}>
            {clearing ? (
              <>
                <LoaderCircle size={16} className="spin" />
                <span>{t("buttons.clearingHistory")}</span>
              </>
            ) : (
              t("buttons.clearHistory")
            )}
          </Button>
        </div>
      </aside>
    </div>
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
    <div className="overlay overlay--modal">
      <div className="overlay__scrim overlay__scrim--solid" onClick={onClose} />
      <div className="history-modal" role="dialog" aria-modal="true" aria-label={t("history.detailTitle")}>
        <div className="history-modal__top">
          <div className="history-modal__time">
            <span>{t("history.translationTime")}</span>
            <strong>{entry.when}</strong>
          </div>
          <button type="button" className="icon-button icon-button--outline" aria-label={t("history.closeDetail")} onClick={onClose}>
            <X size={16} strokeWidth={2} />
          </button>
        </div>

        <div className="history-modal__pair">
          <div className="history-modal__pill">
            <span>{t("history.source")}</span>
            <strong>{languageLabel(entry.source)}</strong>
          </div>
          <div className="history-modal__pill">
            <span>{t("history.target")}</span>
            <strong>{languageLabel(entry.target)}</strong>
          </div>
        </div>

        <div className="history-modal__section">
          <span>{t("history.sourceText")}</span>
          <div className="history-modal__box">
            <p>{entry.input}</p>
          </div>
        </div>

        <div className="history-modal__section">
          <span>{t("history.targetText")}</span>
          <div className="history-modal__box">
            <p>{entry.output}</p>
          </div>
        </div>

        <div className="history-modal__actions">
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
) {
  const response = await fetch(url, {
    method: "POST",
    body,
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
  const [busyText, setBusyText] = useState(false);
  const [busyImage, setBusyImage] = useState(false);
  const [imageDragActive, setImageDragActive] = useState(false);
  const [translationModelLoading, setTranslationModelLoading] = useState(false);
  const [clearingHistory, setClearingHistory] = useState(false);

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

  useEffect(() => {
    document.title = t("app.title");
  }, [i18n.resolvedLanguage, t]);

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
    setBusyModelId(modelId || "__global__");
    setPendingModelAction(url);
    try {
      await streamJsonLines(
        url,
        modelId ? new URLSearchParams({ model_id: modelId }) : undefined,
        (event) => {
          if (event.type === "error") {
            throw new Error(event.message || t("errors.modelActionFailed"));
          }
        },
      );
      await refreshState(true);
      if (url === "/api/models/enable-vision") {
        setScreenMode("image");
      }
    } catch (error) {
      applyClientError(error instanceof Error ? error.message : "model action failed");
    } finally {
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

  if (loading) {
    return (
      <div className="app-shell">
        <div className="loading-card">
          <LoaderCircle size={18} className="spin" />
          <span>{t("app.loading")}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="app-shell">
      <header className="page-header">
        <button
          type="button"
          className="brand-title"
          onClick={() => setScreenMode("text")}
          aria-label={t("header.goToTextScreen")}
        >
          TranslateGemmaUI
        </button>

        <div className="page-header__actions">
          <AppLocaleField value={currentLocale} onChange={(nextLocale) => void setAppLocale(nextLocale)} />
          <Button onClick={() => setDrawer("model")}>{t("header.model")}</Button>
        </div>
      </header>

      {showStatusBanner ? <div className="status-banner">{statusMessage}</div> : null}

      <main className="page-main">
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
        busyModelId={busyModelId}
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

      {needsModelSetup ? (
        <div className="setup-banner">{t("setup.banner")}</div>
      ) : null}
    </div>
  );
}

export default App;
