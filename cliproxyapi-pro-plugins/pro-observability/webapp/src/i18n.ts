import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import en from './i18n/locales/en.json';
import ru from './i18n/locales/ru.json';
import zhCN from './i18n/locales/zh-CN.json';
import zhTW from './i18n/locales/zh-TW.json';
import additions from './i18n/monitoring-locales.json';

type Dictionary = Record<string, any>;
const merge = (base: Dictionary, extra: Dictionary): Dictionary => {
  const result = { ...base };
  Object.entries(extra).forEach(([key, value]) => {
    result[key] = value && typeof value === 'object' && !Array.isArray(value)
      ? merge((result[key] as Dictionary) || {}, value as Dictionary)
      : value;
  });
  return result;
};

const resources = {
  en: { translation: merge(en, additions['en.json']) },
  ru: { translation: merge(ru, additions['ru.json']) },
  'zh-CN': { translation: merge(zhCN, additions['zh-CN.json']) },
  'zh-TW': { translation: merge(zhTW, additions['zh-TW.json']) },
};

void i18n.use(initReactI18next).init({
  resources,
  lng: 'zh-CN',
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
});

export const setPluginLanguage = async (language: string) => {
  const normalized = language.toLowerCase();
  const target = normalized.startsWith('zh-tw') || normalized.startsWith('zh-hk')
    ? 'zh-TW'
    : normalized.startsWith('zh')
      ? 'zh-CN'
      : normalized.startsWith('ru') ? 'ru' : 'en';
  await i18n.changeLanguage(target);
};

export default i18n;
