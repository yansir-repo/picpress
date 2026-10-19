import { AppService } from "/bindings/picpress/index.js";
import { Events } from "/wails/runtime.js";

const STORAGE_KEY = "picpress.settings.v1";

const DEFAULT_OPTIONS = {
  format: "auto",
  quality: 82,
  resizeMode: "none",
  maxLongEdge: 2560,
  percent: 100,
};

const FORMAT_HINTS = {
  auto: "自动从 JPG / WebP / PNG 中选出体积最小的结果",
  jpeg: "兼容性最好，适合照片；透明区域会填充背景色",
  png: "无损格式，适合截图、图标和需要透明背景的图片",
  webp: "现代格式，通常比同质量 JPG 小 25% 左右",
  avif: "压缩率最高，编码稍慢，较老的软件可能打不开",
};

const state = {
  items: [],
  options: { ...DEFAULT_OPTIONS },
  compressing: false,
  progress: null,
  runSeq: 0,
  picking: false,
  saving: false,
  queued: false,
  compareId: null,
};

const els = {};

document.addEventListener("DOMContentLoaded", init);

function init() {
  cacheElements();
  loadSettings();
  bindUi();
  bindRuntimeEvents();
  applyOptionsToUi();
  renderAll();
  updateContentMeta();
  renderSummary();
}

function cacheElements() {
  const ids = [
    "content",
    "content-sub",
    "content-actions",
    "dropzone",
    "pick-btn",
    "add-more-btn",
    "add-strip",
    "clear-btn",
    "list-wrap",
    "list",
    "format-seg",
    "format-hint",
    "quality-field",
    "quality-range",
    "quality-number",
    "quality-tag",
    "quality-hint",
    "resize-seg",
    "resize-longest",
    "resize-percent",
    "longest-input",
    "longest-chips",
    "percent-range",
    "percent-value",
    "reset-btn",
    "summary-metric",
    "summary-detail",
    "summary-badge",
    "summary-bar-fill",
    "summary-rows",
    "sum-original",
    "sum-output",
    "save-all-btn",
    "compare-modal",
    "compare-stage",
    "compare-before",
    "compare-after",
    "compare-before-img",
    "compare-range",
    "compare-sub",
    "compare-original",
    "compare-output",
    "compare-saved",
    "compare-save",
    "toasts",
    "item-template",
  ];
  for (const id of ids) {
    els[toCamel(id)] = document.getElementById(id);
  }
}

function toCamel(id) {
  return id.replace(/-([a-z])/g, (_, c) => c.toUpperCase());
}

/* ---------- UI wiring ---------- */

function bindUi() {
  const openPicker = (event) => {
    if (event) event.stopPropagation();
    pickFiles();
  };
  els.pickBtn.addEventListener("click", openPicker);
  els.addStrip.addEventListener("click", openPicker);
  els.addMoreBtn.addEventListener("click", openPicker);

  els.dropzone.addEventListener("click", (event) => {
    if (event.target.closest("button")) return;
    pickFiles();
  });
  els.clearBtn.addEventListener("click", clearAll);
  els.resetBtn.addEventListener("click", resetSettings);

  els.formatSeg.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-format]");
    if (!button) return;
    const format = button.dataset.format;
    if (format === state.options.format) return;
    state.options.format = format;
    persistSettings();
    applyOptionsToUi();
    scheduleCompress(160);
  });

  const quality = els.qualityRange;
  quality.addEventListener("input", () => {
    state.options.quality = clamp(Number(quality.value) || 82, 10, 100);
    updateQualityUi();
  });
  quality.addEventListener("change", () => {
    persistSettings();
    scheduleCompress();
  });

  els.resizeSeg.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-resize]");
    if (!button) return;
    const mode = button.dataset.resize;
    if (mode === state.options.resizeMode) return;
    state.options.resizeMode = mode;
    persistSettings();
    applyOptionsToUi();
    scheduleCompress(160);
  });

  els.longestInput.addEventListener("input", () => {
    const value = Number(els.longestInput.value);
    if (Number.isFinite(value) && value > 0) {
      state.options.maxLongEdge = clamp(Math.round(value), 32, 20000);
      updateLongestChips();
    }
  });
  els.longestInput.addEventListener("change", () => {
    normalizeLongestInput();
    persistSettings();
    scheduleCompress();
  });
  els.longestChips.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-edge]");
    if (!button) return;
    state.options.maxLongEdge = Number(button.dataset.edge);
    els.longestInput.value = state.options.maxLongEdge;
    updateLongestChips();
    persistSettings();
    scheduleCompress(120);
  });

  els.percentRange.addEventListener("input", () => {
    state.options.percent = clamp(Number(els.percentRange.value) || 100, 10, 100);
    updatePercentUi();
  });
  els.percentRange.addEventListener("change", () => {
    persistSettings();
    scheduleCompress();
  });

  els.saveAllBtn.addEventListener("click", saveAll);
  els.list.addEventListener("click", onListClick);

  document.addEventListener("click", (event) => {
    if (event.target.closest("[data-close-compare]")) closeCompare();
  });
  els.compareRange.addEventListener("input", () => {
    setComparePosition(Number(els.compareRange.value));
  });
  els.compareSave.addEventListener("click", () => {
    if (state.compareId) saveOne(state.compareId);
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !els.compareModal.hidden) closeCompare();
  });

  const dragTargets = ["dragenter", "dragover", "dragleave", "drop"];
  for (const type of dragTargets) {
    window.addEventListener(type, onWindowDrag, false);
  }
}

