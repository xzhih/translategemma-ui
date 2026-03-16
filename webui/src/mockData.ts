export type ScreenMode = "text" | "image";

export type ModelActionState = "active" | "installed" | "remote";

export interface ModelItem {
  id: string;
  name: string;
  speed: string;
  state: ModelActionState;
}

export interface HistoryEntry {
  id: string;
  time: string;
  sourceLanguageLabel: string;
  targetLanguageLabel: string;
  sourceText: string;
  targetText: string;
}

export const defaultTextInput =
  "Please translate the following product update into Simplified Chinese. The local runtime is now ready, model switching is faster, image translation has been improved, and translation history can be reviewed later without interrupting the current task. Keep the tone clear, direct, and suitable for a desktop tool used every day by developers and bilingual users.";

export const defaultTextOutput =
  "请将以下产品更新翻译为简体中文。本地运行时现已就绪，模型切换更快，图片翻译能力也已提升，并且用户可以在不中断当前任务的情况下回看翻译历史。整体语气需要清晰、直接，适合开发者和双语用户每天使用的桌面工具。";

export const imageUploadHintTitle = "拖拽图片到这里，或点击上传";
export const imageUploadHintText = "支持 PNG / JPG / WEBP，最大 10MB";

export const modelsSeed: ModelItem[] = [
  {
    id: "gemma-3-27b-it-q4",
    name: "gemma-3-27b-it-q4",
    speed: "",
    state: "active",
  },
  {
    id: "gemma-3-12b-it-q4",
    name: "gemma-3-12b-it-q4",
    speed: "Fast",
    state: "installed",
  },
  {
    id: "llama-3.1-8b-instruct-q4",
    name: "llama-3.1-8b-instruct-q4",
    speed: "Slow",
    state: "remote",
  },
];

export const historySeed: HistoryEntry[] = [
  {
    id: "entry-1",
    time: "2026-03-06 10:24",
    sourceLanguageLabel: "English",
    targetLanguageLabel: "中文",
    sourceText: "Please upload your passport for verification.",
    targetText: "请上传您的护照以供验证。",
  },
  {
    id: "entry-2",
    time: "2026-03-05 18:11",
    sourceLanguageLabel: "日本語",
    targetLanguageLabel: "English",
    sourceText: "画像内のテキストを翻訳してください。",
    targetText: "Please translate the text in the image.",
  },
  {
    id: "entry-3",
    time: "2026-03-04 09:02",
    sourceLanguageLabel: "한국어",
    targetLanguageLabel: "Deutsch",
    sourceText: "모델 다운로드를 시작합니다.",
    targetText: "Der Model-Download wird gestartet.",
  },
];

export const historyDetailSeed = {
  time: "2026-03-06 14:28:11",
  sourceLanguageLabel: "中文",
  targetLanguageLabel: "English",
  sourceText:
    "请将以下内容翻译为英文：我们计划在下周上线新的语音翻译功能，重点优化延迟与术语一致性。",
  targetText:
    "Please translate the following into English: We plan to launch a new voice translation feature next week, with a focus on latency optimization and terminology consistency.",
};
