(function () {
  "use strict";

  const APP = window.APP_CONFIG || {};
  const STORAGE_KEY = "polyphone-checker:v2:draft";
  const EXAMPLE_TEXT = `欢迎来到重庆。我们公司在银行行业工作，行长今天出差了。
这次项目很重要，需要重新检查，不能重复犯错。
他长大了，喜欢音乐，也觉得这个角色很有挑战。
北京朝阳区和辽宁朝阳市，读音不一样。
六安、蚌埠、亳州、丽水、台州、乐清，这些地名你读对了吗？
曾先生、解老师、查女士、仇总、朴同学，姓氏读音也容易错。
薄荷、薄弱、单薄，这几个“薄”也不一样。`;

  const TYPE_LABELS = {
    place: "地名",
    surname: "姓氏",
    word: "词语",
    name: "专名",
    idiom: "成语",
    brand: "品牌"
  };

  const RISK_LABELS = {
    high: "高风险",
    medium: "中风险"
  };

  const TONE_MARKS = {
    a: ["ā", "á", "ǎ", "à"],
    o: ["ō", "ó", "ǒ", "ò"],
    e: ["ē", "é", "ě", "è"],
    i: ["ī", "í", "ǐ", "ì"],
    u: ["ū", "ú", "ǔ", "ù"],
    "ü": ["ǖ", "ǘ", "ǚ", "ǜ"]
  };

  const state = {
    user: null,
    view: "checker",
    matches: [],
    selected: new Map(),
    confirmed: new Set(),
    ignored: new Set(),
    exportMode: "annotated",
    lastCheckedText: "",
    stale: false,
    truncated: false,
    terms: [],
    cases: [],
    glossaryQuery: "",
    glossaryType: "",
    glossaryRisk: "",
    draftTimer: 0
  };

  const els = {};

  function init() {
    cacheElements();
    bindEvents();
    configureLinks();
    setAuthMode("login");
    restoreDraft();
    updateCharCount();
    setView("checker", false);
    refreshMe();
    loadStats();
    loadTermOptions();
    loadCases();

    const hash = window.location.hash.replace(/^#/, "");
    if (["checker", "glossary", "cases", "about"].includes(hash)) {
      setView(hash, false);
    }

    const params = new URLSearchParams(window.location.search);
    if (params.get("demo") === "1") {
      els.input.value = EXAMPLE_TEXT;
      updateCharCount();
      setView("checker", false);
      checkText();
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
    els.exportOutput = document.getElementById("export-output");
    els.exportHint = document.getElementById("export-hint");
    els.copyExport = document.getElementById("copy-export");
    els.downloadExport = document.getElementById("download-export");
    els.glossarySearch = document.getElementById("glossary-search");
    els.glossaryType = document.getElementById("glossary-type");
    els.glossaryRisk = document.getElementById("glossary-risk");
    els.glossaryList = document.getElementById("glossary-list");
    els.glossaryCount = document.getElementById("glossary-count");
    els.casesList = document.getElementById("cases-list");
    els.statsRow = document.getElementById("stats-row");
    els.caseForm = document.getElementById("case-form");
    els.caseTerm = document.getElementById("case-term");
    els.caseWrong = document.getElementById("case-wrong");
    els.casePinyin = document.getElementById("case-pinyin");
    els.caseNote = document.getElementById("case-note");
    els.caseFormStatus = document.getElementById("case-form-status");
    els.caseSubmit = document.getElementById("case-submit");
    els.adminPanel = document.getElementById("admin-panel");
    els.adminList = document.getElementById("admin-list");
    els.adminStatus = document.getElementById("admin-status");
    els.accountArea = document.getElementById("account-area");
    els.authDialog = document.getElementById("auth-dialog");
    els.authTitle = document.getElementById("auth-title");
    els.authError = document.getElementById("auth-error");
    els.loginForm = document.getElementById("login-form");
    els.registerForm = document.getElementById("register-form");
    els.loginEmail = document.getElementById("login-email");
    els.loginPassword = document.getElementById("login-password");
    els.registerName = document.getElementById("register-name");
    els.registerEmail = document.getElementById("register-email");
    els.registerPassword = document.getElementById("register-password");
    els.closeAuth = document.getElementById("close-auth");
    els.toast = document.getElementById("toast");
    els.views = Array.from(document.querySelectorAll(".view"));
    els.navLinks = Array.from(document.querySelectorAll("[data-view]"));
    els.exportTabs = Array.from(document.querySelectorAll("[data-export]"));
    els.authTabs = Array.from(document.querySelectorAll("[data-auth-mode]"));
  }

  function bindEvents() {
    els.navLinks.forEach(function (link) {
      link.addEventListener("click", function () {
        setView(link.getAttribute("data-view"));
      });
    });

    els.checkButton.addEventListener("click", function () {
      checkText();
    });

    els.loadExample.addEventListener("click", function () {
      els.input.value = EXAMPLE_TEXT;
      updateCharCount();
      setView("checker");
      checkText();
    });

    els.clearText.addEventListener("click", function () {
      els.input.value = "";
      clearResults();
      try {
        localStorage.removeItem(STORAGE_KEY);
      } catch (error) {
        // localStorage may be unavailable.
      }
      els.draftStatus.textContent = "草稿自动保存在本机";
      updateCharCount();
      els.input.focus();
    });

    els.input.addEventListener("input", function () {
      updateCharCount();
      saveDraft();
      const wasStale = state.stale;
      state.stale = state.matches.length > 0 && els.input.value !== state.lastCheckedText;
      if (state.matches.length && state.stale) {
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
        els.input.value = /\.srt$/i.test(file.name) ? stripSrt(raw) : raw.trim();
        updateCharCount();
        setView("checker");
        checkText();
        els.fileInput.value = "";
      };
      reader.readAsArrayBuffer(file);
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
      state.selected.set(card.getAttribute("data-key"), input.value);
      state.confirmed.delete(card.getAttribute("data-key"));
      renderMatches();
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
        renderMatches();
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
        renderMatches();
        updateSummary();
        renderExport();
      }
      if (action === "report") {
        prefillCase(match);
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

    let glossaryTimer = 0;
    els.glossarySearch.addEventListener("input", function () {
      state.glossaryQuery = els.glossarySearch.value;
      window.clearTimeout(glossaryTimer);
      glossaryTimer = window.setTimeout(loadTerms, 250);
    });
    els.glossaryType.addEventListener("change", function () {
      state.glossaryType = els.glossaryType.value;
      loadTerms();
    });
    els.glossaryRisk.addEventListener("change", function () {
      state.glossaryRisk = els.glossaryRisk.value;
      loadTerms();
    });

    document.getElementById("refresh-cases").addEventListener("click", function () {
      loadCases();
      loadStats();
    });
    document.getElementById("jump-case-form").addEventListener("click", function () {
      document.getElementById("case-form-panel").scrollIntoView({ behavior: "smooth", block: "start" });
      window.setTimeout(function () {
        els.caseWrong.focus();
      }, 260);
    });
    els.caseForm.addEventListener("submit", submitCase);
    els.adminStatus.addEventListener("change", loadAdminCases);
    els.adminList.addEventListener("click", function (event) {
      const button = event.target.closest("[data-moderate]");
      if (!button) {
        return;
      }
      moderateCase(button.getAttribute("data-moderate"), button.getAttribute("data-status"));
    });
    els.casesList.addEventListener("click", function (event) {
      const button = event.target.closest("[data-vote]");
      if (!button) {
        return;
      }
      voteCase(button.getAttribute("data-vote"), button.getAttribute("data-voted") === "true");
    });

    els.accountArea.addEventListener("click", function (event) {
      if (event.target.closest("[data-account='login']") || event.target.closest("#login-button")) {
        openAuth("login");
      }
      if (event.target.closest("[data-account='logout']")) {
        logout();
      }
    });

    els.authTabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        setAuthMode(tab.getAttribute("data-auth-mode"));
      });
    });
    els.closeAuth.addEventListener("click", function () {
      els.authDialog.close();
    });
    els.loginForm.addEventListener("submit", function (event) {
      submitAuth("login", event);
    });
    els.registerForm.addEventListener("submit", function (event) {
      submitAuth("register", event);
    });
  }

  function setView(name, scroll) {
    if (!["checker", "glossary", "cases", "about"].includes(name)) {
      name = "checker";
    }
    state.view = name;
    els.views.forEach(function (view) {
      view.classList.toggle("is-active", view.id === "view-" + name);
    });
    els.navLinks.forEach(function (link) {
      const active = link.getAttribute("data-view") === name;
      link.classList.toggle("is-active", active);
      link.setAttribute("aria-selected", active ? "true" : "false");
    });
    if (name === "glossary") {
      loadTerms();
    }
    if (name === "cases") {
      loadCases();
      loadStats();
      refreshAdminVisibility();
    }
    if (scroll !== false) {
      try {
        window.history.replaceState(null, "", "#" + name);
      } catch (error) {
        // Ignore history failures in file-like contexts.
      }
      window.scrollTo({ top: 0, behavior: "smooth" });
    }
  }
  async function checkText(options) {
    const opts = options || {};
    const text = els.input.value;
    if (!text.trim()) {
      clearResults();
      if (!opts.silent) {
        toast("请先输入要检查的文案");
      }
      return;
    }
    els.checkButton.disabled = true;
    els.checkButton.textContent = "检查中…";
    try {
      const data = await api("/api/check", {
        method: "POST",
        body: JSON.stringify({ text: text })
      });
      state.matches = Array.isArray(data.matches) ? data.matches : [];
      state.selected.clear();
      state.confirmed.clear();
      state.ignored.clear();
      state.matches.forEach(function (match) {
        state.selected.set(match.key, match.entry.default);
      });
      state.lastCheckedText = text;
      state.stale = false;
      state.truncated = Boolean(data.truncated);
      els.draftStatus.textContent = state.matches.length
        ? "已检查 " + state.matches.length + " 处 · 结果来自服务端词库"
        : "没有发现词库中的高风险多音字";
      renderMatches();
      updateSummary();
      renderExport();
      if (state.truncated) {
        toast("结果较多，已截断显示前 200 处");
      }
    } catch (error) {
      els.draftStatus.textContent = "检查失败，请稍后重试";
      if (!opts.silent) {
        toast(error.message);
      }
    } finally {
      els.checkButton.disabled = false;
      els.checkButton.textContent = "检查多音字";
    }
  }

  function clearResults() {
    state.matches = [];
    state.selected.clear();
    state.confirmed.clear();
    state.ignored.clear();
    state.lastCheckedText = "";
    state.stale = false;
    state.truncated = false;
    renderMatches();
    updateSummary();
    renderExport();
  }

  function renderMatches() {
    els.emptyState.hidden = state.matches.length > 0;
    els.resultToolbar.hidden = state.matches.length === 0;
    if (!state.matches.length) {
      els.matches.innerHTML = "";
      return;
    }
    const html = state.matches.map(function (match, index) {
      const entry = match.entry || {};
      const selected = state.selected.get(match.key) || entry.default;
      const confirmed = state.confirmed.has(match.key);
      const ignored = state.ignored.has(match.key);
      const classes = ["match-card", "risk-" + (entry.risk || "medium")];
      if (confirmed) {
        classes.push("is-confirmed");
      }
      if (ignored) {
        classes.push("is-ignored");
      }
      const readings = (entry.readings || []).map(function (reading) {
        const checked = reading.pinyin === selected ? " checked" : "";
        return '<label class="reading ' + (reading.wrong ? "is-wrong" : "") + '">' +
          '<input type="radio" name="reading-' + index + '" value="' + escapeHtml(reading.pinyin) + '"' + checked + '>' +
          '<span><span class="reading-main">' + escapeHtml(formatTone(reading.pinyin)) + '</span>' +
          '<span class="reading-note">' + escapeHtml(reading.note || (reading.wrong ? "常见误读" : "")) + '</span></span>' +
          '</label>';
      }).join("");
      const context = escapeHtml(match.context || match.text || "");
      const word = escapeHtml(match.text || "");
      const highlightedContext = word ? context.replace(word, "<mark>" + word + "</mark>") : context;
      return '<article class="' + classes.join(" ") + '" data-key="' + escapeHtml(match.key) + '">' +
        '<div class="match-head">' +
          '<span class="match-word">' + word + '</span>' +
          riskBadge(entry.risk) +
          '<span class="badge">' + escapeHtml(TYPE_LABELS[entry.type] || entry.type || "词语") + '</span>' +
          (confirmed ? '<span class="badge confirmed">已确认</span>' : "") +
          (ignored ? '<span class="badge">已忽略</span>' : "") +
        '</div>' +
        '<p class="match-context">' + highlightedContext + '</p>' +
        '<div class="readings">' + readings + '</div>' +
        (entry.note ? '<p class="match-note">' + escapeHtml(entry.note) + '</p>' : "") +
        '<div class="match-actions">' +
          '<button class="btn btn-ghost btn-small" type="button" data-action="confirm">' + (confirmed ? "取消确认" : "确认") + '</button>' +
          '<button class="text-button" type="button" data-action="ignore">' + (ignored ? "恢复" : "忽略") + '</button>' +
          '<button class="text-button" type="button" data-action="report">提交案例</button>' +
        '</div>' +
      '</article>';
    }).join("");
    els.matches.innerHTML = html;
  }

  function updateSummary() {
    const visible = state.matches.filter(function (match) {
      return !state.ignored.has(match.key);
    });
    const confirmed = visible.filter(function (match) {
      return state.confirmed.has(match.key);
    }).length;
    const pending = visible.length - confirmed;
    els.summaryCount.textContent = String(visible.length);
    let detail = "已确认 " + confirmed + " 处 · 待确认 " + pending + " 处";
    if (state.ignored.size) {
      detail += " · 已忽略 " + state.ignored.size + " 处";
    }
    if (state.stale) {
      detail = "文案已修改，请重新检查后再导出";
    }
    if (state.truncated) {
      detail += " · 结果已截断";
    }
    els.summaryDetail.textContent = state.matches.length ? detail : "粘贴文案后点击检查";
  }

  function renderExport() {
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
    const pending = state.matches.filter(function (match) {
      return !state.ignored.has(match.key) && !state.confirmed.has(match.key);
    }).length;
    if (state.stale) {
      els.exportHint.textContent = "文案已修改，当前导出基于上次检查结果，请重新检查。";
      els.exportHint.classList.add("is-warning");
    } else if (pending > 0) {
      els.exportHint.textContent = "还有 " + pending + " 处未确认，导出会使用当前选择。";
      els.exportHint.classList.add("is-warning");
    } else {
      els.exportHint.textContent = "标注稿里的拼音是给人看的；SSML 是给支持 phoneme 的 TTS 用的。";
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
    const chars = Array.from(text);
    let cursor = 0;
    let output = "";
    state.matches.forEach(function (match) {
      if (match.start < cursor) {
        return;
      }
      output += chars.slice(cursor, match.start).join("");
      if (state.ignored.has(match.key)) {
        output += chars.slice(match.start, match.end).join("");
      } else {
        const reading = state.selected.get(match.key) || match.entry.default;
        output += transform(match, reading);
      }
      cursor = match.end;
    });
    output += chars.slice(cursor).join("");
    return output;
  }

  async function loadTerms() {
    if (!els.glossaryList) {
      return;
    }
    const params = new URLSearchParams({ limit: "100" });
    if (state.glossaryQuery.trim()) {
      params.set("q", state.glossaryQuery.trim());
    }
    if (state.glossaryType) {
      params.set("type", state.glossaryType);
    }
    if (state.glossaryRisk) {
      params.set("risk", state.glossaryRisk);
    }
    try {
      const data = await api("/api/terms?" + params.toString());
      state.terms = Array.isArray(data.items) ? data.items : [];
      els.glossaryCount.textContent = (data.total || 0) + " 条词条";
      renderTerms();
      fillTermOptions(state.terms);
    } catch (error) {
      els.glossaryCount.textContent = "加载失败";
      els.glossaryList.innerHTML = '<p class="glossary-empty">词库加载失败，请稍后重试。</p>';
    }
  }

  function renderTerms() {
    if (!state.terms.length) {
      els.glossaryList.innerHTML = '<p class="glossary-empty">没有找到匹配的词条，欢迎提交案例补充。</p>';
      return;
    }
    els.glossaryList.innerHTML = state.terms.map(function (entry) {
      const readings = (entry.readings || []).map(function (reading) {
        const classes = ["reading-chip"];
        if (reading.pinyin === entry.default) {
          classes.push("is-default");
        }
        if (reading.wrong) {
          classes.push("is-wrong");
        }
        return '<span class="' + classes.join(" ") + '">' + escapeHtml(formatTone(reading.pinyin)) +
          (reading.wrong ? " · 误读" : "") + '</span>';
      }).join("");
      return '<article class="glossary-item">' +
        '<div class="glossary-item-head">' +
          '<span class="glossary-word">' + escapeHtml(entry.word) + '</span>' +
          '<span class="glossary-pinyin">' + escapeHtml(formatTone(entry.default)) + '</span>' +
        '</div>' +
        '<div class="glossary-readings">' +
          '<span class="badge ' + (entry.risk === "high" ? "risk" : "") + '">' + escapeHtml(RISK_LABELS[entry.risk] || entry.risk) + '</span>' +
          '<span class="badge">' + escapeHtml(TYPE_LABELS[entry.type] || entry.type) + '</span>' +
          readings +
        '</div>' +
        (entry.note ? '<p class="glossary-note">' + escapeHtml(entry.note) + '</p>' : '') +
      '</article>';
    }).join("");
  }

  async function loadTermOptions() {
    try {
      const data = await api("/api/terms?limit=100");
      fillTermOptions(data.items || []);
    } catch (error) {
      // The case form still works without a preselected term.
    }
  }

  function fillTermOptions(items) {
    if (!els.caseTerm) {
      return;
    }
    const current = els.caseTerm.value;
    const options = ['<option value="">不关联，提交新词</option>'];
    (items || []).forEach(function (entry) {
      options.push('<option value="' + escapeHtml(entry.id) + '">' + escapeHtml(entry.word) + ' · ' + escapeHtml(formatTone(entry.default)) + '</option>');
    });
    els.caseTerm.innerHTML = options.join("");
    if (current) {
      els.caseTerm.value = current;
    }
  }
  async function loadStats() {
    try {
      const stats = await api("/api/stats");
      const cards = [
        ["词条", stats.terms],
        ["已通过案例", stats.cases],
        ["待审核", stats.pending],
        ["贡献者", stats.contributors]
      ];
      els.statsRow.innerHTML = cards.map(function (card) {
        return '<div class="stat-card"><strong>' + Number(card[1] || 0) + '</strong><span>' + card[0] + '</span></div>';
      }).join("");
    } catch (error) {
      els.statsRow.innerHTML = "";
    }
  }

  async function loadCases() {
    try {
      const data = await api("/api/cases?limit=50");
      state.cases = Array.isArray(data.items) ? data.items : [];
      renderCases();
    } catch (error) {
      els.casesList.innerHTML = '<p class="glossary-empty">案例加载失败，请稍后重试。</p>';
    }
  }

  function renderCases() {
    if (!state.cases.length) {
      els.casesList.innerHTML = '<p class="glossary-empty">还没有已通过的案例。登录后提交第一个真实错读案例。</p>';
      return;
    }
    els.casesList.innerHTML = state.cases.map(function (item) {
      return caseCard(item, false);
    }).join("");
  }

  function caseCard(item, admin) {
    const title = item.term_word
      ? escapeHtml(item.term_word) + ' · <span class="glossary-pinyin">' + escapeHtml(formatTone(item.correct_pinyin)) + '</span>'
      : escapeHtml(formatTone(item.correct_pinyin));
    const actions = admin
      ? '<div class="admin-actions">' +
          '<button class="btn btn-ghost btn-small" type="button" data-moderate="' + escapeHtml(item.id) + '" data-status="approved">通过</button>' +
          '<button class="text-button" type="button" data-moderate="' + escapeHtml(item.id) + '" data-status="rejected">拒绝</button>' +
        '</div>'
      : '<button class="vote-button ' + (item.voted ? "is-voted" : "") + '" type="button" data-vote="' + escapeHtml(item.id) + '" data-voted="' + (item.voted ? "true" : "false") + '">▲ ' + Number(item.votes || 0) + '</button>';
    return '<article class="case-card ' + (admin ? "admin-card" : "") + '">' +
      '<div class="case-card-head">' +
        '<div><h3>' + title + '</h3>' +
        '<p class="case-meta">' + escapeHtml(item.author || "匿名") + ' · ' + escapeHtml(formatDate(item.created_at)) + (admin ? ' · ' + escapeHtml(statusLabel(item.status)) : '') + '</p></div>' +
        (admin ? actions : '') +
      '</div>' +
      '<p class="case-wrong">' + escapeHtml(item.wrong_text) + '</p>' +
      (item.note ? '<p class="case-note">' + escapeHtml(item.note) + '</p>' : '') +
      (admin ? '' : '<div class="case-actions">' + actions + '</div>') +
    '</article>';
  }

  async function voteCase(id, voted) {
    if (!state.user) {
      openAuth("login");
      toast("登录后才能投票");
      return;
    }
    try {
      await api("/api/cases/" + encodeURIComponent(id) + "/vote", { method: voted ? "DELETE" : "POST" });
      await loadCases();
    } catch (error) {
      toast(error.message);
    }
  }

  async function refreshAdminVisibility() {
    const canModerate = state.user && (state.user.role === "admin" || state.user.role === "moderator");
    els.adminPanel.hidden = !canModerate;
    if (canModerate) {
      await loadAdminCases();
    }
  }

  async function loadAdminCases() {
    const canModerate = state.user && (state.user.role === "admin" || state.user.role === "moderator");
    if (!canModerate || els.adminPanel.hidden) {
      return;
    }
    try {
      const status = els.adminStatus.value || "pending";
      const data = await api("/api/admin/cases?status=" + encodeURIComponent(status) + "&limit=50");
      const items = Array.isArray(data.items) ? data.items : [];
      els.adminList.innerHTML = items.length
        ? items.map(function (item) { return caseCard(item, true); }).join("")
        : '<p class="glossary-empty">这个队列里暂时没有案例。</p>';
    } catch (error) {
      els.adminList.innerHTML = '<p class="glossary-empty">审核队列加载失败。</p>';
    }
  }

  async function moderateCase(id, status) {
    try {
      await api("/api/admin/cases/" + encodeURIComponent(id) + "/moderate", {
        method: "POST",
        body: JSON.stringify({ status: status, note: "" })
      });
      toast(status === "approved" ? "案例已通过" : "案例已拒绝");
      await loadAdminCases();
      await loadCases();
      await loadStats();
    } catch (error) {
      toast(error.message);
    }
  }

  function prefillCase(match) {
    setView("cases");
    const selected = state.selected.get(match.key) || match.entry.default;
    els.caseTerm.value = match.entry.id || "";
    if (els.caseTerm.value !== match.entry.id) {
      els.caseTerm.value = "";
    }
    els.caseWrong.value = match.context || match.text || "";
    els.casePinyin.value = selected || "";
    els.caseNote.value = match.entry.note ? "参考：" + match.entry.note : "";
    setCaseStatus(state.user ? "已带入检查结果，确认后提交待审核。" : "登录后即可提交，内容已帮你填好。", Boolean(state.user));
    if (!state.user) {
      openAuth("login");
    }
    window.setTimeout(function () {
      document.getElementById("case-form-panel").scrollIntoView({ behavior: "smooth", block: "start" });
    }, 80);
  }

  async function submitCase(event) {
    event.preventDefault();
    if (!state.user) {
      openAuth("login");
      setCaseStatus("请先登录，再提交案例。", false);
      return;
    }
    const payload = {
      term_id: els.caseTerm.value,
      wrong_text: els.caseWrong.value.trim(),
      correct_pinyin: els.casePinyin.value.trim(),
      note: els.caseNote.value.trim()
    };
    if (!payload.wrong_text) {
      setCaseStatus("请填写错误文本或上下文。", false);
      return;
    }
    if (!payload.correct_pinyin) {
      setCaseStatus("请填写正确读音，例如 chong2 qing4。", false);
      return;
    }
    els.caseSubmit.disabled = true;
    els.caseSubmit.textContent = "提交中…";
    try {
      await api("/api/cases", { method: "POST", body: JSON.stringify(payload) });
      els.caseWrong.value = "";
      els.casePinyin.value = "";
      els.caseNote.value = "";
      setCaseStatus("已提交，等待审核。通过后会出现在案例库。", true);
      toast("案例已提交");
      await loadStats();
    } catch (error) {
      setCaseStatus(error.message, false);
    } finally {
      els.caseSubmit.disabled = false;
      els.caseSubmit.textContent = "提交待审核";
    }
  }

  function setCaseStatus(message, ok) {
    els.caseFormStatus.textContent = message;
    els.caseFormStatus.classList.toggle("is-ok", Boolean(ok));
  }

  async function refreshMe() {
    try {
      const data = await api("/api/auth/me");
      state.user = data.user || null;
    } catch (error) {
      state.user = null;
    }
    renderAccount();
    refreshAdminVisibility();
    if (state.user) {
      loadCases();
    }
  }

  function renderAccount() {
    if (state.user) {
      els.accountArea.innerHTML = '<span class="account-name">你好，<strong>' + escapeHtml(state.user.display_name) + '</strong></span>' +
        '<button class="btn btn-ghost btn-small" type="button" data-account="logout">退出</button>';
      return;
    }
    els.accountArea.innerHTML = '<span class="privacy-badge">服务端检查 · 不存原文</span>' +
      '<button class="btn btn-ghost btn-small" type="button" data-account="login">登录</button>';
  }

  function openAuth(mode) {
    setAuthMode(mode || "login");
    if (!els.authDialog.open) {
      els.authDialog.showModal();
    }
    window.setTimeout(function () {
      const input = mode === "register" ? els.registerName : els.loginEmail;
      if (input) {
        input.focus();
      }
    }, 60);
  }

  function setAuthMode(mode) {
    const register = mode === "register";
    els.authTitle.textContent = register ? "注册" : "登录";
    els.loginForm.hidden = register;
    els.registerForm.hidden = !register;
    els.authTabs.forEach(function (tab) {
      const active = tab.getAttribute("data-auth-mode") === mode;
      tab.classList.toggle("is-active", active);
      tab.setAttribute("aria-selected", active ? "true" : "false");
    });
    els.authError.textContent = "";
    els.authError.classList.remove("is-ok");
  }

  async function submitAuth(mode, event) {
    event.preventDefault();
    els.authError.textContent = "";
    els.authError.classList.remove("is-ok");
    let path = "/api/auth/login";
    let payload;
    if (mode === "register") {
      payload = {
        display_name: els.registerName.value.trim(),
        email: els.registerEmail.value.trim(),
        password: els.registerPassword.value
      };
      path = "/api/auth/register";
    } else {
      payload = {
        email: els.loginEmail.value.trim(),
        password: els.loginPassword.value
      };
    }
    const submit = event.submitter;
    if (submit) {
      submit.disabled = true;
    }
    try {
      const data = await api(path, { method: "POST", body: JSON.stringify(payload) });
      state.user = data.user;
      els.authDialog.close();
      renderAccount();
      await refreshAdminVisibility();
      await loadCases();
      toast(mode === "register" ? "账号已创建" : "已登录");
    } catch (error) {
      els.authError.textContent = error.message;
    } finally {
      if (submit) {
        submit.disabled = false;
      }
    }
  }

  async function logout() {
    try {
      await api("/api/auth/logout", { method: "POST" });
    } catch (error) {
      // Clear local state even if the network request failed.
    }
    state.user = null;
    renderAccount();
    els.adminPanel.hidden = true;
    await loadCases();
    toast("已退出登录");
  }

  function updateCharCount() {
    els.charCount.textContent = Array.from(els.input.value).length + " 字";
  }

  function restoreDraft() {
    try {
      const draft = localStorage.getItem(STORAGE_KEY);
      if (draft) {
        els.input.value = draft;
        const params = new URLSearchParams(window.location.search);
        if (params.get("demo") !== "1") {
          window.setTimeout(function () {
            checkText({ silent: true });
          }, 350);
        }
      }
    } catch (error) {
      // localStorage may be unavailable.
    }
  }

  function saveDraft() {
    window.clearTimeout(state.draftTimer);
    state.draftTimer = window.setTimeout(function () {
      try {
        if (els.input.value.trim()) {
          localStorage.setItem(STORAGE_KEY, els.input.value);
        } else {
          localStorage.removeItem(STORAGE_KEY);
        }
      } catch (error) {
        // Ignore storage quota and privacy mode errors.
      }
    }, 300);
  }

  async function api(path, options) {
    const opts = Object.assign({ credentials: "same-origin", headers: {} }, options || {});
    if (opts.body && !opts.headers["Content-Type"]) {
      opts.headers["Content-Type"] = "application/json";
    }
    const response = await fetch(path, opts);
    let data = {};
    try {
      data = await response.json();
    } catch (error) {
      data = {};
    }
    if (!response.ok) {
      const message = data.error || "请求失败（" + response.status + "）";
      const error = new Error(message);
      error.status = response.status;
      throw error;
    }
    return data;
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

  function riskBadge(risk) {
    return '<span class="badge ' + (risk === "high" ? "risk" : "") + '">' + escapeHtml(RISK_LABELS[risk] || risk || "风险") + '</span>';
  }

  function statusLabel(status) {
    return {
      pending: "待审核",
      approved: "已通过",
      rejected: "已拒绝"
    }[status] || status;
  }

  function formatDate(value) {
    if (!value) {
      return "";
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return "";
    }
    return date.toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit"
    });
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
    return String(pinyin).split(/\s+/).map(formatSyllable).join(" ");
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
    }, 2600);
  }

  init();
})();
