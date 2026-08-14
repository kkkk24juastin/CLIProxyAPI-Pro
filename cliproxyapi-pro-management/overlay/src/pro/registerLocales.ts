import i18n from '@/i18n';
import proLocales from './locales.generated.json';
import { apiKeyPolicyLocales } from './apiKeyPolicyLocales';

Object.entries(proLocales).forEach(([language, resources]) => {
  i18n.addResourceBundle(language, 'translation', resources, true, true);
});

Object.entries(apiKeyPolicyLocales).forEach(([language, resources]) => {
  i18n.addResourceBundle(language, 'translation', resources, true, true);
});