function bindRuntimeEvents() {
  Events.On("files-dropped", (event) => {
    const files = event?.data;
    if (Array.isArray(files) && files.length > 0) addPaths(files);
  });
  Events.On("compress:progress", (event) => {
    state.progress = event?.data || null;
    updateProgressUi();
  });
  Events.On("menu:open", () => pickFiles());
  Events.On("menu:save-all", () => saveAll());
  Events.On("menu:clear", () => clearAll());
}

function onWindowDrag(event) {
  if (state.items.length > 0) return;
  const hasFiles =
    event.dataTransfer &&
    Array.from(event.dataTransfer.types || []).includes("Files");
  if (!hasFiles) return;

  if (event.type === "dragenter" || event.type === "dragover") {
    event.preventDefault();
    els.dropzone.classList.add("is-dragging");
  } else {
    els.dropzone.classList.remove("is-dragging");
  }
}

/* ---------- Actions ---------- */

async function pickFiles() {
  if (state.picking) return;
  state.picking = true;
  try {
    const result = await AppService.PickFiles();
    ingest(result);
  } catch (error) {
    toast(`打开文件选择器失败：${errorText(error)}`, "error");
  } finally {
    state.picking = false;
  }
}

async function addPaths(paths) {
  if (!Array.isArray(paths) || paths.length === 0) return;
  try {
    const result = await AppService.AddFiles(paths);
    ingest(result);
  } catch (error) {
    toast(`添加图片失败：${errorText(error)}`, "error");
  }
}

function ingest(result) {
  if (!result) return;
  const items = Array.isArray(result.items) ? result.items : [];
  let added = 0;

  for (const info of items) {
    if (!info || !info.id) continue;
    const existing = state.items.find((item) => item.info.id === info.id);
    if (existing) {
      existing.info = info;
      continue;
    }
    state.items.push({
      info,
      result: null,
      updating: false,
      error: "",
      savedPath: "",
      isNew: true,
    });
    added += 1;
  }

  const errors = Array.isArray(result.errors) ? result.errors : [];
  if (errors.length > 0) {
    const first = errors[0];
    const name = first.path ? baseName(first.path) : "文件";
    const extra = errors.length > 1 ? `，另有 ${errors.length - 1} 个失败` : "";
    toast(`${name}：${first.message}${extra}`, "error");
  }

  renderAll();
  updateContentMeta();
  renderSummary();

  if (added > 0 || items.length > 0) {
    scheduleCompress(0);
  }
}

let compressTimer = null;

function scheduleCompress(delay = 320) {
  if (state.items.length === 0) return;
  clearTimeout(compressTimer);
  compressTimer = setTimeout(() => {
    if (state.compressing) {
      state.queued = true;
      return;
    }
    compressAll();
  }, delay);
}

