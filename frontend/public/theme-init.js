'use strict';

(function applyStoredPreferences() {
  var root = document.documentElement;

  function stored(key) {
    try {
      return window.localStorage.getItem(key);
    } catch {
      return null;
    }
  }

  var themePreference = stored('ovc.theme');
  var prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  var theme =
    themePreference === 'light' || themePreference === 'dark' ? themePreference : prefersDark ? 'dark' : 'light';
  root.dataset.theme = theme;
  root.dataset.themePreference = themePreference === 'light' || themePreference === 'dark' ? themePreference : 'system';

  var languagePreference = stored('ovc.language');
  var browserLanguage = (navigator.language || '').toLowerCase();
  var language =
    languagePreference === 'ru' || languagePreference === 'en'
      ? languagePreference
      : browserLanguage.indexOf('ru') === 0
        ? 'ru'
        : 'en';
  root.lang = language;
})();
