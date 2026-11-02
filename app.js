(function () {
  "use strict";

  const APP = window.APP_CONFIG || {};
  const DATA = window.GLOSSARY_DATA || { entries: [] };
  const ENTRIES = Array.isArray(DATA.entries) ? DATA.entries.slice() : [];
  const STORAGE_KEY = "polyphone-checker:draft:v1";

  const TYPE_LABELS = {
    place: "地名",
    surname: "姓氏",
    word: "词语",
    name: "专名",
    idiom: "成语/专名",
    brand: "品牌"
  };

  const RISK_LABELS = {
    high: "高风险",
    medium: "易错"
  };

  const TONE_MARKS = {
    a: ["ā", "á", "ǎ", "à"],
    o: ["ō", "ó", "ǒ", "ò"],
    e: ["ē", "é", "ě", "è"],
    i: ["ī", "í", "ǐ", "ì"],
    u: ["ū", "ú", "ǔ", "ù"],
    "ü": ["ǖ", "ǘ", "ǚ", "ǜ"]
  };

  const EXAMPLE_TEXT = `欢迎来到重庆。我们公司在银行行业工作，行长今天出差了。
这次项目很重要，需要重新检查，不能重复犯错。
他长大了，喜欢音乐，也觉得这个角色很有挑战。
北京朝阳区和辽宁朝阳市，读音不一样。
六安、蚌埠、亳州、丽水、台州、乐清，这些地名你读对了吗？
曾先生、解老师、查女士、仇总、朴同学，姓氏读音也容易错。
薄荷、薄弱、单薄，这几个“薄”也不一样。`;

  const state = {
    matches: [],
    selected: new Map(),
    confirmed: new Set(),
    ignored: new Set(),
    exportMode: "annotated",
    glossaryQuery: "",
    lastCheckedText: "",
    stale: false
  };

  const els = {};

  function init() {
    cacheElements();
    bindEvents();
    configureLinks();
    restoreDraft();
    updateCharCount();
    renderGlossary();

    const params = new URLSearchParams(window.location.search);
    if (params.get("tab") === "glossary") {
      setTab("glossary");
    }

    if (params.get("demo") === "1") {
      els.input.value = EXAMPLE_TEXT;
      updateCharCount();
      checkText();
      return;
    }

    if (els.input.value.trim()) {
      checkText({ silent: true });
    }
  }

  function cacheElements() {
    els.input = document.getElementById("script-input");
    els.charCount = document.getElementById("char-count");
    els.draftStatus = document.getElementById("draft-status");
    els.checkButton = document.getElementById("check-button");
    els.loadExample = document.getElementById("load-example");
    els.clearText = document.getElementById("clear-text");
    els.fileInput = document.getElementById("file-input");
    els.matches = document.getElementById("matches");
    els.emptyState = document.getElementById("empty-state");
    els.resultToolbar = document.getElementById("result-toolbar");
    els.summaryCount = document.getElementById("summary-count");
    els.summaryDetail = document.getElementById("summary-detail");
    els.acceptAll = document.getElementById("accept-all");
    els.resetAll = document.getElementById("reset-all");
    els.glossarySearch = document.getElementById("glossary-search");
    els.glossaryList = document.getElementById("glossary-list");
    els.exportOutput = document.getElementById("export-output");
    els.exportHint = document.getElementById("export-hint");
    els.copyExport = document.getElementById("copy-export");
    els.downloadExport = document.getElementById("download-export");
    els.toast = document.getElementById("toast");
    els.tabs = Array.from(document.querySelectorAll("[data-tab]"));
    els.panels = {
      results: document.getElementById("results-panel"),
      glossary: document.getElementById("glossary-panel")
    };
    els.exportTabs = Array.from(document.querySelectorAll("[data-export]"));
  }

  function bindEvents() {
    els.checkButton.addEventListener("click", function () {
      checkText();
    });

    els.loadExample.addEventListener("click", function () {
      els.input.value = EXAMPLE_TEXT;
      updateCharCount();
      checkText();
    });

    els.clearText.addEventListener("click", function () {
      els.input.value = "";
      state.matches = [];
      state.selected.clear();
      state.confirmed.clear();
      state.ignored.clear();
      state.lastCheckedText = "";
      state.stale = false;
      try {
        localStorage.removeItem(STORAGE_KEY);
      } catch (error) {
        // Local storage may be unavailable in private mode.
      }
      els.draftStatus.textContent = "草稿自动保存在本机";
      updateCharCount();
      renderMatches();
      updateSummary();
      renderExport();
      els.input.focus();
    });

    els.input.addEventListener("input", function () {
      updateCharCount();
      saveDraft();
      const wasStale = state.stale;
      state.stale = state.matches.length > 0 && els.input.value !== state.lastCheckedText;
      if (state.matches.length && els.input.value !== state.lastCheckedText) {
        els.draftStatus.textContent = "文案已修改，重新检查后更新结果";
      } else if (!state.matches.length) {
        els.draftStatus.textContent = "草稿自动保存在本机";
      }
      if (wasStale !== state.stale) {
        updateSummary();
        renderExport();
      }
    });

    els.input.addEventListener("keydown", function (event) {
      if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
        event.preventDefault();
        checkText();
      }
    });

    els.fileInput.addEventListener("change", function () {
      const file = els.fileInput.files && els.fileInput.files[0];
      if (!file) {
        return;
      }
      const reader = new FileReader();
      reader.onload = function () {
        const buffer = reader.result;
        let raw = "";
        try {
          raw = new TextDecoder("utf-8", { fatal: false }).decode(buffer);
          if (raw.indexOf("\uFFFD") !== -1) {
            raw = new TextDecoder("gbk").decode(buffer);
          }
        } catch (error) {
          raw = new TextDecoder("utf-8").decode(buffer);
        }
        const text = /\.srt$/i.test(file.name) ? stripSrt(raw) : raw.trim();
        els.input.value = text;
        updateCharCount();
        checkText();
        els.fileInput.value = "";
      };
      reader.readAsArrayBuffer(file);
    });

    els.tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        setTab(tab.getAttribute("data-tab"));
      });
    });

    els.glossarySearch.addEventListener("input", function () {
      state.glossaryQuery = els.glossarySearch.value;
      renderGlossary();
    });

    els.matches.addEventListener("change", function (event) {
      const input = event.target;
      if (!input.matches || input.type !== "radio") {
        return;
      }
      const card = input.closest(".match-card");
      if (!card) {
        return;
      }
      const key = card.getAttribute("data-key");
      state.selected.set(key, input.value);
      state.confirmed.delete(key);
      refreshCard(card);
      updateSummary();
      renderExport();
    });

    els.matches.addEventListener("click", function (event) {
      const button = event.target.closest("[data-action]");
      if (!button) {
        return;
      }
      const card = button.closest(".match-card");
      if (!card) {
        return;
      }
      const key = card.getAttribute("data-key");
      const match = state.matches.find(function (item) {
        return item.key === key;
      });
      if (!match) {
        return;
      }

      const action = button.getAttribute("data-action");
      if (action === "confirm") {
        if (state.confirmed.has(key)) {
          state.confirmed.delete(key);
        } else {
          state.confirmed.add(key);
          state.ignored.delete(key);
        }
        refreshCard(card);
        updateSummary();
        renderExport();
      }

      if (action === "ignore") {
        if (state.ignored.has(key)) {
          state.ignored.delete(key);
        } else {
          state.ignored.add(key);
          state.confirmed.delete(key);
        }
        refreshCard(card);
        updateSummary();
        renderExport();
      }

      if (action === "report") {
        exportCase(match);
      }
    });

    els.acceptAll.addEventListener("click", function () {
      state.matches.forEach(function (match) {
        state.selected.set(match.key, match.entry.default);
        state.confirmed.add(match.key);
        state.ignored.delete(match.key);
      });
      renderMatches();
      updateSummary();
      renderExport();
      toast("已全部采用建议读音");
    });

    els.resetAll.addEventListener("click", function () {
      state.matches.forEach(function (match) {
        state.selected.set(match.key, match.entry.default);
        state.confirmed.delete(match.key);
        state.ignored.delete(match.key);
      });
      renderMatches();
      updateSummary();
      renderExport();
      toast("已重置选择");
    });

    els.exportTabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        state.exportMode = tab.getAttribute("data-export");
        els.exportTabs.forEach(function (item) {
          item.classList.toggle("is-active", item === tab);
        });
        renderExport();
      });
    });

    els.copyExport.addEventListener("click", function () {
      copyText(els.exportOutput.textContent || "");
    });

    els.downloadExport.addEventListener("click", function () {
      const text = els.exportOutput.textContent || "";
      if (!text.trim()) {
        toast("还没有可导出的内容");
        return;
      }
      const extension = state.exportMode === "annotated" ? "txt" : "xml";
      downloadFile("多音字修正稿-" + state.exportMode + "." + extension, text);
    });
  }

  function configureLinks() {
    if (!APP.repoUrl) {
      return;
    }
    document.querySelectorAll(".js-repo-link").forEach(function (link) {
      link.href = APP.repoUrl;
      link.hidden = false;
    });
  }

  function checkText(options) {
    const opts = options || {};
    const text = els.input.value;

    if (!text.trim()) {
      toast("先粘贴一段文案，或载入示例");
      els.input.focus();
      return;
    }

    state.matches = detectMatches(text);
    state.lastCheckedText = text;
    state.stale = false;
    state.selected.clear();
    state.confirmed.clear();
    state.ignored.clear();

    state.matches.forEach(function (match) {
      state.selected.set(match.key, match.entry.default);
    });

    els.draftStatus.textContent = "草稿已保存在本机";
    renderMatches();
    updateSummary();
    renderExport();

    if (!opts.silent) {
      setTab("results");
      if (state.matches.length === 0) {
        toast("没有命中词库中的高风险词，可以补充词库");
      } else {
        toast("检测到 " + state.matches.length + " 处，请确认读音");
      }
      if (window.innerWidth < 980) {
        document.getElementById("checker").scrollIntoView({ behavior: "smooth", block: "start" });
      }
    }
  }

  function detectMatches(text) {
    const found = [];

    ENTRIES.forEach(function (entry) {
      if (!entry || !entry.word) {
        return;
      }
      const pattern = entry.pattern || escapeRegExp(entry.word);
      let regex;
      try {
        regex = new RegExp(pattern, "g");
      } catch (error) {
        return;
      }

      let match;
      while ((match = regex.exec(text)) !== null) {
        if (!match[0]) {
          regex.lastIndex += 1;
          continue;
        }
        found.push({
          entry: entry,
          start: match.index,
          end: match.index + match[0].length,
          text: match[0]
        });
      }
    });

    found.sort(function (a, b) {
      return a.start - b.start || (b.end - b.start) - (a.end - a.start);
    });

    const result = [];
    let lastEnd = -1;
    found.forEach(function (item) {
      if (item.start < lastEnd) {
        return;
      }
      const key = item.entry.id + "@" + item.start + "-" + item.end;
      result.push({
        entry: item.entry,
        start: item.start,
        end: item.end,
        text: item.text,
        key: key,
        contextBefore: text.slice(Math.max(0, item.start - 16), item.start),
        contextAfter: text.slice(item.end, item.end + 16)
      });
      lastEnd = item.end;
    });

    return result;
  }

  function renderMatches() {
    els.matches.innerHTML = "";
    const hasMatches = state.matches.length > 0;
    els.emptyState.hidden = hasMatches;
    els.resultToolbar.hidden = !hasMatches;

    if (!hasMatches) {
      return;
    }

    const fragment = document.createDocumentFragment();
    state.matches.forEach(function (match) {
      fragment.appendChild(createMatchCard(match));
    });
    els.matches.appendChild(fragment);
  }

  function createMatchCard(match) {
    const entry = match.entry;
    const selected = state.selected.get(match.key) || entry.default;
    const confirmed = state.confirmed.has(match.key);
    const ignored = state.ignored.has(match.key);
    const card = document.createElement("article");
    card.className = "match-card risk-" + (entry.risk || "medium");
    if (confirmed) {
      card.classList.add("is-confirmed");
    }
    if (ignored) {
      card.classList.add("is-ignored");
    }
    card.setAttribute("data-key", match.key);

    const readings = entry.readings.map(function (reading) {
      const checked = reading.pinyin === selected ? " checked" : "";
      const wrongClass = reading.wrong ? " is-wrong" : "";
      return '<label class="reading' + wrongClass + '">' +
        '<input type="radio" name="reading-' + escapeHtml(match.key) + '" value="' + escapeHtml(reading.pinyin) + '"' + checked + '>' +
        '<span><span class="reading-main">' + escapeHtml(formatTone(reading.pinyin)) + '</span>' +
        '<span class="reading-note">' + escapeHtml(reading.note || "") + '</span></span>' +
        '</label>';
    }).join("");

    card.innerHTML =
      '<div class="match-head">' +
        '<span class="match-word">' + escapeHtml(entry.word) + '</span>' +
        '<span class="badge">' + escapeHtml(TYPE_LABELS[entry.type] || "词语") + '</span>' +
        '<span class="badge risk">' + escapeHtml(RISK_LABELS[entry.risk] || "易错") + '</span>' +
        (confirmed ? '<span class="badge confirmed">已确认</span>' : '') +
      '</div>' +
      '<p class="match-context">' + escapeHtml(match.contextBefore) + '<mark>' + escapeHtml(match.text) + '</mark>' + escapeHtml(match.contextAfter) + '</p>' +
      '<div class="readings">' + readings + '</div>' +
      '<p class="match-note">' + escapeHtml(entry.note || "") + '</p>' +
      '<div class="match-actions">' +
        '<button class="btn btn-primary" type="button" data-action="confirm">' + (confirmed ? "取消确认" : "确认读音") + '</button>' +
        '<button class="btn btn-ghost" type="button" data-action="ignore">' + (ignored ? "恢复" : "忽略") + '</button>' +
        '<button class="btn btn-ghost" type="button" data-action="report">导出案例</button>' +
      '</div>';

    return card;
  }

  function refreshCard(card) {
    const key = card.getAttribute("data-key");
    const match = state.matches.find(function (item) {
      return item.key === key;
    });
    if (!match) {
      return;
    }

    const confirmed = state.confirmed.has(key);
    const ignored = state.ignored.has(key);
    card.classList.toggle("is-confirmed", confirmed);
    card.classList.toggle("is-ignored", ignored);

    const head = card.querySelector(".match-head");
    const existingBadge = head.querySelector(".badge.confirmed");
    if (confirmed && !existingBadge) {
      const badge = document.createElement("span");
      badge.className = "badge confirmed";
      badge.textContent = "已确认";
      head.appendChild(badge);
    } else if (!confirmed && existingBadge) {
      existingBadge.remove();
    }

    const confirmButton = card.querySelector('[data-action="confirm"]');
    const ignoreButton = card.querySelector('[data-action="ignore"]');
    if (confirmButton) {
      confirmButton.textContent = confirmed ? "取消确认" : "确认读音";
    }
    if (ignoreButton) {
      ignoreButton.textContent = ignored ? "恢复" : "忽略";
    }
  }

  function updateSummary() {
    const total = state.matches.length;
    const ignored = state.matches.filter(function (match) {
      return state.ignored.has(match.key);
    }).length;
    const active = state.matches.filter(function (match) {
      return !state.ignored.has(match.key);
    });
    const confirmed = active.filter(function (match) {
      return state.confirmed.has(match.key);
    }).length;
    const pending = active.length - confirmed;

    els.summaryCount.textContent = String(total);

    if (!total) {
      els.summaryDetail.textContent = "粘贴文案后点击检查";
      return;
    }

    if (state.stale) {
      els.summaryDetail.textContent = "文案已修改，请重新检查";
      return;
    }

    const parts = ["已确认 " + confirmed + " 处", "待确认 " + pending + " 处"];
    if (ignored) {
      parts.push("已忽略 " + ignored + " 处");
    }
    els.summaryDetail.textContent = parts.join(" · ");
  }

  function renderExport() {
    const active = state.matches.filter(function (match) {
      return !state.ignored.has(match.key);
    });
    const pending = active.filter(function (match) {
      return !state.confirmed.has(match.key);
    }).length;

    if (!state.matches.length) {
      els.exportOutput.textContent = "检查后，这里会生成可复制的修正稿。";
      els.exportHint.textContent = "";
      els.exportHint.classList.remove("is-warning");
      return;
    }

    let output = "";
    if (state.exportMode === "annotated") {
      output = buildPlainAnnotation();
    } else {
      output = buildSsml(state.exportMode);
    }

    els.exportOutput.textContent = output;
    if (state.stale) {
      els.exportHint.textContent = "文案已修改，导出仍是上次检查的版本；请重新检查。";
      els.exportHint.classList.add("is-warning");
      return;
    }
    if (pending) {
      els.exportHint.textContent = "还有 " + pending + " 处未确认，导出使用当前选中的读音。";
      els.exportHint.classList.add("is-warning");
    } else {
      els.exportHint.textContent = "已按确认的读音生成。";
      els.exportHint.classList.remove("is-warning");
    }
  }

  function buildPlainAnnotation() {
    return buildWithMatches(function (match, reading) {
      return match.text + "（" + formatTone(reading) + "）";
    });
  }

  function buildSsml(kind) {
    const body = buildWithMatches(function (match, reading) {
      const text = escapeXml(match.text);
      if (kind === "tencent") {
        return '<phoneme alphabet="py" ph="' + reading + '">' + text + '</phoneme>';
      }
      return '<phoneme py="' + reading + '">' + text + '</phoneme>';
    });
    return "<speak>\n" + body + "\n</speak>";
  }

  function buildWithMatches(transform) {
    const text = state.lastCheckedText || els.input.value;
    let cursor = 0;
    let output = "";

    state.matches.forEach(function (match) {
      if (match.start < cursor) {
        return;
      }
      output += text.slice(cursor, match.start);
      if (state.ignored.has(match.key)) {
        output += text.slice(match.start, match.end);
      } else {
        const reading = state.selected.get(match.key) || match.entry.default;
        output += transform(match, reading);
      }
      cursor = match.end;
    });

    output += text.slice(cursor);
    return output;
  }

  function exportCase(match) {
    const selected = state.selected.get(match.key) || match.entry.default;
    const start = Math.max(0, match.start - 30);
    const end = Math.min(els.input.value.length, match.end + 30);
    const payload = {
      type: "pronunciation-case",
      word: match.text,
      entryId: match.entry.id,
      context: els.input.value.slice(start, end),
      expected: selected,
      candidates: match.entry.readings.map(function (reading) {
        return reading.pinyin;
      }),
      note: match.entry.note || "",
      createdAt: new Date().toISOString()
    };
    downloadFile("case-" + match.entry.id + ".json", JSON.stringify(payload, null, 2));
    toast("案例 JSON 已下载，可贴到 GitHub Issue");
  }

  function setTab(name) {
    els.tabs.forEach(function (tab) {
      const active = tab.getAttribute("data-tab") === name;
      tab.classList.toggle("is-active", active);
      tab.setAttribute("aria-selected", active ? "true" : "false");
    });
    Object.keys(els.panels).forEach(function (key) {
      const panel = els.panels[key];
      if (panel) {
        panel.classList.toggle("is-active", key === name);
      }
    });
  }

  function renderGlossary() {
    const query = state.glossaryQuery.trim().toLowerCase();
    const entries = ENTRIES.filter(function (entry) {
      if (!query) {
        return true;
      }
      const haystack = [
        entry.word,
        entry.default,
        entry.note,
        TYPE_LABELS[entry.type],
        entry.readings.map(function (reading) {
          return reading.pinyin + " " + (reading.note || "");
        }).join(" ")
      ].join(" ").toLowerCase();
      return haystack.indexOf(query) !== -1;
    });

    if (!entries.length) {
      els.glossaryList.innerHTML = '<p class="glossary-empty">没有找到匹配的词条，欢迎补充。</p>';
      return;
    }

    const fragment = document.createDocumentFragment();
    entries.forEach(function (entry) {
      const item = document.createElement("article");
      item.className = "glossary-item";
      const chips = entry.readings.map(function (reading) {
        const isDefault = reading.pinyin === entry.default;
        return '<span class="reading-chip' + (isDefault ? " is-default" : "") + '">' +
          escapeHtml(formatTone(reading.pinyin)) +
          '</span>';
      }).join("");

      item.innerHTML =
        '<div class="glossary-item-head">' +
          '<span class="glossary-word">' + escapeHtml(entry.word) + ' <span class="badge">' + escapeHtml(TYPE_LABELS[entry.type] || "词语") + '</span></span>' +
          '<span class="glossary-pinyin">' + escapeHtml(formatTone(entry.default)) + '</span>' +
        '</div>' +
        '<p class="glossary-note">' + escapeHtml(entry.note || "") + '</p>' +
        '<div class="glossary-readings">' + chips + '</div>';

      fragment.appendChild(item);
    });

    els.glossaryList.innerHTML = "";
    els.glossaryList.appendChild(fragment);
  }

  function updateCharCount() {
    const count = Array.from(els.input.value.replace(/\s/g, "")).length;
    els.charCount.textContent = count + " 字";
  }

  let saveTimer = 0;
  function saveDraft() {
    window.clearTimeout(saveTimer);
    saveTimer = window.setTimeout(function () {
      try {
        localStorage.setItem(STORAGE_KEY, els.input.value);
      } catch (error) {
        // Ignore storage failures.
      }
    }, 300);
  }

  function restoreDraft() {
    try {
      const draft = localStorage.getItem(STORAGE_KEY);
      if (draft) {
        els.input.value = draft;
        els.draftStatus.textContent = "已恢复本机草稿";
      }
    } catch (error) {
      // Ignore storage failures.
    }
  }

  function stripSrt(raw) {
    const lines = String(raw || "").replace(/\r/g, "").split("\n");
    const output = [];
    lines.forEach(function (line) {
      const trimmed = line.trim();
      if (!trimmed) {
        output.push("");
        return;
      }
      if (/^\d+$/.test(trimmed)) {
        return;
      }
      if (/^\d{2}:\d{2}:\d{2}[,.]\d{3}\s*-->/.test(trimmed)) {
        return;
      }
      if (/^WEBVTT$/i.test(trimmed)) {
        return;
      }
      output.push(trimmed);
    });
    return output.join("\n").replace(/\n{3,}/g, "\n\n").trim();
  }

  function formatTone(pinyin) {
    if (!pinyin) {
      return "";
    }
    return pinyin.split(/\s+/).map(formatSyllable).join(" ");
  }

  function formatSyllable(syllable) {
    const match = String(syllable).match(/^([a-zA-ZüÜvV]+)([1-5])?$/);
    if (!match) {
      return syllable;
    }

    const base = match[1].toLowerCase().replace(/v/g, "ü");
    const tone = match[2] ? Number(match[2]) : 5;
    if (tone === 5) {
      return base;
    }

    let vowel = "";
    if (base.indexOf("a") !== -1) {
      vowel = "a";
    } else if (base.indexOf("o") !== -1) {
      vowel = "o";
    } else if (base.indexOf("e") !== -1) {
      vowel = "e";
    } else if (base.indexOf("iu") !== -1) {
      vowel = "u";
    } else if (base.indexOf("ui") !== -1) {
      vowel = "i";
    } else {
      for (let index = base.length - 1; index >= 0; index -= 1) {
        if (base[index] === "i" || base[index] === "u" || base[index] === "ü") {
          vowel = base[index];
          break;
        }
      }
    }

    if (!vowel || !TONE_MARKS[vowel]) {
      return base;
    }

    let position;
    if (base.indexOf("iu") !== -1) {
      position = base.indexOf("u", base.indexOf("i"));
    } else if (base.indexOf("ui") !== -1) {
      position = base.indexOf("i");
    } else {
      position = base.indexOf(vowel);
    }

    if (position === -1) {
      return base;
    }

    return base.slice(0, position) + TONE_MARKS[vowel][tone - 1] + base.slice(position + 1);
  }

  function escapeRegExp(value) {
    return String(value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  function escapeHtml(value) {
    return String(value == null ? "" : value).replace(/[&<>"']/g, function (character) {
      return {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;"
      }[character];
    });
  }

  function escapeXml(value) {
    return escapeHtml(value);
  }

  function copyText(text) {
    if (!text.trim()) {
      toast("还没有可复制的内容");
      return;
    }
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(function () {
        toast("已复制到剪贴板");
      }).catch(function () {
        fallbackCopy(text);
      });
      return;
    }
    fallbackCopy(text);
  }

  function fallbackCopy(text) {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    try {
      document.execCommand("copy");
      toast("已复制到剪贴板");
    } catch (error) {
      toast("复制失败，请手动选择文本");
    }
    textarea.remove();
  }

  function downloadFile(filename, text) {
    const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(function () {
      URL.revokeObjectURL(url);
    }, 1000);
  }

  let toastTimer = 0;
  function toast(message) {
    els.toast.textContent = message;
    els.toast.classList.add("is-visible");
    window.clearTimeout(toastTimer);
    toastTimer = window.setTimeout(function () {
      els.toast.classList.remove("is-visible");
    }, 2400);
  }

  init();
})();