async function compressAll() {
  if (state.items.length === 0) return;
  if (state.compressing) {
    state.queued = true;
    return;
  }

  const seq = ++state.runSeq;
  const runItems = state.items.slice();
  state.compressing = true;
  state.progress = null;
  for (const item of runItems) {
    item.updating = true;
    item.error = "";
  }

  markBusyUi();
  updateProgressUi();
  renderSummary();

  try {
    const results = await AppService.Compress([], currentOptions());
    if (seq !== state.runSeq) return;

    const byId = new Map();
    for (const result of results || []) {
      if (result && result.id) byId.set(result.id, result);
    }

    for (const item of runItems) {
      item.updating = false;
      const result = byId.get(item.info.id);
      if (!result) {
        item.error = "没有得到压缩结果";
        continue;
      }
      if (result.error) {
        item.error = result.error;
        item.result = null;
        continue;
      }
      item.result = result;
      item.error = "";
      item.savedPath = "";
    }
  } catch (error) {
    if (seq === state.runSeq) {
      for (const item of runItems) item.updating = false;
      toast(`压缩失败：${errorText(error)}`, "error");
    }
  } finally {
    if (seq === state.runSeq) {
      state.compressing = false;
      state.progress = null;
      renderAll();
      updateContentMeta();
      renderSummary();
      if (state.queued) {
        state.queued = false;
        scheduleCompress(40);
      }
    }
  }
}

async function onListClick(event) {
  const button = event.target.closest("button[data-action]");
  if (!button) return;
  const card = button.closest(".item");
  const id = card?.dataset.id;
  if (!id) return;

  const action = button.dataset.action;
  if (action === "compare") openCompare(id);
  if (action === "save") saveOne(id);
  if (action === "remove") removeItem(id);
}

async function removeItem(id) {
  const item = state.items.find((entry) => entry.info.id === id);
  if (!item) return;
  try {
    await AppService.Remove(id);
  } catch (error) {
    toast(`移除失败：${errorText(error)}`, "error");
    return;
  }
  state.items = state.items.filter((entry) => entry.info.id !== id);
  renderAll();
  updateContentMeta();
  renderSummary();
  toast(`已移除 ${item.info.name}`);
}

async function clearAll() {
  if (state.items.length === 0) return;
  try {
    await AppService.Clear();
  } catch (error) {
    toast(`清空失败：${errorText(error)}`, "error");
    return;
  }
  state.items = [];
  state.progress = null;
  state.compressing = false;
  state.queued = false;
  ++state.runSeq;
  renderAll();
  updateContentMeta();
  renderSummary();
  toast("列表已清空");
}

async function saveOne(id) {
  const item = state.items.find((entry) => entry.info.id === id);
  if (!item || !item.result || item.updating) return;
  try {
    const path = await AppService.SaveFile(id);
    if (!path) return;
    item.savedPath = path;
    toast(`已保存 ${baseName(path)}`, "success", {
      label: "在 Finder 中显示",
      onClick: () => AppService.RevealPath(path).catch(() => {}),
    });
    renderAll();
  } catch (error) {
    toast(`保存失败：${errorText(error)}`, "error");
  }
}

async function saveAll() {
  if (state.saving || !canSaveAll()) return;
  state.saving = true;
  updateSaveButton();
  try {
    const result = await AppService.SaveAll([]);
    if (!result || !result.directory) return;
    const saved = Array.isArray(result.saved) ? result.saved : [];
    if (saved.length === 0) {
      toast("没有可保存的图片", "warn");
      return;
    }
    let savedIndex = 0;
    for (const item of state.items) {
      if (item.result) {
        item.savedPath = saved[savedIndex] || result.directory;
        savedIndex += 1;
      }
    }
    const message =
      saved.length === state.items.length
        ? `${saved.length} 张图片已保存`
        : `${saved.length} 张图片已保存，${state.items.length - saved.length} 张失败`;
    toast(message, saved.length === state.items.length ? "success" : "warn", {
      label: "打开文件夹",
      onClick: () => AppService.RevealPath(result.directory).catch(() => {}),
    });
    if (Array.isArray(result.errors) && result.errors.length > 0) {
      toast(result.errors[0], "error");
    }
  } catch (error) {
    toast(`保存失败：${errorText(error)}`, "error");
  } finally {
    state.saving = false;
    updateSaveButton();
  }
}

function canSaveAll() {
  return (
    !state.compressing &&
    state.items.length > 0 &&
    state.items.some((item) => item.result && !item.error)
  );
}

