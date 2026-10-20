// Console preferences: language and theme, both persisted.
//
// They live in one hook because they are coupled through the document: the same
// effect that applies the theme class also sets the page title from the language,
// so splitting them would mean two effects racing over document.title.

import { useCallback, useEffect, useState } from 'react';

const LANG_KEY = 'airoute_lang';
const THEME_KEY = 'airoute_theme';

function readStored(key, fallback) {
  try {
    return localStorage.getItem(key) || fallback;
  } catch {
    // Private browsing modes can throw on access.
    return fallback;
  }
}

function writeStored(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // A preference that cannot be persisted is still applied for this session.
  }
}

export function useUiPreferences({ translations }) {
  const [lang, setLang] = useState(() => readStored(LANG_KEY, 'zh'));
  const [theme, setTheme] = useState(() => readStored(THEME_KEY, 'light'));

  const t = translations[lang] || translations.zh;
  const isZh = lang === 'zh';

  const toggleLang = useCallback(() => {
    setLang((prev) => {
      const nextLang = prev === 'zh' ? 'en' : 'zh';
      writeStored(LANG_KEY, nextLang);
      return nextLang;
    });
  }, []);

  const toggleTheme = useCallback(() => {
    setTheme((prev) => (prev === 'dark' ? 'light' : 'dark'));
  }, []);

  // Applies the theme class and the localised title together: they are set from the
  // same two values, so one effect keeps them consistent.
  useEffect(() => {
    const root = document.documentElement;
    if (theme === 'dark') {
      root.classList.add('dark');
      root.classList.remove('light');
    } else {
      root.classList.remove('dark');
      root.classList.add('light');
    }
    writeStored(THEME_KEY, theme);
    document.title = lang === 'zh' ? 'Airoute · AI路由器' : 'Airoute';
  }, [theme, lang]);

  return { lang, setLang, theme, setTheme, toggleLang, toggleTheme, t, isZh };
}
