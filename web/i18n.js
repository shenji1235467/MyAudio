/* MyAudio 轻量 i18n 引擎（零依赖、零构建）
 * 用法：
 *   t('key') / t('key', { name: 'x' })
 *   HTML: data-i18n="key"（文本）、data-i18n-placeholder / data-i18n-title / data-i18n-aria-label / data-i18n-alt
 *   I18N.setLang('en') / I18N.getLang() / I18N.toggleLang()
 */
(function () {
  'use strict';

  var STORAGE_KEY = 'myaudio_lang';
  var SUPPORTED = ['zh-CN', 'en'];
  var FALLBACK = 'zh-CN';

  // 页面可能在 locale 文件加载前就引用 I18N，先建桩对象
  window.I18N = window.I18N || {};
  var I18N = window.I18N;
  I18N._locales = I18N._locales || {};

  function detectLang() {
    var saved = localStorage.getItem(STORAGE_KEY);
    if (saved && SUPPORTED.indexOf(saved) !== -1) return saved;
    var nav = (navigator.language || '').toLowerCase();
    if (nav.indexOf('zh') === 0) return 'zh-CN';
    return 'en';
  }

  var currentLang = detectLang();

  function t(key, params) {
    var dict = I18N._locales[currentLang] || {};
    var text = dict[key];
    if (text === undefined) {
      dict = I18N._locales[FALLBACK] || {};
      text = dict[key];
    }
    if (text === undefined) return key;
    if (params) {
      text = text.replace(/\{(\w+)\}/g, function (_, k) {
        return params[k] !== undefined ? String(params[k]) : '{' + k + '}';
      });
    }
    return text;
  }

  function parseParams(el) {
    var raw = el.getAttribute('data-i18n-params');
    if (!raw) return undefined;
    try { return JSON.parse(raw); } catch (e) { return undefined; }
  }

  function applyDOM(root) {
    root = root || document;
    // 文本内容（支持 data-i18n-params 传参）
    root.querySelectorAll('[data-i18n]').forEach(function (el) {
      el.textContent = t(el.getAttribute('data-i18n'), parseParams(el));
    });
    // 属性
    var attrMap = [
      ['data-i18n-placeholder', 'placeholder'],
      ['data-i18n-title', 'title'],
      ['data-i18n-aria-label', 'aria-label'],
      ['data-i18n-alt', 'alt'],
    ];
    attrMap.forEach(function (pair) {
      root.querySelectorAll('[' + pair[0] + ']').forEach(function (el) {
        el.setAttribute(pair[1], t(el.getAttribute(pair[0])));
      });
    });
    // <title>
    var titleEl = document.querySelector('title[data-i18n]');
    if (titleEl) document.title = t(titleEl.getAttribute('data-i18n'));
    // <html lang>
    document.documentElement.lang = currentLang === 'zh-CN' ? 'zh-CN' : 'en';
  }

  function setLang(lang) {
    if (SUPPORTED.indexOf(lang) === -1) return;
    currentLang = lang;
    localStorage.setItem(STORAGE_KEY, lang);
    applyDOM();
    // 通知业务代码语言已切换（app.js 监听后重建动态 DOM）
    document.dispatchEvent(new CustomEvent('langchange', { detail: { lang: lang } }));
  }

  function getLang() { return currentLang; }

  function toggleLang() {
    setLang(currentLang === 'zh-CN' ? 'en' : 'zh-CN');
  }

  I18N.t = t;
  I18N.setLang = setLang;
  I18N.getLang = getLang;
  I18N.toggleLang = toggleLang;
  I18N.applyDOM = applyDOM;
  I18N.SUPPORTED = SUPPORTED;

  // 暴露全局 t() 简写
  window.t = t;
})();