function resetSettings() {
  state.options = { ...DEFAULT_OPTIONS };
  persistSettings();
  applyOptionsToUi();
  scheduleCompress(80);
  toast("已恢复默认设置");
}

/* ---------- Rendering ---------- */

function renderAll() {
  renderList();
  updateSaveButton();
}

function renderList() {
  const hasItems = state.items.length > 0;
  els.dropzone.hidden = hasItems;
  els.listWrap.hidden = !hasItems;
  els.contentActions.hidden = !hasItems;

  if (!hasItems) {
    els.list.replaceChildren();
    return;
  }

  const fragment = document.createDocumentFragment();
  for (const item of state.items) {
    fragment.appendChild(renderItem(item));
  }
  els.list.replaceChildren(fragment);
}

function renderItem(item) {
  const node = els.itemTemplate.content.firstElementChild.cloneNode(true);
  const { info, result } = item;
  node.dataset.id = info.id;
  node.classList.toggle("is-updating", item.updating);
  node.classList.toggle("has-error", Boolean(item.error));
  if (item.isNew) {
    node.classList.add("is-new");
    delete item.isNew;
  }

  const thumb = node.querySelector(".item-thumb img");
  thumb.src = info.preview || "";
  thumb.alt = info.name || "";

  node.querySelector(".item-name").textContent = info.name || "未命名图片";
  node.querySelector(".format-chip").textContent = sourceFormatLabel(info.format);

  const meta = `${formatDimensions(info.width, info.height)} · ${formatBytes(info.bytes)}`;
  node.querySelector(".item-meta").textContent = meta;

  const status = node.querySelector(".item-status");
  if (item.error) {
    status.hidden = false;
    status.classList.add("is-error");
    status.textContent = item.error;
  } else if (item.updating) {
    status.hidden = false;
    status.innerHTML = '<span class="spinner"></span>正在压缩…';
  } else if (info.warning) {
    status.hidden = false;
    status.textContent = info.warning;
  } else if (item.savedPath) {
    status.hidden = false;
    status.textContent = `已保存 · ${baseName(item.savedPath)}`;
  } else {
    status.hidden = true;
    status.textContent = "";
  }

  const resultSize = node.querySelector(".result-size");
  const resultFormat = node.querySelector(".result-format");
  const resultDetail = node.querySelector(".result-detail");
  const savings = node.querySelector(".item-savings");
  const compareBtn = node.querySelector('[data-action="compare"]');
  const saveBtn = node.querySelector('[data-action="save"]');

  if (result && !item.error) {
    resultSize.textContent = formatBytes(result.bytes);
    resultFormat.textContent = outputFormatLabel(result.format);
    resultDetail.textContent = formatDimensions(result.width, result.height);
    const delta = savingsLabel(result.savedPercent);
    savings.hidden = false;
    savings.innerHTML = "";
    const pill = document.createElement("span");
    pill.className = "savings-pill";
    if (result.savedPercent < 0) pill.classList.add("is-bigger");
    pill.textContent = delta;
    pill.title = result.warning || "相比原图的大小变化";
    savings.appendChild(pill);
    compareBtn.disabled = false;
    saveBtn.disabled = false;
  } else {
    resultSize.textContent = item.updating ? "—" : "未压缩";
    resultFormat.textContent = "";
    resultDetail.textContent = item.updating ? "" : "调整设置后将自动重试";
    savings.hidden = true;
    compareBtn.disabled = true;
    saveBtn.disabled = true;
  }

  if (result?.warning) {
    node.querySelector(".item-meta").title = result.warning;
  }
  return node;
}

function updateProgressUi() {
  updateContentMeta();
  updateSaveButton();
}

function markBusyUi() {
  for (const card of els.list.querySelectorAll(".item")) {
    const item = state.items.find((entry) => entry.info.id === card.dataset.id);
    if (!item) continue;
    card.classList.toggle("is-updating", item.updating);
    card.classList.toggle("has-error", Boolean(item.error));

    const status = card.querySelector(".item-status");
    if (item.updating) {
      status.hidden = false;
      status.classList.remove("is-error");
      status.innerHTML = '<span class="spinner"></span>正在压缩…';
    } else if (!item.error) {
      status.hidden = true;
      status.textContent = "";
    }

    const compareBtn = card.querySelector('[data-action="compare"]');
    const saveBtn = card.querySelector('[data-action="save"]');
    const available = Boolean(item.result) && !item.updating && !item.error;
    compareBtn.disabled = !available;
    saveBtn.disabled = !available;
  }
}

