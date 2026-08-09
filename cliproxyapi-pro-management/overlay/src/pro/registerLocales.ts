import i18n from '@/i18n';
import proLocales from './locales.generated.json';

Object.entries(proLocales).forEach(([language, resources]) => {
  i18n.addResourceBundle(language, 'translation', resources, true, true);
});