function updateContentMeta() {
  if (state.items.length === 0) {
    els.contentSub.textContent = "拖入图片，立即自动压缩";
    return;
  }

  if (state.compressing) {
    const progress = state.progress;
    if (progress && progress.total > 0) {
      els.contentSub.textContent = `正在压缩 ${progress.index} / ${progress.total}…`;
    } else {
      els.contentSub.textContent = "正在压缩…";
    }
    return;
  }

  const total = state.items.reduce((sum, item) => sum + (item.info.bytes || 0), 0);
  els.contentSub.textContent = `${state.items.length} 张图片 · 共 ${formatBytes(total)}`;
}

function renderSummary() {
  const results = state.items.filter((item) => item.result && !item.error);
  const metric = els.summaryMetric;
  const detail = els.summaryDetail;
  const badge = els.summaryBadge;
  const rows = els.summaryRows;
  const bar = els.summaryBarFill;

  if (state.items.length === 0) {
    metric.textContent = "—";
    detail.textContent = "添加图片后显示压缩效果";
    badge.hidden = true;
    rows.hidden = true;
    bar.style.width = "0%";
    bar.classList.remove("is-bigger");
    return;
  }

  if (results.length === 0) {
    metric.textContent = state.compressing ? "压缩中…" : "—";
    detail.textContent = state.compressing ? "正在计算压缩效果" : "等待压缩结果";
    badge.hidden = true;
    rows.hidden = true;
    bar.style.width = "0%";
    bar.classList.remove("is-bigger");
    return;
  }

  const original = results.reduce((sum, item) => sum + (item.info.bytes || 0), 0);
  const output = results.reduce((sum, item) => sum + (item.result.bytes || 0), 0);
  const saved = original - output;
  const percent = original > 0 ? (saved / original) * 100 : 0;
  const bigger = saved < 0;

  const [value, unit] = bytesParts(Math.abs(saved));
  metric.innerHTML = "";
  const sign = document.createElement("span");
  sign.textContent = bigger ? "+" : "−";
  const number = document.createElement("span");
  number.textContent = value;
  const unitEl = document.createElement("span");
  unitEl.className = "metric-unit";
  unitEl.textContent = unit;
  metric.append(sign, number, unitEl);

  detail.textContent =
    results.length === state.items.length
      ? `${formatBytes(original)} → ${formatBytes(output)}`
      : `${formatBytes(original)} → ${formatBytes(output)} · ${results.length}/${state.items.length} 张`;

  badge.hidden = false;
  badge.textContent = savingsLabel(percent);
  badge.classList.toggle("is-bigger", bigger);

  rows.hidden = false;
  els.sumOriginal.textContent = formatBytes(original);
  els.sumOutput.textContent = formatBytes(output);

  const width = clamp(Math.abs(percent), 0, 100);
  bar.style.width = `${width}%`;
  bar.classList.toggle("is-bigger", bigger);
}

function updateSaveButton() {
  const enabled = canSaveAll() && !state.saving;
  els.saveAllBtn.disabled = !enabled;
  els.saveAllBtn.innerHTML = "";
  const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  icon.setAttribute("viewBox", "0 0 16 16");
  icon.setAttribute("fill", "none");
  icon.innerHTML =
    '<path d="M8 2.8v7.4m0 0 3-3m-3 3-3-3M3 12.5h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  const label = document.createElement("span");
  if (state.saving) {
    label.textContent = "正在保存…";
  } else if (state.compressing) {
    label.textContent = "压缩中…";
  } else {
    label.textContent = "保存全部";
  }
  els.saveAllBtn.append(icon, label);
}

/* ---------- Settings ---------- */

function applyOptionsToUi() {
  for (const button of els.formatSeg.querySelectorAll("button[data-format]")) {
    button.classList.toggle("is-active", button.dataset.format === state.options.format);
  }
  els.formatHint.textContent = FORMAT_HINTS[state.options.format] || "";

  for (const button of els.resizeSeg.querySelectorAll("button[data-resize]")) {
    button.classList.toggle("is-active", button.dataset.resize === state.options.resizeMode);
  }

  els.resizeLongest.hidden = state.options.resizeMode !== "longest";
  els.resizePercent.hidden = state.options.resizeMode !== "percent";

  els.longestInput.value = state.options.maxLongEdge;
  updateLongestChips();
  updatePercentUi();
  updateQualityUi();
}

function updateQualityUi() {
  const quality = state.options.quality;
  const isPng = state.options.format === "png";
  els.qualityRange.value = quality;
  els.qualityNumber.textContent = quality;
  setRangeFill(els.qualityRange, quality, 10, 100);

  els.qualityRange.disabled = isPng;
  els.qualityField.classList.toggle("is-disabled", isPng);
  els.qualityHint.hidden = !isPng;

  let label = "推荐";
  let kind = "";
  if (isPng) {
    label = "无损";
  } else if (quality >= 90) {
    label = "近乎无损";
    kind = "is-max";
  } else if (quality >= 75) {
    label = "推荐";
  } else if (quality >= 50) {
    label = "更小";
    kind = "is-strong";
  } else {
    label = "极限";
    kind = "is-strong";
  }
  els.qualityTag.textContent = label;
  els.qualityTag.className = `quality-tag ${kind}`.trim();
}

function updateLongestChips() {
  for (const button of els.longestChips.querySelectorAll("button[data-edge]")) {
    button.classList.toggle(
      "is-active",
      Number(button.dataset.edge) === Number(state.options.maxLongEdge)
    );
  }
}

function updatePercentUi() {
  const percent = state.options.percent;
  els.percentRange.value = percent;
  els.percentValue.textContent = `${percent}%`;
  setRangeFill(els.percentRange, percent, 10, 100);
}

function normalizeLongestInput() {
  let value = Number(els.longestInput.value);
  if (!Number.isFinite(value)) value = DEFAULT_OPTIONS.maxLongEdge;
  value = clamp(Math.round(value), 32, 20000);
  state.options.maxLongEdge = value;
  els.longestInput.value = value;
  updateLongestChips();
}

function setRangeFill(input, value, min, max) {
  const ratio = clamp((value - min) / (max - min), 0, 1);
  input.style.setProperty("--pct", `${ratio * 100}%`);
}

function loadSettings() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return;
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object") return;
    if (["auto", "jpeg", "png", "webp", "avif"].includes(parsed.format)) {
      state.options.format = parsed.format;
    }
    if (Number.isFinite(parsed.quality)) {
      state.options.quality = clamp(Math.round(parsed.quality), 10, 100);
    }
    if (["none", "longest", "percent"].includes(parsed.resizeMode)) {
      state.options.resizeMode = parsed.resizeMode;
    }
    if (Number.isFinite(parsed.maxLongEdge)) {
      state.options.maxLongEdge = clamp(Math.round(parsed.maxLongEdge), 32, 20000);
    }
    if (Number.isFinite(parsed.percent)) {
      state.options.percent = clamp(Math.round(parsed.percent), 10, 100);
    }
  } catch {
    /* ignore malformed settings */
  }
}

function persistSettings() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state.options));
  } catch {
    /* storage can be unavailable; settings just won't persist */
  }
}

function currentOptions() {
  normalizeLongestInput();
  return {
    format: state.options.format,
    quality: state.options.quality,
    resizeMode: state.options.resizeMode,
    maxLongEdge: state.options.maxLongEdge,
    percent: state.options.percent,
    background: "#ffffff",
  };
}

/* ---------- Compare ---------- */

function openCompare(id) {
  const item = state.items.find((entry) => entry.info.id === id);
  if (!item || !item.result) return;
  state.compareId = id;

  els.compareBeforeImg.src = item.info.preview || "";
  els.compareAfter.src = item.result.preview || item.info.preview || "";
  els.compareSub.textContent = `${item.info.name} · ${sourceFormatLabel(item.info.format)} → ${outputFormatLabel(item.result.format)}`;
  els.compareOriginal.textContent = `${formatDimensions(item.info.width, item.info.height)} · ${formatBytes(item.info.bytes)}`;
  els.compareOutput.textContent = `${formatDimensions(item.result.width, item.result.height)} · ${formatBytes(item.result.bytes)}`;
  els.compareSaved.textContent = `${savingsLabel(item.result.savedPercent)} · ${formatBytes(Math.abs(item.info.bytes - item.result.bytes))}`;
  els.compareSaved.style.color = item.result.savedPercent < 0 ? "var(--amber)" : "";

  els.compareRange.value = 50;
  setComparePosition(50);
  els.compareModal.hidden = false;
  document.body.classList.add("modal-open");

  requestAnimationFrame(() => {
    const ratio = item.info.width / Math.max(1, item.info.height);
    const width = els.compareStage.clientWidth || 800;
    const height = clamp(Math.round(width / Math.max(ratio, 0.2)), 220, Math.round(window.innerHeight * 0.56));
    els.compareStage.style.height = `${height}px`;
  });

  els.compareModal.querySelector(".icon-btn--large")?.focus();
}

function closeCompare() {
  els.compareModal.hidden = true;
  state.compareId = null;
  document.body.classList.remove("modal-open");
}

function setComparePosition(value) {
  const clamped = clamp(value, 0, 100);
  els.compareStage.style.setProperty("--pos", `${clamped}%`);
}

/* ---------- Toasts ---------- */

function toast(message, kind = "info", action = null) {
  if (!message) return;
  const existing = els.toasts.querySelectorAll(".toast");
  if (existing.length >= 3) existing[0].remove();

  const node = document.createElement("div");
  node.className = `toast${kind === "error" ? " is-error" : kind === "warn" ? " is-warn" : ""}`;
  const dot = document.createElement("span");
  dot.className = "toast-dot";
  const text = document.createElement("span");
  text.textContent = message;
  node.append(dot, text);

  if (action && typeof action.onClick === "function") {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "toast-action";
    button.textContent = action.label;
    button.addEventListener("click", () => {
      action.onClick();
      dismiss();
    });
    node.appendChild(button);
  }

  els.toasts.appendChild(node);
  let removed = false;
  const dismiss = () => {
    if (removed) return;
    removed = true;
    node.classList.add("is-out");
    setTimeout(() => node.remove(), 220);
  };
  setTimeout(dismiss, action ? 6500 : 4200);
}

/* ---------- Formatting ---------- */

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function bytesParts(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return ["0", "B"];
  if (bytes < 1024) return [String(Math.round(bytes)), "B"];
  if (bytes < 1024 * 1024) {
    const value = bytes / 1024;
    return [trimNumber(value, value < 10 ? 1 : 0), "KB"];
  }
  const value = bytes / (1024 * 1024);
  return [trimNumber(value, value >= 100 ? 0 : value >= 10 ? 1 : 2), "MB"];
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  const [value, unit] = bytesParts(bytes);
  return `${value} ${unit}`;
}

function trimNumber(value, digits) {
  const text = value.toFixed(digits);
  return text.includes(".") ? text.replace(/\.?0+$/, "") : text;
}

function formatDimensions(width, height) {
  if (!width || !height) return "—";
  return `${width} × ${height}`;
}

function savingsLabel(percent) {
  if (!Number.isFinite(percent)) return "—";
  const rounded = Math.abs(percent) >= 10 ? Math.round(Math.abs(percent)) : Math.abs(percent).toFixed(1);
  return percent < 0 ? `+${rounded}%` : `−${rounded}%`;
}

function sourceFormatLabel(format) {
  const value = String(format || "").toLowerCase();
  const map = {
    jpeg: "JPG",
    jpg: "JPG",
    png: "PNG",
    webp: "WEBP",
    avif: "AVIF",
    heic: "HEIC",
    heif: "HEIF",
    gif: "GIF",
  };
  return map[value] || value.toUpperCase() || "IMG";
}

function outputFormatLabel(format) {
  const value = String(format || "").toLowerCase();
  const map = { jpeg: "JPG", png: "PNG", webp: "WEBP", avif: "AVIF" };
  return map[value] || value.toUpperCase();
}

function baseName(path) {
  if (!path) return "";
  const parts = String(path).split(/[\\/]/);
  return parts[parts.length - 1] || path;
}

function errorText(error) {
  if (!error) return "未知错误";
  const message = error.message || String(error);
  return message.replace(/^Error:\s*/, "");
}
