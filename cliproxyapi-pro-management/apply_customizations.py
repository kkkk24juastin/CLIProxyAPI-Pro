#!/usr/bin/env python3
import hashlib
import json
import sys
from pathlib import Path

CUSTOMIZATION_DIR = Path(__file__).resolve().parent
OVERLAY_DIR = CUSTOMIZATION_DIR / 'overlay'
LOCALES_FILE = CUSTOMIZATION_DIR / 'monitoring-locales.json'
OVERLAY_REPLACEMENTS_FILE = CUSTOMIZATION_DIR / 'overlay-replacements.json'

MANAGEMENT_UPDATE_LOCALE_KEYS = {
    'en.json': {
        'management_check_update_button': 'Check for updates',
        'management_check_update_updated': 'Management Center updated. Reloading...',
        'management_check_update_unchanged': 'Update check completed; no update was applied.',
        'management_check_update_error': 'Failed to check Management Center update',
    },
    'ru.json': {
        'management_check_update_button': 'Проверить обновления',
        'management_check_update_updated': 'Центр управления обновлён. Перезагрузка...',
        'management_check_update_unchanged': 'Проверка завершена; обновление не применялось.',
        'management_check_update_error': 'Не удалось проверить обновление Центра управления',
    },
    'zh-CN.json': {
        'management_check_update_button': '检查更新',
        'management_check_update_updated': '管理中心已更新，正在重新加载...',
        'management_check_update_unchanged': '检查完成，本次未进行更新。',
        'management_check_update_error': '管理中心更新检查失败',
    },
    'zh-TW.json': {
        'management_check_update_button': '檢查更新',
        'management_check_update_updated': '管理中心已更新，正在重新載入...',
        'management_check_update_unchanged': '檢查完成，本次未進行更新。',
        'management_check_update_error': '管理中心更新檢查失敗',
    },
}

QUOTA_LOCALE_KEYS = {
    'en.json': {
        'cached_at': 'Updated',
        'just_now': 'Just now',
        'minutes_ago': '{{count}} minute ago',
        'minutes_ago_plural': '{{count}} minutes ago',
        'hours_ago': '{{count}} hour ago',
        'hours_ago_plural': '{{count}} hours ago',
        'days_ago': '{{count}} day ago',
        'days_ago_plural': '{{count}} days ago',
        'search_label': 'Search quota credentials',
        'search_placeholder': 'Search name, auth_index, type, provider, note, or plan. Use * as a wildcard',
        'no_search_results': 'No matching quota credentials',
        'no_search_results_desc': 'No quota credential matches the current search.',
        'refresh_progress': 'Refreshing {{completed}} / {{total}}',
        'refresh_job_started': 'Backend quota refresh started',
        'refresh_job_completed': 'Quota refresh finished: {{completed}} / {{total}}, {{failed}} failed',
        'refresh_job_incomplete': 'Quota refresh stopped at {{completed}} / {{total}}: {{message}}',
        'refresh_job_failed': 'Failed to start quota refresh: {{message}}',
        'refresh_job_conflict': 'Another account inspection or quota refresh is already running',
    },
    'ru.json': {
        'cached_at': 'Обновлено',
        'just_now': 'Только что',
        'minutes_ago': '{{count}} минуту назад',
        'minutes_ago_plural': '{{count}} минут назад',
        'hours_ago': '{{count}} час назад',
        'hours_ago_plural': '{{count}} часов назад',
        'days_ago': '{{count}} день назад',
        'days_ago_plural': '{{count}} дней назад',
        'search_label': 'Поиск конфигураций квот',
        'search_placeholder': 'Поиск по имени, auth_index, типу, провайдеру, заметке или тарифу; поддерживается *',
        'no_search_results': 'Подходящие конфигурации квот не найдены',
        'no_search_results_desc': 'Текущему запросу не соответствует ни одна конфигурация квот.',
        'refresh_progress': 'Обновление {{completed}} / {{total}}',
        'refresh_job_started': 'Фоновое обновление квот запущено',
        'refresh_job_completed': 'Обновление квот завершено: {{completed}} / {{total}}, ошибок: {{failed}}',
        'refresh_job_incomplete': 'Обновление остановлено на {{completed}} / {{total}}: {{message}}',
        'refresh_job_failed': 'Не удалось запустить обновление квот: {{message}}',
        'refresh_job_conflict': 'Уже выполняется проверка аккаунтов или обновление квот',
    },
    'zh-CN.json': {
        'cached_at': '更新于',
        'just_now': '刚刚',
        'minutes_ago': '{{count}} 分钟前',
        'hours_ago': '{{count}} 小时前',
        'days_ago': '{{count}} 天前',
        'search_label': '搜索配额配置文件',
        'search_placeholder': '搜索名称、auth_index、类型、提供商、备注或套餐，支持 * 通配',
        'no_search_results': '没有匹配的配额配置文件',
        'no_search_results_desc': '当前搜索条件下没有可显示的配额配置文件。',
        'refresh_progress': '正在刷新 {{completed}} / {{total}}',
        'refresh_job_started': '后端配额刷新任务已启动',
        'refresh_job_completed': '配额刷新完成：{{completed}} / {{total}}，失败 {{failed}} 个',
        'refresh_job_incomplete': '配额刷新在 {{completed}} / {{total}} 时停止：{{message}}',
        'refresh_job_failed': '启动配额刷新失败：{{message}}',
        'refresh_job_conflict': '已有账号检查或配额刷新任务正在运行',
    },
    'zh-TW.json': {
        'cached_at': '更新於',
        'just_now': '剛剛',
        'minutes_ago': '{{count}} 分鐘前',
        'hours_ago': '{{count}} 小時前',
        'days_ago': '{{count}} 天前',
        'search_label': '搜尋配額設定檔',
        'search_placeholder': '搜尋名稱、auth_index、類型、供應商、備註或套餐，支援 * 萬用字元',
        'no_search_results': '沒有符合的配額設定檔',
        'no_search_results_desc': '目前搜尋條件下沒有可顯示的配額設定檔。',
        'refresh_progress': '正在重新整理 {{completed}} / {{total}}',
        'refresh_job_started': '後端配額重新整理工作已啟動',
        'refresh_job_completed': '配額重新整理完成：{{completed}} / {{total}}，失敗 {{failed}} 個',
        'refresh_job_incomplete': '配額重新整理在 {{completed}} / {{total}} 時停止：{{message}}',
        'refresh_job_failed': '啟動配額重新整理失敗：{{message}}',
        'refresh_job_conflict': '已有帳號檢查或配額重新整理工作正在執行',
    },
}

GEMINI_CLI_LOCALE_KEYS = {
    'en.json': {
        'auth_filter': 'GeminiCLI',
        'quota': {
            'title': 'Gemini CLI Quota',
            'empty_title': 'No Gemini CLI Auth Files',
            'empty_desc': 'Upload a Gemini CLI credential to view remaining quota.',
            'idle': 'Click here to refresh quota',
            'loading': 'Loading quota...',
            'load_failed': 'Failed to load quota: {{message}}',
            'missing_auth_index': 'Auth file missing auth_index',
            'missing_project_id': 'Gemini CLI credential missing project ID',
            'empty_buckets': 'No quota data available',
            'remaining_amount': 'Remaining {{count}}',
            'tier_label': 'Tier',
            'tier_free': 'Gemini Code Assist Free',
            'tier_legacy': 'Gemini Code Assist Legacy',
            'tier_standard': 'Gemini Code Assist Standard',
            'tier_pro': 'Google AI Pro',
            'tier_ultra': 'Google AI Ultra',
            'credit_label': 'Google One AI Credits',
            'credit_amount': '{{count}} credits',
        },
    },
    'ru.json': {
        'auth_filter': 'GeminiCLI',
        'quota': {
            'title': 'Квота Gemini CLI',
            'empty_title': 'Файлы авторизации Gemini CLI отсутствуют',
            'empty_desc': 'Загрузите учётные данные Gemini CLI, чтобы увидеть оставшуюся квоту.',
            'idle': 'Не загружено. Нажмите "Обновить квоту".',
            'loading': 'Загрузка квоты...',
            'load_failed': 'Не удалось загрузить квоту: {{message}}',
            'missing_auth_index': 'В файле авторизации отсутствует auth_index',
            'missing_project_id': 'В учётных данных Gemini CLI отсутствует идентификатор проекта',
            'empty_buckets': 'Данные по квоте отсутствуют',
            'remaining_amount': 'Осталось {{count}}',
            'tier_label': 'Уровень',
            'tier_free': 'Gemini Code Assist Free',
            'tier_legacy': 'Gemini Code Assist Legacy',
            'tier_standard': 'Gemini Code Assist Standard',
            'tier_pro': 'Google AI Pro',
            'tier_ultra': 'Google AI Ultra',
            'credit_label': 'Google One AI кредиты',
            'credit_amount': '{{count}} кредитов',
        },
    },
    'zh-CN.json': {
        'auth_filter': 'GeminiCLI',
        'quota': {
            'title': 'Gemini CLI 额度',
            'empty_title': '暂无 Gemini CLI 认证',
            'empty_desc': '上传 Gemini CLI 认证文件后即可查看额度。',
            'idle': '点击此处刷新额度',
            'loading': '正在加载额度...',
            'load_failed': '额度获取失败：{{message}}',
            'missing_auth_index': '认证文件缺少 auth_index',
            'missing_project_id': 'Gemini CLI 凭证缺少 Project ID',
            'empty_buckets': '暂无额度数据',
            'remaining_amount': '剩余 {{count}}',
            'tier_label': '层级',
            'tier_free': 'Gemini Code Assist 免费版',
            'tier_legacy': 'Gemini Code Assist Legacy',
            'tier_standard': 'Gemini Code Assist Standard',
            'tier_pro': 'Google AI Pro',
            'tier_ultra': 'Google AI Ultra',
            'credit_label': 'Google One AI 积分',
            'credit_amount': '{{count}} 积分',
        },
    },
    'zh-TW.json': {
        'auth_filter': 'GeminiCLI',
        'quota': {
            'title': 'Gemini CLI 配額',
            'empty_title': '暫無 Gemini CLI 驗證',
            'empty_desc': '上傳 Gemini CLI 驗證檔案後即可查看配額。',
            'idle': '點擊此處重新整理配額',
            'loading': '正在載入配額...',
            'load_failed': '配額取得失敗：{{message}}',
            'missing_auth_index': '驗證檔案缺少 auth_index',
            'missing_project_id': 'Gemini CLI 憑證缺少 Project ID',
            'empty_buckets': '暫無配額資料',
            'remaining_amount': '剩餘 {{count}}',
            'tier_label': '層級',
            'tier_free': 'Gemini Code Assist 免費版',
            'tier_legacy': 'Gemini Code Assist Legacy',
            'tier_standard': 'Gemini Code Assist Standard',
            'tier_pro': 'Google AI Pro',
            'tier_ultra': 'Google AI Ultra',
            'credit_label': 'Google One AI 點數',
            'credit_amount': '{{count}} 點數',
        },
    },
}

XAI_QUOTA_LOCALE_KEYS = {
    'en.json': {
        'plan_free': 'Free',
        'plan_x_premium_plus': 'X Premium+',
        'plan_paid_unknown': 'Paid (unknown tier)',
        'free_quota': 'Free token quota',
        'free_quota_exhausted': 'Exhausted',
        'free_quota_window': 'Rolling 24 hours',
    },
    'ru.json': {
        'plan_free': 'Бесплатный',
        'plan_x_premium_plus': 'X Premium+',
        'plan_paid_unknown': 'Платный (неизвестный уровень)',
        'free_quota': 'Бесплатная квота токенов',
        'free_quota_exhausted': 'Исчерпана',
        'free_quota_window': 'Скользящие 24 часа',
    },
    'zh-CN.json': {
        'plan_free': '免费套餐',
        'plan_x_premium_plus': 'X Premium+',
        'plan_paid_unknown': '付费版（未知档位）',
        'free_quota': '免费 Token 额度',
        'free_quota_exhausted': '已耗尽',
        'free_quota_window': '滚动 24 小时',
    },
    'zh-TW.json': {
        'plan_free': '免費套餐',
        'plan_x_premium_plus': 'X Premium+',
        'plan_paid_unknown': '付費版（未知級別）',
        'free_quota': '免費 Token 配額',
        'free_quota_exhausted': '已用盡',
        'free_quota_window': '滾動 24 小時',
    },
}

AUTH_FILES_SEARCH_PLACEHOLDER_KEYS = {
    'en.json': 'Filter by name, auth_index, type, provider, note, or plan. Use * as a wildcard',
    'ru.json': 'Фильтр по имени, auth_index, типу, провайдеру, заметке или тарифу; поддерживается *',
    'zh-CN.json': '输入名称、auth_index、类型、提供方、备注或套餐关键字，支持 * 通配',
    'zh-TW.json': '輸入名稱、auth_index、類型、供應方、備註或套餐關鍵字，支援 * 萬用字元',
}

AUTH_FILES_PLAN_SORT_LABEL_KEYS = {
    'en.json': 'Plan: High to Low',
    'ru.json': 'Тариф: по убыванию',
    'zh-CN.json': '套餐从高到低',
    'zh-TW.json': '套餐由高到低',
}

AUTH_FILES_QUOTA_SORT_LABEL_KEYS = {
    'en.json': 'Available Quota: High to Low',
    'ru.json': 'Доступная квота: по убыванию',
    'zh-CN.json': '可用额度从高到低',
    'zh-TW.json': '可用額度由高到低',
}

AUTH_FILES_SELECTED_COUNT_LABEL_KEYS = {
    'en.json': 'Scheduled',
    'ru.json': 'Назначено',
    'zh-CN.json': '调度',
    'zh-TW.json': '調度',
}

AUTH_FILES_BATCH_LOCALE_KEYS = {
    'en.json': {
        'batch_test': 'Test Selected', 'batch_clear_errors': 'Clear Errors',
        'batch_test_title': 'Batch Test Results', 'batch_clear_errors_title': 'Clear Errors Results',
        'batch_test_running': 'Testing...', 'batch_clear_errors_running': 'Clearing errors...',
        'batch_no_auth_index': 'Missing auth_index',
        'batch_unsupported_provider': 'Unsupported provider: {{provider}}',
        'batch_clear_errors_failed': 'Failed to clear errors',
        'batch_result_success': 'Success {{count}}', 'batch_result_failed': 'Failed {{count}}',
        'batch_result_skipped': 'Skipped {{count}}', 'batch_result_total': 'Total {{count}}',
        'batch_result_col_name': 'Name', 'batch_result_col_provider': 'Provider',
        'batch_result_col_result': 'Result', 'batch_result_col_error': 'Error',
        'batch_result_badge_success': 'Success', 'batch_result_badge_failed': 'Failed',
        'batch_result_badge_skipped': 'Skipped',
    },
    'ru.json': {
        'batch_test': 'Проверить выбранные', 'batch_clear_errors': 'Очистить ошибки',
        'batch_test_title': 'Результаты проверки', 'batch_clear_errors_title': 'Результаты очистки ошибок',
        'batch_test_running': 'Проверка...', 'batch_clear_errors_running': 'Очистка ошибок...',
        'batch_no_auth_index': 'Отсутствует auth_index',
        'batch_unsupported_provider': 'Неподдерживаемый провайдер: {{provider}}',
        'batch_clear_errors_failed': 'Не удалось очистить ошибки',
        'batch_result_success': 'Успешно {{count}}', 'batch_result_failed': 'Ошибка {{count}}',
        'batch_result_skipped': 'Пропущено {{count}}', 'batch_result_total': 'Всего {{count}}',
        'batch_result_col_name': 'Имя', 'batch_result_col_provider': 'Провайдер',
        'batch_result_col_result': 'Результат', 'batch_result_col_error': 'Ошибка',
        'batch_result_badge_success': 'Успешно', 'batch_result_badge_failed': 'Ошибка',
        'batch_result_badge_skipped': 'Пропущено',
    },
    'zh-CN.json': {
        'batch_test': '测试选中', 'batch_clear_errors': '清除错误',
        'batch_test_title': '批量测试结果', 'batch_clear_errors_title': '清除错误结果',
        'batch_test_running': '正在测试...', 'batch_clear_errors_running': '正在清除错误...',
        'batch_no_auth_index': '缺少 auth_index',
        'batch_unsupported_provider': '不支持的提供方：{{provider}}',
        'batch_clear_errors_failed': '清除错误失败',
        'batch_result_success': '成功 {{count}} 项', 'batch_result_failed': '失败 {{count}} 项',
        'batch_result_skipped': '跳过 {{count}} 项', 'batch_result_total': '共 {{count}} 项',
        'batch_result_col_name': '名称', 'batch_result_col_provider': '提供方',
        'batch_result_col_result': '结果', 'batch_result_col_error': '错误',
        'batch_result_badge_success': '成功', 'batch_result_badge_failed': '失败',
        'batch_result_badge_skipped': '跳过',
    },
    'zh-TW.json': {
        'batch_test': '測試選中', 'batch_clear_errors': '清除錯誤',
        'batch_test_title': '批量測試結果', 'batch_clear_errors_title': '清除錯誤結果',
        'batch_test_running': '正在測試...', 'batch_clear_errors_running': '正在清除錯誤...',
        'batch_no_auth_index': '缺少 auth_index',
        'batch_unsupported_provider': '不支援的供應方：{{provider}}',
        'batch_clear_errors_failed': '清除錯誤失敗',
        'batch_result_success': '成功 {{count}} 項', 'batch_result_failed': '失敗 {{count}} 項',
        'batch_result_skipped': '跳過 {{count}} 項', 'batch_result_total': '共 {{count}} 項',
        'batch_result_col_name': '名稱', 'batch_result_col_provider': '供應方',
        'batch_result_col_result': '結果', 'batch_result_col_error': '錯誤',
        'batch_result_badge_success': '成功', 'batch_result_badge_failed': '失敗',
        'batch_result_badge_skipped': '跳過',
    },
}

QUOTA_DELETE_LOCALE_KEYS = {
    'en.json': {
        'select_all': 'Select All', 'select_credential': 'Select {{name}}', 'delete_one': 'Delete',
        'delete_one_title': 'Delete Credential',
        'delete_one_confirm': 'Delete credential "{{name}}"? This action cannot be undone.',
        'delete_selected': 'Delete Selected ({{count}})',
        'delete_selected_title': 'Delete Selected Credentials',
        'delete_selected_confirm': 'Delete {{count}} selected credential(s)? This action cannot be undone.',
        'delete_success': 'Credentials deleted', 'delete_partial': 'Succeeded {{success}}, failed {{failed}}',
    },
    'ru.json': {
        'select_all': 'Выбрать все', 'select_credential': 'Выбрать {{name}}', 'delete_one': 'Удалить',
        'delete_one_title': 'Удалить учётные данные',
        'delete_one_confirm': 'Удалить учётные данные «{{name}}»? Это действие нельзя отменить.',
        'delete_selected': 'Удалить выбранные ({{count}})',
        'delete_selected_title': 'Удалить выбранные учётные данные',
        'delete_selected_confirm': 'Удалить {{count}} выбранных учётных данных? Это действие нельзя отменить.',
        'delete_success': 'Учётные данные удалены', 'delete_partial': 'Успешно {{success}}, не удалось {{failed}}',
    },
    'zh-CN.json': {
        'select_all': '全选', 'select_credential': '选择 {{name}}', 'delete_one': '删除',
        'delete_one_title': '删除凭证', 'delete_one_confirm': '确定删除凭证 "{{name}}"？此操作不可撤销。',
        'delete_selected': '删除已选（{{count}}）', 'delete_selected_title': '删除所选凭证',
        'delete_selected_confirm': '确定删除选中的 {{count}} 个凭证？此操作不可撤销。',
        'delete_success': '凭证已删除', 'delete_partial': '成功 {{success}}，失败 {{failed}}',
    },
    'zh-TW.json': {
        'select_all': '全選', 'select_credential': '選擇 {{name}}', 'delete_one': '刪除',
        'delete_one_title': '刪除憑證', 'delete_one_confirm': '確定刪除憑證 "{{name}}"？此操作無法復原。',
        'delete_selected': '刪除已選（{{count}}）', 'delete_selected_title': '刪除所選憑證',
        'delete_selected_confirm': '確定刪除選取的 {{count}} 個憑證？此操作無法復原。',
        'delete_success': '憑證已刪除', 'delete_partial': '成功 {{success}}，失敗 {{failed}}',
    },
}

CLAUDE_MODEL_ID_CLOAK_LOCALE_KEYS = {
    'en.json': {
        'title': 'Anthropic Client Compatibility',
        'description': 'Control how non-Claude model IDs are exposed to Anthropic-compatible clients.',
        'label': 'Anthropic model ID compatibility mode',
        'hint': 'Only changes non-Claude IDs returned by /v1/models. Auto cloaks IDs for identified Claude Desktop clients.',
        'auto': 'Auto (Claude Desktop only)', 'always': 'Always cloak IDs', 'never': 'Keep original IDs',
    },
    'ru.json': {
        'title': 'Совместимость клиентов Anthropic',
        'description': 'Управление отображением идентификаторов моделей не-Claude.',
        'label': 'Режим совместимости идентификаторов моделей Anthropic',
        'hint': 'Изменяет только идентификаторы моделей не-Claude в ответе /v1/models.',
        'auto': 'Авто (только Claude Desktop)', 'always': 'Всегда маскировать', 'never': 'Сохранять исходные ID',
    },
    'zh-CN.json': {
        'title': 'Anthropic 客户端兼容性', 'description': '控制非 Claude 模型 ID 的展示方式。',
        'label': 'Anthropic 模型 ID 兼容模式',
        'hint': '仅影响 /v1/models 返回的非 Claude 模型 ID；自动模式仅对 Claude Desktop 伪装。',
        'auto': '自动（仅 Claude Desktop）', 'always': '始终伪装 ID', 'never': '保留原始 ID',
    },
    'zh-TW.json': {
        'title': 'Anthropic 用戶端相容性', 'description': '控制非 Claude 模型 ID 的顯示方式。',
        'label': 'Anthropic 模型 ID 相容模式',
        'hint': '僅影響 /v1/models 回傳的非 Claude 模型 ID；自動模式只對 Claude Desktop 偽裝。',
        'auto': '自動（僅 Claude Desktop）', 'always': '一律偽裝 ID', 'never': '保留原始 ID',
    },
}

PROXY_POOL_NAV_LOCALE_KEYS = {
    'en.json': {'label': 'Proxy Management', 'meta': 'Rotating upstream proxy gateway'},
    'ru.json': {'label': 'Управление прокси', 'meta': 'Шлюз ротации внешних прокси'},
    'zh-CN.json': {'label': '代理管理', 'meta': '多节点轮询与故障转移'},
    'zh-TW.json': {'label': '代理管理', 'meta': '多節點輪詢與故障轉移'},
}

OAUTH_MODEL_POLICY_NAV_LOCALE_KEYS = {
    'en.json': {'label': 'Model Policy', 'meta': 'Per-plan model availability rules'},
    'ru.json': {'label': 'Политика моделей', 'meta': 'Правила доступности моделей по тарифам'},
    'zh-CN.json': {'label': '模型策略', 'meta': '按账号套餐配置模型可用范围'},
    'zh-TW.json': {'label': '模型策略', 'meta': '依帳號套餐設定模型可用範圍'},
}

def load_overlay_replacement_manifest(path: Path) -> dict[str, set[str]]:
    payload = json.loads(path.read_text())
    if payload.get('schemaVersion') != 1 or not isinstance(payload.get('replacements'), list):
        raise RuntimeError(f'Invalid overlay replacement manifest: {path}')

    upstream_hashes: dict[str, set[str]] = {}
    for entry in payload['replacements']:
        if not isinstance(entry, dict):
            raise RuntimeError(f'Invalid overlay replacement entry: {entry!r}')
        relative_path = entry.get('path')
        upstream = entry.get('upstreamSha256')
        if (
            not isinstance(relative_path, str)
            or not relative_path
            or Path(relative_path).is_absolute()
            or '..' in Path(relative_path).parts
            or relative_path in upstream_hashes
            or not isinstance(upstream, list)
            or not upstream
            or not all(isinstance(item, str) and len(item) == 64 for item in upstream)
        ):
            raise RuntimeError(f'Invalid overlay replacement entry: {entry!r}')
        upstream_hashes[relative_path] = set(upstream)
    return upstream_hashes


OVERLAY_REPLACEMENT_HASHES = load_overlay_replacement_manifest(OVERLAY_REPLACEMENTS_FILE)


_writes = {}


def read(path: Path) -> str:
    if path in _writes:
        return _writes[path]
    return path.read_text()


def write(path: Path, text: str) -> None:
    _writes[path] = text


def flush_writes() -> None:
    for path, text in _writes.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
    _writes.clear()


def replace_once(path: Path, old: str, new: str) -> None:
    text = read(path)
    if new in text:
        return
    match_count = text.count(old)
    if match_count != 1:
        raise RuntimeError(f'Expected one pattern in {path}, found {match_count}: {old[:120]!r}')
    write(path, text.replace(old, new, 1))


def replace_once_if_present(path: Path, old: str, new: str) -> None:
    text = read(path)
    if new in text:
        return
    match_count = text.count(old)
    if match_count == 0:
        return
    if match_count != 1:
        raise RuntimeError(f'Expected at most one pattern in {path}, found {match_count}: {old[:120]!r}')
    write(path, text.replace(old, new, 1))


def replace_all(path: Path, old: str, new: str) -> None:
    text = read(path)
    if old not in text:
        return
    write(path, text.replace(old, new))


def ensure_cached_at_in_quota_success_state(path: Path, store_setter: str) -> None:
    text = read(path)
    marker = f"  storeSetter: '{store_setter}',"
    marker_start = text.find(marker)
    if marker_start == -1:
        raise RuntimeError(f'Pattern not found in {path}: {marker!r}')

    success_start = text.find('  buildSuccessState:', marker_start)
    error_start = text.find('  buildErrorState:', success_start)
    if success_start == -1 or error_start == -1:
        raise RuntimeError(f'Pattern not found in {path}: buildSuccessState block for {store_setter}')

    block = text[success_start:error_start]
    if 'cachedAt:' in block:
        return

    multiline_end = '\n  }),'
    if multiline_end in block:
        updated = block.replace(multiline_end, '\n    cachedAt: Date.now(),\n  }),', 1)
    else:
        inline_end = '}),'
        inline_end_start = block.rfind(inline_end)
        if inline_end_start == -1:
            raise RuntimeError(f'Pattern not found in {path}: buildSuccessState return end for {store_setter}')
        updated = f'{block[:inline_end_start].rstrip()}, cachedAt: Date.now() {block[inline_end_start:]}'

    write(path, f'{text[:success_start]}{updated}{text[error_start:]}')


def auth_files_page_path(target: Path) -> Path:
    for relative in ('src/features/authFiles/AuthFilesPage.tsx', 'src/pages/AuthFilesPage.tsx'):
        path = target / relative
        if path.is_file():
            return path
    raise RuntimeError(f'AuthFilesPage.tsx not found under {target}')


def auth_files_styles_path(target: Path) -> Path:
    for relative in (
        'src/features/authFiles/AuthFilesPage.module.scss',
        'src/pages/AuthFilesPage.module.scss',
    ):
        path = target / relative
        if path.is_file():
            return path
    raise RuntimeError(f'AuthFilesPage.module.scss not found under {target}')


def insert_once(path: Path, marker: str, insertion: str, present: str) -> None:
    text = read(path)
    if present in text:
        return
    match_count = text.count(marker)
    if match_count != 1:
        raise RuntimeError(f'Expected one marker in {path}, found {match_count}: {marker[:120]!r}')
    write(path, text.replace(marker, insertion, 1))


def validate_overlay_collisions(target: Path) -> None:
    for src in OVERLAY_DIR.rglob('*'):
        if src.is_dir():
            continue
        rel = src.relative_to(OVERLAY_DIR)
        dst = target / rel
        if not dst.is_file():
            continue
        source_digest = hashlib.sha256(src.read_bytes()).hexdigest()
        target_digest = hashlib.sha256(dst.read_bytes()).hexdigest()
        if target_digest == source_digest:
            continue
        allowed_hashes = OVERLAY_REPLACEMENT_HASHES.get(rel.as_posix())
        if allowed_hashes is None:
            raise RuntimeError(f'Unexpected overlay collision with upstream file: {dst}')
        if target_digest not in allowed_hashes:
            raise RuntimeError(f'Upstream overlay replacement changed: {dst} ({target_digest})')


def copy_overlay(target: Path) -> None:
    validate_overlay_collisions(target)
    for src in OVERLAY_DIR.rglob('*'):
        if src.is_dir():
            continue
        rel = src.relative_to(OVERLAY_DIR)
        dst = target / rel
        write(dst, src.read_text())


def patch_modal_focus_restore(target: Path) -> None:
    path = target / 'src/components/ui/Modal.tsx'
    replace_once(
        path,
        "  useEffect(() => {\n"
        "    if (open || isVisible) return;\n"
        "    previouslyFocusedRef.current?.focus();\n"
        "    previouslyFocusedRef.current = null;\n"
        "  }, [isVisible, open]);\n",
        "  useEffect(() => {\n"
        "    if (open || isVisible) return;\n"
        "    const previouslyFocused = previouslyFocusedRef.current;\n"
        "    if (previouslyFocused?.isConnected) {\n"
        "      previouslyFocused.focus({ preventScroll: true });\n"
        "    }\n"
        "    previouslyFocusedRef.current = null;\n"
        "  }, [isVisible, open]);\n",
    )


def patch_modal_scroll_lock(target: Path) -> None:
    path = target / 'src/components/ui/scrollLock.ts'
    text = read(path)
    replacement_marker = "  locksDocumentScroll: false,"
    if replacement_marker in text:
        return

    start_marker = 'const snapshot = {'
    end_marker = 'export const FOCUSABLE_SELECTOR'
    start = text.find(start_marker)
    end = text.find(end_marker, start)
    if start == -1 or end == -1:
        raise RuntimeError(f'Pattern not found in {path}: scroll lock implementation')

    current = text[start:end]
    upstream_markers = (
        "body.style.position = 'fixed';",
        "body.style.width = '100%';",
        'contentEl.scrollTo(',
        'window.scrollTo(',
    )
    previous_patch_markers = (
        "  bodyOverflow: '',",
        "  htmlOverflow: '',",
        "    body.style.overflow = 'hidden';",
        "    html.style.overflow = 'hidden';",
    )
    if not all(marker in current for marker in upstream_markers) and not all(
        marker in current for marker in previous_patch_markers
    ):
        raise RuntimeError(f'Pattern not found in {path}: supported scroll lock implementation')

    replacement = (
        "const snapshot = {\n"
        "  scrollX: 0,\n"
        "  scrollY: 0,\n"
        "  locksDocumentScroll: false,\n"
        "  bodyPosition: '',\n"
        "  bodyTop: '',\n"
        "  bodyLeft: '',\n"
        "  bodyRight: '',\n"
        "  bodyWidth: '',\n"
        "  bodyOverflow: '',\n"
        "  htmlOverflow: '',\n"
        "};\n\n"
        "export function lockScroll(): void {\n"
        "  if (typeof document === 'undefined') return;\n"
        "  if (activeLockCount === 0) {\n"
        "    const body = document.body;\n"
        "    const html = document.documentElement;\n\n"
        "    const scrollingElement = document.scrollingElement;\n"
        "    snapshot.scrollX = window.scrollX || window.pageXOffset || 0;\n"
        "    snapshot.scrollY = window.scrollY || window.pageYOffset || scrollingElement?.scrollTop || 0;\n"
        "    snapshot.locksDocumentScroll = Boolean(\n"
        "      scrollingElement && scrollingElement.scrollHeight > scrollingElement.clientHeight + 1\n"
        "    );\n"
        "    snapshot.bodyPosition = body.style.position;\n"
        "    snapshot.bodyTop = body.style.top;\n"
        "    snapshot.bodyLeft = body.style.left;\n"
        "    snapshot.bodyRight = body.style.right;\n"
        "    snapshot.bodyWidth = body.style.width;\n"
        "    snapshot.bodyOverflow = body.style.overflow;\n"
        "    snapshot.htmlOverflow = html.style.overflow;\n\n"
        "    body.classList.add(MODAL_LOCK_CLASS);\n"
        "    html.classList.add(MODAL_LOCK_CLASS);\n"
        "    if (snapshot.locksDocumentScroll) {\n"
        "      body.style.position = 'fixed';\n"
        "      body.style.top = `-${snapshot.scrollY}px`;\n"
        "      body.style.left = '0';\n"
        "      body.style.right = '0';\n"
        "      body.style.width = '100%';\n"
        "    }\n"
        "    body.style.overflow = 'hidden';\n"
        "    html.style.overflow = 'hidden';\n"
        "  }\n"
        "  activeLockCount += 1;\n"
        "}\n\n"
        "export function unlockScroll(): void {\n"
        "  if (typeof document === 'undefined') return;\n"
        "  activeLockCount = Math.max(0, activeLockCount - 1);\n"
        "  if (activeLockCount === 0) {\n"
        "    const body = document.body;\n"
        "    const html = document.documentElement;\n"
        "    const scrollX = snapshot.scrollX;\n"
        "    const scrollY = snapshot.scrollY;\n"
        "    const restoreDocumentScroll = snapshot.locksDocumentScroll;\n\n"
        "    body.classList.remove(MODAL_LOCK_CLASS);\n"
        "    html.classList.remove(MODAL_LOCK_CLASS);\n"
        "    body.style.position = snapshot.bodyPosition;\n"
        "    body.style.top = snapshot.bodyTop;\n"
        "    body.style.left = snapshot.bodyLeft;\n"
        "    body.style.right = snapshot.bodyRight;\n"
        "    body.style.width = snapshot.bodyWidth;\n"
        "    body.style.overflow = snapshot.bodyOverflow;\n"
        "    html.style.overflow = snapshot.htmlOverflow;\n"
        "\n"
        "    if (restoreDocumentScroll) {\n"
        "      window.scrollTo({ top: scrollY, left: scrollX, behavior: 'auto' });\n"
        "    }\n"
        "    snapshot.scrollX = 0;\n"
        "    snapshot.scrollY = 0;\n"
        "    snapshot.locksDocumentScroll = false;\n"
        "  }\n"
        "}\n\n"
    )
    write(path, f'{text[:start]}{replacement}{text[end:]}')


def patch_modal_content_scrollbar_layout(target: Path) -> None:
    path = target / 'src/styles/global.scss'
    text = read(path)
    content_lock = "body.modal-open .content {\n  overflow: hidden;\n}\n\n"
    if content_lock in text:
        write(path, text.replace(content_lock, '', 1))
        return
    if 'body.modal-open .content' in text:
        raise RuntimeError(f'Pattern not found in {path}: modal content scroll lock')


def patch_api_client_connection_isolation(target: Path) -> None:
    client = target / 'src/services/api/client.ts'
    insert_once(
        client,
        "  private managementKey: string = '';\n",
        "  private managementKey: string = '';\n"
        "  private connectionGeneration: number = 0;\n"
        "  private connectionAbortController = new AbortController();\n",
        "private connectionGeneration: number",
    )
    replace_once_if_present(
        client,
        "    this.apiBase = computeApiUrl(config.apiBase);\n"
        "    this.managementKey = config.managementKey;\n"
        "\n"
        "    if (config.timeout) {\n",
        "    const nextApiBase = computeApiUrl(config.apiBase);\n"
        "    const connectionChanged =\n"
        "      this.apiBase !== nextApiBase || this.managementKey !== config.managementKey;\n"
        "    this.apiBase = nextApiBase;\n"
        "    this.managementKey = config.managementKey;\n"
        "    if (connectionChanged) {\n"
        "      this.connectionAbortController.abort();\n"
        "      this.connectionAbortController = new AbortController();\n"
        "      this.connectionGeneration += 1;\n"
        "    }\n"
        "\n"
        "    if (config.timeout) {\n",
    )
    replace_once_if_present(
        client,
        "    if (connectionChanged) {\n"
        "      this.runtimeKind = 'unknown';\n"
        "    }\n",
        "    if (connectionChanged) {\n"
        "      this.connectionAbortController.abort();\n"
        "      this.connectionAbortController = new AbortController();\n"
        "      this.connectionGeneration += 1;\n"
        "      this.runtimeKind = 'unknown';\n"
        "    }\n",
    )
    if 'this.connectionGeneration += 1;' not in read(client):
        raise RuntimeError(f'Pattern not found in {client}: connection change handling')
    insert_once(
        client,
        "  /**\n   * 设置请求/响应拦截器\n   */\n",
        "  private combineRequestSignal(requestSignal: AxiosRequestConfig['signal']): AbortSignal {\n"
        "    const connectionSignal = this.connectionAbortController.signal;\n"
        "    if (!requestSignal) return connectionSignal;\n"
        "    const callerSignal = requestSignal as AbortSignal;\n"
        "    if (callerSignal === connectionSignal) return connectionSignal;\n"
        "    if (typeof AbortSignal.any === 'function') {\n"
        "      return AbortSignal.any([callerSignal, connectionSignal]);\n"
        "    }\n"
        "    const controller = new AbortController();\n"
        "    const abort = () => controller.abort();\n"
        "    if (callerSignal.aborted || connectionSignal.aborted) {\n"
        "      abort();\n"
        "    } else {\n"
        "      callerSignal.addEventListener('abort', abort, { once: true });\n"
        "      connectionSignal.addEventListener('abort', abort, { once: true });\n"
        "    }\n"
        "    return controller.signal;\n"
        "  }\n\n"
        "  private isStaleConnection(config: AxiosRequestConfig | undefined): boolean {\n"
        "    const generation = (config as AxiosRequestConfig & { __connectionGeneration?: number } | undefined)\n"
        "      ?.__connectionGeneration;\n"
        "    return typeof generation === 'number' && generation !== this.connectionGeneration;\n"
        "  }\n\n"
        "  private staleConnectionError(): Error {\n"
        "    return new axios.CanceledError('Connection changed while the request was in flight');\n"
        "  }\n\n"
        "  /**\n   * 设置请求/响应拦截器\n   */\n",
        'private isStaleConnection(config: AxiosRequestConfig | undefined)',
    )
    replace_once(
        client,
        "      (config) => {\n"
        "        // 设置 baseURL\n"
        "        config.baseURL = this.apiBase;\n",
        "      (config) => {\n"
        "        (config as AxiosRequestConfig & { __connectionGeneration?: number })\n"
        "          .__connectionGeneration = this.connectionGeneration;\n"
        "        config.signal = this.combineRequestSignal(config.signal);\n"
        "        // 设置 baseURL\n"
        "        config.baseURL = this.apiBase;\n",
    )
    replace_once(
        client,
        "      (response) => {\n"
        "        const headers = response.headers as Record<string, string | undefined>;\n",
        "      (response) => {\n"
        "        if (this.isStaleConnection(response.config)) {\n"
        "          throw this.staleConnectionError();\n"
        "        }\n"
        "        const headers = response.headers as Record<string, string | undefined>;\n",
    )
    replace_once(
        client,
        "        return response;\n"
        "      },\n"
        "      (error) => Promise.reject(this.handleError(error))\n"
        "    );\n",
        "        return response;\n"
        "      },\n"
        "      (error) => {\n"
        "        if (axios.isAxiosError(error) && this.isStaleConnection(error.config)) {\n"
        "          return Promise.reject(this.staleConnectionError());\n"
        "        }\n"
        "        return Promise.reject(this.handleError(error));\n"
        "      }\n"
        "    );\n",
    )

    auth_store = target / 'src/stores/useAuthStore.ts'
    replace_once(
        auth_store,
        "        useQuotaStore.getState().clearQuotaCache();\n"
        "        set({\n"
        "          isAuthenticated: false,\n",
        "        useQuotaStore.getState().clearQuotaCache();\n"
        "        apiClient.setConfig({ apiBase: '', managementKey: '' });\n"
        "        set({\n"
        "          isAuthenticated: false,\n",
    )


def patch_routes(target: Path) -> None:
    path = target / 'src/router/MainRoutes.tsx'
    replace_once(
        path,
        "import { QuotaPage } from '@/features/quota/QuotaPage';\n",
        "import { QuotaPage } from '@/features/quota/QuotaPage';\nimport { proRoutes } from '@/pro/registry';\n",
    )
    replace_once(
        path,
        "  { path: '/quota', element: <QuotaPage /> },\n",
        "  { path: '/quota', element: <QuotaPage /> },\n  ...proRoutes,\n",
    )


def patch_layout(target: Path) -> None:
    path = target / 'src/components/layout/MainLayout.tsx'
    insert_once(
        path,
        "import {\n  IconSidebar",
        "import { ProBootstrap } from '@/pro/ProBootstrap';\nimport { proNavigationGroups } from '@/pro/registry';\nimport {\n  IconSidebar",
        "proNavigationGroups",
    )
    insert_once(
        path,
        "    {\n      id: 'control',\n",
        "    ...proNavigationGroups,\n    {\n      id: 'control',\n",
        "...proNavigationGroups",
    )
    replace_once(
        path,
        "            <PageTransition\n",
        "            <ProBootstrap />\n            <PageTransition\n",
    )

def patch_icons(target: Path) -> None:
    path = target / 'src/components/ui/icons.tsx'
    text = read(path)

    if "baseSvgProps" in text:
        svg_props = "baseSvgProps"
    elif "sidebarSvgProps" in text:
        svg_props = "sidebarSvgProps"
    else:
        raise RuntimeError(f'Pattern not found in {path}: svg props constant')

    monitor_icon = (
        "export function IconSidebarMonitor({ size = 20, ...props }: IconProps) {\n"
        "  return (\n"
        f"    <svg {{...{svg_props}}} width={{size}} height={{size}} {{...props}}>\n"
        "      <path d=\"M3 12h3l2.2-4.5 4.2 9 2.4-5h6.2\" />\n"
        "      <path d=\"M4 19h16\" />\n"
        "      <path d=\"M4 5h16\" fill=\"currentColor\" fillOpacity=\"0.08\" />\n"
        "    </svg>\n"
        "  );\n"
        "}\n\n"
    )
    account_inspection_icon = (
        "export function IconSidebarAccountInspection({ size = 20, ...props }: IconProps) {\n"
        "  return (\n"
        f"    <svg {{...{svg_props}}} width={{size}} height={{size}} {{...props}}>\n"
        "      <rect x=\"5\" y=\"3\" width=\"11\" height=\"16\" rx=\"2\" />\n"
        "      <path d=\"M9 7h3\" />\n"
        "      <path d=\"m8.5 11 1.4 1.4 2.6-2.8\" />\n"
        "      <circle cx=\"16.5\" cy=\"16.5\" r=\"3\" />\n"
        "      <path d=\"m19 19 2 2\" />\n"
        "      <path d=\"M8 3.5h5\" fill=\"currentColor\" fillOpacity=\"0.08\" />\n"
        "    </svg>\n"
        "  );\n"
        "}\n\n"
    )
    routing_icon = (
        "export function IconSidebarRouting({ size = 20, ...props }: IconProps) {\n"
        "  return (\n"
        f"    <svg {{...{svg_props}}} width={{size}} height={{size}} {{...props}}>\n"
        "      <circle cx=\"6\" cy=\"6\" r=\"2\" />\n"
        "      <circle cx=\"18\" cy=\"6\" r=\"2\" />\n"
        "      <circle cx=\"12\" cy=\"18\" r=\"2\" />\n"
        "      <path d=\"M8 6h8\" />\n"
        "      <path d=\"m7.5 7.5 3.2 7.2\" />\n"
        "      <path d=\"m16.5 7.5-3.2 7.2\" />\n"
        "    </svg>\n"
        "  );\n"
        "}\n\n"
    )
    proxy_pool_icon = (
        "export function IconSidebarProxyPool({ size = 20, ...props }: IconProps) {\n"
        "  return (\n"
        f"    <svg {{...{svg_props}}} width={{size}} height={{size}} {{...props}}>\n"
        "      <circle cx=\"6\" cy=\"7\" r=\"2.5\" />\n"
        "      <circle cx=\"18\" cy=\"7\" r=\"2.5\" />\n"
        "      <circle cx=\"12\" cy=\"18\" r=\"2.5\" />\n"
        "      <path d=\"M8.5 7h7\" />\n"
        "      <path d=\"m7.4 9 3.2 6.6\" />\n"
        "      <path d=\"m16.6 9-3.2 6.6\" />\n"
        "      <path d=\"m12.5 4.5 2 2.5-2 2.5\" />\n"
        "    </svg>\n"
        "  );\n"
        "}\n\n"
    )
    icons_to_insert = ""
    if "export function IconSidebarMonitor" not in text:
        icons_to_insert += monitor_icon
    if "export function IconSidebarAccountInspection" not in text:
        icons_to_insert += account_inspection_icon
    if "export function IconSidebarRouting" not in text:
        icons_to_insert += routing_icon
    if "export function IconSidebarProxyPool" not in text:
        icons_to_insert += proxy_pool_icon
    if not icons_to_insert:
        return
    for marker in (
        "export function IconSidebarLogs({ size = 20, ...props }: IconProps) {\n",
        "export const IconSidebarLogs = ",
        "export function IconSidebarSystem({ size = 20, ...props }: IconProps) {\n",
    ):
        if marker in text:
            write(path, text.replace(marker, icons_to_insert + marker, 1))
            return

    write(path, text.rstrip() + "\n\n" + icons_to_insert)


def patch_quota_store(target: Path) -> None:
    path = target / 'src/stores/useQuotaStore.ts'
    replace_once(
        path,
        "  CodexQuotaState,\n  KimiQuotaState,",
        "  CodexQuotaState,\n  GeminiCliQuotaState,\n  KimiQuotaState,",
    )
    replace_once(
        path,
        "  codexQuota: Record<string, CodexQuotaState>;\n  kimiQuota: Record<string, KimiQuotaState>;",
        "  codexQuota: Record<string, CodexQuotaState>;\n  geminiCliQuota: Record<string, GeminiCliQuotaState>;\n  kimiQuota: Record<string, KimiQuotaState>;",
    )
    replace_once(
        path,
        "  setCodexQuota: (updater: QuotaUpdater<Record<string, CodexQuotaState>>) => void;\n  setKimiQuota: (updater: QuotaUpdater<Record<string, KimiQuotaState>>) => void;",
        "  setCodexQuota: (updater: QuotaUpdater<Record<string, CodexQuotaState>>) => void;\n  setGeminiCliQuota: (updater: QuotaUpdater<Record<string, GeminiCliQuotaState>>) => void;\n  setKimiQuota: (updater: QuotaUpdater<Record<string, KimiQuotaState>>) => void;",
    )
    replace_once(
        path,
        "  codexQuota: {},\n  kimiQuota: {},",
        "  codexQuota: {},\n  geminiCliQuota: {},\n  kimiQuota: {},",
    )
    replace_once(
        path,
        "  setCodexQuota: (updater) =>\n    set((state) => ({\n      codexQuota: resolveUpdater(updater, state.codexQuota),\n    })),\n  setKimiQuota: (updater) =>",
        "  setCodexQuota: (updater) =>\n    set((state) => ({\n      codexQuota: resolveUpdater(updater, state.codexQuota),\n    })),\n  setGeminiCliQuota: (updater) =>\n    set((state) => ({\n      geminiCliQuota: resolveUpdater(updater, state.geminiCliQuota),\n    })),\n  setKimiQuota: (updater) =>",
    )
    replace_once(
        path,
        "      codexQuota: {},\n      kimiQuota: {},",
        "      codexQuota: {},\n      geminiCliQuota: {},\n      kimiQuota: {},",
    )


def patch_quota_constants(target: Path) -> None:
    path = target / 'src/utils/quota/constants.ts'
    insert_once(
        path,
        "  aistudio: {\n",
        "  'gemini-cli': {\n    light: { bg: '#e0e8ff', text: '#1e4fa3' },\n    dark: { bg: '#1c3f73', text: '#a8c7ff' },\n  },\n  aistudio: {\n",
        "'gemini-cli':",
    )


def patch_antigravity_quota_builders(target: Path) -> None:
    path = target / 'src/utils/quota/builders.ts'
    insert_once(
        path,
        "\nfunction getAntigravityWindowOrder(bucket: AntigravityQuotaBucket): number {\n",
        "\nfunction getCanonicalAntigravityGroupId(label: string, description?: string): string {\n  const normalizedLabel = toStableId(label, '');\n  const normalizedDescription = description ? toStableId(description, '') : '';\n  const combined = `${normalizedLabel}-${normalizedDescription}`;\n  if (combined.includes('claude') && (combined.includes('gpt') || combined.includes('gpt-oss') || combined.includes('openai'))) {\n    return 'claude-gpt';\n  }\n  if (combined.includes('gemini')) {\n    return 'gemini';\n  }\n  return normalizedLabel;\n}\n\nfunction getAntigravityWindowOrder(bucket: AntigravityQuotaBucket): number {\n",
        "getCanonicalAntigravityGroupId",
    )
    replace_once(
        path,
        "      const groupId = toStableId(label, `quota-group-${groupIndex + 1}`);\n      const buckets = Array.isArray(group.buckets) ? group.buckets : [];\n",
        "      const description = normalizeStringValue(group.description) ?? undefined;\n      const groupId = getCanonicalAntigravityGroupId(label, description) || `quota-group-${groupIndex + 1}`;\n      const buckets = Array.isArray(group.buckets) ? group.buckets : [];\n",
    )
    replace_once(
        path,
        "        description: normalizeStringValue(group.description) ?? undefined,\n",
        "        description,\n",
    )
    replace_once(
        path,
        "    productUsage: primary.productUsage.length > 0 ? primary.productUsage : fallback.productUsage,\n",
        "    productUsage: Array.isArray(primary.productUsage) && primary.productUsage.length > 0\n      ? primary.productUsage\n      : Array.isArray(fallback.productUsage)\n        ? fallback.productUsage\n        : [],\n",
    )


def patch_account_inspection_page(target: Path) -> None:
    path = target / 'src/pro/modules/inspection/AccountInspectionPage.tsx'
    replace_once_if_present(
        path,
        "  const used = normalizeNumberValue(quota.billing.usedPercent ?? quota.billing.used_percent);\n"
        "  return used !== null && used >= usedPercentThreshold;\n",
        "  const used =\n"
        "    normalizeNumberValue(quota.billing.usagePercent ?? quota.billing.usage_percent)\n"
        "    ?? normalizeNumberValue(quota.billing.usedPercent ?? quota.billing.used_percent)\n"
        "    ?? maxAntigravityGroupUsedPercent(Array.isArray(quota.billing.productUsage) ? quota.billing.productUsage : []);\n"
        "  return used !== null && used >= usedPercentThreshold;\n",
    )


def patch_auth_files_runtime_state(target: Path) -> None:
    type_path = target / 'src/types/authFile.ts'
    card_path = target / 'src/features/authFiles/components/AuthFileCard.tsx'
    page_path = auth_files_page_path(target)

    insert_once(
        type_path,
        "  success?: unknown;\n",
        "  selected?: unknown;\n  success?: unknown;\n",
        "selected?: unknown;",
    )
    card_text = read(card_path)
    legacy_stats = (
        "  const fileStats = {\n    success: normalizeUsageTotal(file.success),\n"
        "    failure: normalizeUsageTotal(file.failed),\n  };\n"
    )
    if legacy_stats in card_text:
        write(
            card_path,
            card_text.replace(
                legacy_stats,
                "  const fileStats = {\n    selected: normalizeUsageTotal(file.selected),\n"
                "    success: normalizeUsageTotal(file.success),\n"
                "    failure: normalizeUsageTotal(file.failed),\n  };\n",
                1,
            ),
        )
        insert_once(
            card_path,
            "            <div className={`${styles.cardStats} ${compact ? styles.cardStatsCompact : ''}`}>\n",
            "            <div className={`${styles.cardStats} ${compact ? styles.cardStatsCompact : ''}`}>\n"
            "              <div className={styles.statPill}>\n"
            "                <span className={styles.statLabel}>{t('auth_files.selected_count')}</span>\n"
            "                <span className={styles.statValue}>{fileStats.selected}</span>\n"
            "              </div>\n",
            "t('auth_files.selected_count')",
        )
    elif 'const selectedCount =' not in card_text and 'const successCount = file.successCount ?? 0;' in card_text:
        write(
            card_path,
            card_text.replace(
                '  const successCount = file.successCount ?? 0;\n',
                "  const selectedCount = Math.max(0, Number(file.selected) || 0);\n"
                "  const successCount = file.successCount ?? 0;\n",
                1,
            ),
        )
        insert_once(
            card_path,
            "          <span className={styles.healthCounts}>\n",
            "          <span className={styles.healthCounts}>\n"
            "            <span className={styles.countOk} title={t('auth_files.selected_count')}>\n"
            "              {t('auth_files.selected_count')} {selectedCount}\n"
            "            </span>\n",
            "{t('auth_files.selected_count')} {selectedCount}",
        )
    elif "t('auth_files.selected_count')" not in card_text:
        raise RuntimeError(f'Pattern not found in {card_path}: auth runtime counters')

    insert_once(
        page_path,
        "import { useHeaderRefresh } from '@/hooks/useHeaderRefresh';\n",
        "import { useHeaderRefresh } from '@/hooks/useHeaderRefresh';\n"
        "import { quotaPersistenceMiddleware } from '@/pro/modules/quota';\n",
        "quotaPersistenceMiddleware } from '@/pro/modules/quota'",
    )
    text = read(page_path)
    if 'quotaPersistenceMiddleware.ensureFresh()' not in text:
        refresh_variants = (
            "    await Promise.all([loadFiles(), loadExcluded(), loadModelAlias()]);\n",
            "    await Promise.all([loadFiles({ background: true }), loadExcluded(), loadModelAlias()]);\n",
        )
        for refresh in refresh_variants:
            if refresh in text:
                replacement = refresh.replace(']);\n', ', quotaPersistenceMiddleware.ensureFresh()]);\n')
                write(page_path, text.replace(refresh, replacement, 1))
                break
        else:
            raise RuntimeError(f'Pattern not found in {page_path}: header refresh')


def patch_account_usage_feature(target: Path) -> None:
    icons_path = target / 'src/components/ui/icons.tsx'
    card_path = target / 'src/features/authFiles/components/AuthFileCard.tsx'
    page_path = auth_files_page_path(target)
    styles_path = auth_files_styles_path(target)

    insert_once(
        icons_path,
        'export function IconModelCluster({ size = 20, ...props }: IconProps) {\n',
        '''export function IconChartColumnIncreasing({ size = 20, ...props }: IconProps) {
  return (
    <svg {...baseSvgProps} width={size} height={size} {...props}>
      <path d="M3 3v18h18" />
      <path d="M7 16v1" />
      <path d="M11 12v5" />
      <path d="M15 8v9" />
      <path d="M19 4v13" />
    </svg>
  );
}

export function IconModelCluster({ size = 20, ...props }: IconProps) {
''',
        'export function IconChartColumnIncreasing',
    )

    replace_once(
        card_path,
        '  IconDownload,\n  IconInfo,\n',
        '  IconChartColumnIncreasing,\n  IconDownload,\n  IconInfo,\n',
    )
    replace_once(
        card_path,
        '  onShowModels: (file: AuthFileItem) => void;\n',
        '  onShowModels: (file: AuthFileItem) => void;\n  onShowUsage: (file: AuthFileItem) => void;\n',
    )
    replace_once(
        card_path,
        '    onShowModels,\n    onDownload,\n',
        '    onShowModels,\n    onShowUsage,\n    onDownload,\n',
    )
    card_text = read(card_path)
    legacy_usage_marker = '            </div>\n          </div>\n\n          <div className={`${styles.cardMeta}'
    if "onClick={() => onShowUsage(file)}" not in card_text and legacy_usage_marker in card_text:
        write(card_path, card_text.replace(legacy_usage_marker, '''            </div>
            {authIndexKey && (
              <Button
                variant="secondary"
                size="sm"
                onClick={() => onShowUsage(file)}
                className={styles.usageCornerButton}
                title={t('account_usage.card_action')}
                aria-label={t('account_usage.card_action')}
                disabled={disableControls}
              >
                <IconChartColumnIncreasing className={styles.actionIcon} size={17} />
              </Button>
            )}
          </div>

          <div className={`${styles.cardMeta}''',
        1))
        insert_once(
            styles_path,
            '.modelsActionButton:global(.btn.btn-sm) {\n',
            '''.usageCornerButton:global(.btn.btn-sm) {
  flex: 0 0 auto;
  align-self: flex-start;
  width: 34px;
  height: 34px;
  min-width: 34px;
  padding: 0;
  background: color-mix(in srgb, #0f766e 9%, var(--bg-secondary));
  border-color: color-mix(in srgb, #0f766e 22%, var(--border-color));
  color: color-mix(in srgb, #0f766e 78%, var(--text-primary));
}

.usageCornerButton:global(.btn.btn-sm):hover {
  background: color-mix(in srgb, #0f766e 14%, var(--bg-secondary));
  border-color: color-mix(in srgb, #0f766e 38%, var(--border-color));
}

.usageCornerButton:global(.btn.btn-sm) > span {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.fileCardCompact .usageCornerButton:global(.btn.btn-sm) {
  width: 30px;
  height: 30px;
  min-width: 30px;
}

.modelsActionButton:global(.btn.btn-sm) {
''',
            '.usageCornerButton:global(.btn.btn-sm)',
        )
    elif "onClick={() => onShowUsage(file)}" not in card_text:
        actions_marker = '        <div className={styles.actionsMain}>\n'
        if actions_marker not in card_text:
            raise RuntimeError(f'Pattern not found in {card_path}: auth file actions')
        write(
            card_path,
            card_text.replace(
                actions_marker,
                actions_marker
                + "          {authIndexKey && (\n"
                + "            <Button variant=\"secondary\" size=\"sm\" onClick={() => onShowUsage(file)}\n"
                + "              title={t('account_usage.card_action')} disabled={disableControls}>\n"
                + "              <IconChartColumnIncreasing size={14} />\n"
                + "              {t('account_usage.card_action')}\n"
                + "            </Button>\n"
                + "          )}\n",
                1,
            ),
        )

    insert_once(
        page_path,
        "import { AuthFileModelsModal } from '@/features/authFiles/components/AuthFileModelsModal';\n",
        "import { AuthFileModelsModal } from '@/features/authFiles/components/AuthFileModelsModal';\n"
        "import { AccountUsageModal } from '@/pro/modules/monitoring';\n",
        "AccountUsageModal } from '@/pro/modules/monitoring'",
    )
    insert_once(
        page_path,
        "import { useAuthStore, useNotificationStore, useThemeStore, useQuotaStore } from '@/stores';\n",
        "import { useAuthStore, useNotificationStore, useThemeStore, useQuotaStore } from '@/stores';\n"
        "import type { AuthFileItem } from '@/types';\n",
        "import type { AuthFileItem } from '@/types';",
    )
    page_text = read(page_path)
    if 'const [accountUsageFile, setAccountUsageFile]' not in page_text:
        state_markers = (
            "  const [displaySettingsOpen, setDisplaySettingsOpen] = useState(false);\n",
            "  const [sortMode, setSortMode] = useState<AuthFilesSortMode>('default');\n",
        )
        for marker in state_markers:
            if marker in page_text:
                write(
                    page_path,
                    page_text.replace(
                        marker,
                        marker
                        + "  const [accountUsageFile, setAccountUsageFile] = useState<AuthFileItem | null>(null);\n",
                        1,
                    ),
                )
                break
        else:
            raise RuntimeError(f'Pattern not found in {page_path}: account usage state')
    page_text = read(page_path)
    if 'onShowUsage={setAccountUsageFile}' not in page_text:
        for indent in ('                  ', '                '):
            marker = f'{indent}onShowModels={{showModels}}\n{indent}onDownload={{handleDownload}}\n'
            if marker in page_text:
                write(
                    page_path,
                    page_text.replace(
                        marker,
                        f'{indent}onShowModels={{showModels}}\n'
                        f'{indent}onShowUsage={{setAccountUsageFile}}\n'
                        f'{indent}onDownload={{handleDownload}}\n',
                        1,
                    ),
                )
                break
        else:
            raise RuntimeError(f'Pattern not found in {page_path}: auth card usage callback')
    insert_once(
        page_path,
        "      <AuthFileModelsModal\n",
        "      <AccountUsageModal file={accountUsageFile} onClose={() => setAccountUsageFile(null)} />\n\n"
        "      <AuthFileModelsModal\n",
        '<AccountUsageModal file={accountUsageFile}',
    )

    insert_once(
        page_path,
        "  const existingTypes = useMemo(() => {\n",
        "  useEffect(() => {\n"
        "    if (!isCurrentLayer) return;\n"
        "    void quotaPersistenceMiddleware.ensureFresh();\n"
        "  }, [files, isCurrentLayer]);\n\n"
        "  const existingTypes = useMemo(() => {\n",
        "}, [files, isCurrentLayer]);",
    )


def patch_runtime_detection(target: Path) -> None:
    version_path = target / 'src/services/api/version.ts'
    if "apiClient.get('/nodes')" not in read(version_path):
        return

    client_path = target / 'src/services/api/client.ts'
    insert_once(
        client_path,
        "  private managementKey: string = '';\n",
        "  private managementKey: string = '';\n  private runtimeKind: ServerRuntimeKind = 'unknown';\n",
        "private runtimeKind: ServerRuntimeKind",
    )
    replace_once(
        client_path,
        "    this.apiBase = computeApiUrl(config.apiBase);\n"
        "    this.managementKey = config.managementKey;\n"
        "\n"
        "    if (config.timeout) {\n",
        "    const nextApiBase = computeApiUrl(config.apiBase);\n"
        "    const connectionChanged =\n"
        "      this.apiBase !== nextApiBase || this.managementKey !== config.managementKey;\n"
        "    this.apiBase = nextApiBase;\n"
        "    this.managementKey = config.managementKey;\n"
        "    if (connectionChanged) {\n"
        "      this.runtimeKind = 'unknown';\n"
        "    }\n"
        "\n"
        "    if (config.timeout) {\n",
    )
    insert_once(
        client_path,
        "  private readHeader(headers: Record<string, unknown> | undefined, keys: string[]): string | null {\n",
        "  getRuntimeKind(): ServerRuntimeKind {\n"
        "    return this.runtimeKind;\n"
        "  }\n"
        "\n"
        "  private readHeader(headers: Record<string, unknown> | undefined, keys: string[]): string | null {\n",
        "getRuntimeKind(): ServerRuntimeKind",
    )
    replace_once(
        client_path,
        "        const runtimeKind: ServerRuntimeKind | null =\n"
        "          homeVersion || homeBuildDate ? 'home' : cpaVersion || cpaBuildDate ? 'cpa' : null;\n"
        "\n"
        "        // 触发版本更新事件（后续通过 store 处理）\n",
        "        const runtimeKind: ServerRuntimeKind | null =\n"
        "          homeVersion || homeBuildDate ? 'home' : cpaVersion || cpaBuildDate ? 'cpa' : null;\n"
        "        if (runtimeKind) {\n"
        "          this.runtimeKind = runtimeKind;\n"
        "        }\n"
        "\n"
        "        // 触发版本更新事件（后续通过 store 处理）\n",
    )

    replace_all(
        version_path,
        "import { isRecord } from '@/utils/helpers';\n",
        "",
    )
    replace_once(
        version_path,
        "  async detectRuntimeKind(): Promise<ServerRuntimeKind> {\n"
        "    try {\n"
        "      const data = await apiClient.get('/nodes');\n"
        "      return isRecord(data) && Array.isArray(data.nodes) ? 'home' : 'unknown';\n"
        "    } catch (error: unknown) {\n"
        "      const status = isRecord(error) ? error.status : undefined;\n"
        "      if (status === 404 || status === 405) {\n"
        "        return 'cpa';\n"
        "      }\n"
        "      return 'unknown';\n"
        "    }\n"
        "  },\n",
        "  async detectRuntimeKind(): Promise<ServerRuntimeKind> {\n"
        "    const runtimeKind = apiClient.getRuntimeKind();\n"
        "    return runtimeKind === 'unknown' ? 'cpa' : runtimeKind;\n"
        "  },\n",
    )


def patch_management_update_check(target: Path) -> None:
    version_path = target / 'src/services/api/version.ts'
    insert_once(
        version_path,
        "  checkLatest: () => apiClient.get<Record<string, unknown>>('/latest-version'),\n",
        "  checkLatest: () => apiClient.get<Record<string, unknown>>('/latest-version'),\n"
        "  checkManagementPanelUpdate: () =>\n"
        "    apiClient.post<{ status: string; updated: boolean; sha256: string }>(\n"
        "      '/management-panel/check-update'\n"
        "    ),\n",
        'checkManagementPanelUpdate:',
    )

    page_path = target / 'src/pages/SystemPage.tsx'
    insert_once(
        page_path,
        "  const [checkingVersion, setCheckingVersion] = useState(false);\n",
        "  const [checkingVersion, setCheckingVersion] = useState(false);\n"
        "  const [checkingManagementUpdate, setCheckingManagementUpdate] = useState(false);\n",
        'const [checkingManagementUpdate, setCheckingManagementUpdate]',
    )
    insert_once(
        page_path,
        "  useEffect(() => {\n    fetchConfig().catch(() => {\n",
        "  const handleManagementUpdateCheck = useCallback(async () => {\n"
        "    setCheckingManagementUpdate(true);\n"
        "    try {\n"
        "      const result = await versionApi.checkManagementPanelUpdate();\n"
        "      if (result.updated) {\n"
        "        showNotification(t('system_info.management_check_update_updated'), 'success');\n"
        "        window.setTimeout(() => {\n"
        "          const nextUrl = new URL(window.location.href);\n"
        "          nextUrl.searchParams.set('_management_updated', Date.now().toString());\n"
        "          window.location.replace(nextUrl.toString());\n"
        "        }, 500);\n"
        "      } else {\n"
        "        showNotification(t('system_info.management_check_update_unchanged'), 'success');\n"
        "      }\n"
        "    } catch (error: unknown) {\n"
        "      const message =\n"
        "        error instanceof Error ? error.message : typeof error === 'string' ? error : '';\n"
        "      showNotification(\n"
        "        `${t('system_info.management_check_update_error')}${message ? `: ${message}` : ''}`,\n"
        "        'error'\n"
        "      );\n"
        "    } finally {\n"
        "      setCheckingManagementUpdate(false);\n"
        "    }\n"
        "  }, [showNotification, t]);\n\n"
        "  useEffect(() => {\n    fetchConfig().catch(() => {\n",
        'const handleManagementUpdateCheck = useCallback',
    )
    replace_once(
        page_path,
        "            <button\n"
        "              type=\"button\"\n"
        "              className={`${styles.infoTile} ${styles.tapTile}`}\n"
        "              onClick={handleInfoVersionTap}\n"
        "            >\n"
        "              <div className={styles.tileHeader}>\n"
        "                <div className={styles.tileLabel}>{t('footer.version')}</div>\n"
        "              </div>\n"
        "              <div className={styles.tileValue}>{appVersion}</div>\n"
        "            </button>\n",
        "            <div\n"
        "              className={`${styles.infoTile} ${styles.tapTile}`}\n"
        "              onClick={handleInfoVersionTap}\n"
        "            >\n"
        "              <div className={styles.tileHeader}>\n"
        "                <div className={styles.tileLabel}>{t('footer.version')}</div>\n"
        "                <Button\n"
        "                  type=\"button\"\n"
        "                  variant=\"ghost\"\n"
        "                  size=\"sm\"\n"
        "                  className={styles.tileAction}\n"
        "                  onClick={(event) => {\n"
        "                    event.stopPropagation();\n"
        "                    void handleManagementUpdateCheck();\n"
        "                  }}\n"
        "                  loading={checkingManagementUpdate}\n"
        "                  title={t('system_info.management_check_update_button')}\n"
        "                  aria-label={t('system_info.management_check_update_button')}\n"
        "                >\n"
        "                  {t('system_info.management_check_update_button')}\n"
        "                </Button>\n"
        "              </div>\n"
        "              <div className={styles.tileValue}>{appVersion}</div>\n"
        "            </div>\n",
    )


def patch_supporting_api_and_types(target: Path) -> None:
    config_path = target / 'src/types/config.ts'
    replace_once(
        config_path,
        "export interface Config {\n  debug?: boolean;\n",
        "export interface AuthPoolCleanConfig {\n  baseUrl?: string;\n  token?: string;\n  targetType?: string;\n  workers?: number;\n  deleteWorkers?: number;\n  timeout?: number;\n  retries?: number;\n  usedPercentThreshold?: number;\n  sampleSize?: number;\n}\n\nexport interface Config {\n  debug?: boolean;\n",
    )
    replace_once(
        config_path,
        "  quotaExceeded?: QuotaExceededConfig;\n  requestLog?: boolean;\n",
        "  quotaExceeded?: QuotaExceededConfig;\n  clean?: AuthPoolCleanConfig;\n  usageStatisticsEnabled?: boolean;\n  requestLog?: boolean;\n",
    )
    replace_once(
        config_path,
        "  | 'quota-exceeded'\n  | 'request-log'\n",
        "  | 'quota-exceeded'\n  | 'usage-statistics-enabled'\n  | 'request-log'\n",
    )

    auth_file_type_path = target / 'src/types/authFile.ts'
    replace_once(
        auth_file_type_path,
        "export interface AuthFileItem {\n  name: string;\n",
        "export interface AuthFileLastError {\n  code?: string;\n  message?: string;\n  retryable?: boolean;\n  http_status?: number;\n  httpStatus?: number;\n}\n\nexport interface AuthFileItem {\n  name: string;\n",
    )
    replace_once(
        auth_file_type_path,
        "  statusMessage?: string;\n  lastRefresh?: string | number;\n",
        "  statusMessage?: string;\n  lastError?: AuthFileLastError | null;\n  'last_error'?: AuthFileLastError | null;\n  lastRefresh?: string | number;\n",
    )

    auth_file_constants_path = target / 'src/features/authFiles/constants.ts'
    replace_once(
        auth_file_constants_path,
        "export const getAuthFileStatusMessage = (file: AuthFileItem): string => {\n  const raw = file['status_message'] ?? file.statusMessage;\n  if (typeof raw === 'string') return raw.trim();\n  if (raw == null) return '';\n  return String(raw).trim();\n};\n",
        "const normalizeAuthFileMessageValue = (value: unknown): string => {\n  if (typeof value === 'string') return value.trim();\n  if (value == null) return '';\n  return String(value).trim();\n};\n\nconst getAuthFileLastErrorMessage = (file: AuthFileItem): string => {\n  const raw = file['last_error'] ?? file.lastError;\n  if (!raw || typeof raw !== 'object') return '';\n  return normalizeAuthFileMessageValue((raw as { message?: unknown }).message);\n};\n\nexport const getAuthFileStatusMessage = (file: AuthFileItem): string => {\n  const statusMessage = normalizeAuthFileMessageValue(file['status_message'] ?? file.statusMessage);\n  return statusMessage || getAuthFileLastErrorMessage(file);\n};\n",
    )

    auth_files_path = target / 'src/services/api/authFiles.ts'
    auth_files_normalizer = (
        'normalizeAuthFilesResponse'
        if 'normalizeAuthFilesResponse' in read(auth_files_path)
        else 'dedupeAuthFilesResponse'
    )
    replace_once(
        auth_files_path,
        "type AuthFileStatusResponse = { status: string; disabled: boolean };\n",
        "type AuthFileStatusResponse = { status: string; disabled: boolean };\ntype AuthFilePatchPayload = { name: string; disabled?: boolean; [key: string]: unknown };\n",
    )
    insert_once(
        auth_files_path,
        "export const authFilesApi = {\n",
        "const AUTH_FILES_LIST_CACHE_TTL_MS = 2000;\nlet authFilesListCache: { expiresAt: number; response: AuthFilesResponse } | null = null;\nlet authFilesListRequest: Promise<AuthFilesResponse> | null = null;\nlet authFilesListVersion = 0;\n\nconst cloneAuthFilesResponse = (response: AuthFilesResponse): AuthFilesResponse => ({\n  ...response,\n  files: Array.isArray(response.files) ? [...response.files] : [],\n});\n\nconst invalidateAuthFilesListCache = () => {\n  authFilesListVersion += 1;\n  authFilesListCache = null;\n  authFilesListRequest = null;\n};\n\nconst fetchAuthFilesList = async (): Promise<AuthFilesResponse> => {\n  const now = Date.now();\n  if (authFilesListCache && authFilesListCache.expiresAt > now) {\n    return cloneAuthFilesResponse(authFilesListCache.response);\n  }\n  if (!authFilesListRequest) {\n    const requestVersion = authFilesListVersion;\n    authFilesListRequest = apiClient.get<AuthFilesResponse>('/auth-files')\n      .then(dedupeAuthFilesResponse)\n      .then((response) => {\n        if (requestVersion === authFilesListVersion) {\n          authFilesListCache = {\n            expiresAt: Date.now() + AUTH_FILES_LIST_CACHE_TTL_MS,\n            response: cloneAuthFilesResponse(response),\n          };\n        }\n        return response;\n      })\n      .finally(() => {\n        if (requestVersion === authFilesListVersion) {\n          authFilesListRequest = null;\n        }\n      });\n  }\n  return cloneAuthFilesResponse(await authFilesListRequest);\n};\n\nexport const authFilesApi = {\n",
        "AUTH_FILES_LIST_CACHE_TTL_MS",
    )
    replace_once_if_present(
        auth_files_path,
        '      .then(dedupeAuthFilesResponse)\n',
        f'      .then({auth_files_normalizer})\n',
    )
    text = read(auth_files_path)
    list_variants = (
        "  list: async () => dedupeAuthFilesResponse(await apiClient.get<AuthFilesResponse>('/auth-files')),\n\n"
        "  setStatus: (name: string, disabled: boolean) =>\n"
        "    apiClient.patch<AuthFileStatusResponse>('/auth-files/status', { name, disabled }),\n\n",
        "  list: async () =>\n"
        "    normalizeAuthFilesResponse(await apiClient.get<AuthFilesResponse>('/auth-files')),\n\n"
        "  setStatus: (name: string, disabled: boolean) =>\n"
        "    apiClient.patch<AuthFileStatusResponse>('/auth-files/status', { name, disabled }),\n\n",
    )
    list_replacement = (
        "  list: fetchAuthFilesList,\n\n  patchFile: async (payload: AuthFilePatchPayload) => {\n    const response = await apiClient.patch<AuthFileStatusResponse>('/auth-files', payload);\n    invalidateAuthFilesListCache();\n    return response;\n  },\n\n  setStatus: async (name: string, disabled: boolean) => {\n    const response = await apiClient.patch<AuthFileStatusResponse>('/auth-files/status', { name, disabled });\n    invalidateAuthFilesListCache();\n    return response;\n  },\n"
    )
    if '  list: fetchAuthFilesList,\n' not in text:
        for list_variant in list_variants:
            if list_variant in text:
                write(auth_files_path, text.replace(list_variant, list_replacement, 1))
                break
        else:
            raise RuntimeError(f'Pattern not found in {auth_files_path}: auth files list method')
    replace_once(
        auth_files_path,
        "  patchFields: (name: string, fields: AuthFileFieldsPatch) =>\n    apiClient.patch('/auth-files/fields', { name, ...fields }),\n\n",
        "  setStatusWithFallback: async (name: string, disabled: boolean) => {\n    try {\n      return await authFilesApi.patchFile({ name, disabled });\n    } catch {\n      return authFilesApi.setStatus(name, disabled);\n    }\n  },\n\n  patchFields: async (name: string, fields: AuthFileFieldsPatch) => {\n    const response = await apiClient.patch('/auth-files/fields', { name, ...fields });\n    invalidateAuthFilesListCache();\n    return response;\n  },\n\n",
    )
    replace_once(
        auth_files_path,
        "    const payload = await apiClient.postForm<AuthFileBatchUploadResponse>('/auth-files', formData);\n    return normalizeBatchUploadResponse(payload, requestedNames);\n",
        "    const payload = await apiClient.postForm<AuthFileBatchUploadResponse>('/auth-files', formData);\n    invalidateAuthFilesListCache();\n    return normalizeBatchUploadResponse(payload, requestedNames);\n",
    )
    replace_once(
        auth_files_path,
        "    const payload = await apiClient.delete<AuthFileBatchDeleteResponse>('/auth-files', {\n      data: { names: requestedNames },\n    });\n    return normalizeBatchDeleteResponse(payload, requestedNames);\n",
        "    const payload = await apiClient.delete<AuthFileBatchDeleteResponse>('/auth-files', {\n      data: { names: requestedNames },\n    });\n    invalidateAuthFilesListCache();\n    return normalizeBatchDeleteResponse(payload, requestedNames);\n",
    )
    replace_once(
        auth_files_path,
        "  deleteAll: () => apiClient.delete('/auth-files', { params: { all: true } }),\n",
        "  deleteAll: async () => {\n    const response = await apiClient.delete('/auth-files', { params: { all: true } });\n    invalidateAuthFilesListCache();\n    return response;\n  },\n",
    )

    format_path = target / 'src/utils/format.ts'
    insert_once(
        format_path,
        "/**\n * 格式化文件大小\n */",
        "const API_KEY_MASK_REGEX =\n  /(sk-[A-Za-z0-9-_]{6,}|sk-ant-[A-Za-z0-9-_]{6,}|AIza[0-9A-Za-z-_]{8,}|AI[a-zA-Z0-9_-]{6,}|hf_[A-Za-z0-9]{6,}|pk_[A-Za-z0-9]{6,}|rk_[A-Za-z0-9]{6,})/g;\n\nexport function maskSensitiveText(value: string): string {\n  const trimmed = String(value || '').trim();\n  if (!trimmed) {\n    return '';\n  }\n\n  return trimmed.replace(API_KEY_MASK_REGEX, (match) => maskApiKey(match));\n}\n\n/**\n * 格式化文件大小\n */",
        "export function maskSensitiveText(value: string): string",
    )

    select_path = target / 'src/components/ui/Select.tsx'
    if 'triggerClassName?: string;' not in read(select_path):
        replace_once(
            select_path,
            "  placeholder?: string;\n  className?: string;\n  disabled?: boolean;\n",
            "  placeholder?: string;\n  className?: string;\n  triggerClassName?: string;\n  dropdownClassName?: string;\n  disabled?: boolean;\n",
        )
    if 'triggerClassName,' not in read(select_path):
        replace_once(
            select_path,
            "  placeholder,\n  className,\n  disabled = false,\n",
            "  placeholder,\n  className,\n  triggerClassName,\n  dropdownClassName,\n  disabled = false,\n",
        )
    if 'dropdownClassName].filter(Boolean).join' not in read(select_path):
        text = read(select_path)
        dropdown_class_replacements = [
            (
                "            className={styles.dropdown}\n",
                "            className={[styles.dropdown, dropdownClassName].filter(Boolean).join(' ')}\n",
            ),
            (
                "        className={styles.dropdown}\n",
                "        className={[styles.dropdown, dropdownClassName].filter(Boolean).join(' ')}\n",
            ),
        ]
        for old, new in dropdown_class_replacements:
            if old in text:
                write(select_path, text.replace(old, new, 1))
                break
        else:
            raise RuntimeError(f'Pattern not found in {select_path}: Select dropdown className')
    if 'triggerClassName].filter(Boolean).join' not in read(select_path):
        text = read(select_path)
        old_simple = "          className={styles.trigger}\n"
        old_sized = "          className={`${styles.trigger} ${size === 'sm' ? styles.triggerSm : ''}`.trim()}\n"
        if old_simple in text:
            write(
                select_path,
                text.replace(
                    old_simple,
                    "          className={[styles.trigger, triggerClassName].filter(Boolean).join(' ')}\n",
                    1,
                ),
            )
        elif old_sized in text:
            write(
                select_path,
                text.replace(
                    old_sized,
                    "          className={[styles.trigger, size === 'sm' ? styles.triggerSm : '', triggerClassName].filter(Boolean).join(' ')}\n",
                    1,
                ),
            )
        else:
            raise RuntimeError(f'Pattern not found in {select_path}: Select trigger className')


def patch_locales(target: Path) -> None:
    monitoring = json.loads(LOCALES_FILE.read_text())
    locales_dir = target / 'src/i18n/locales'
    for locale_path in sorted(locales_dir.glob('*.json')):
        data = json.loads(read(locale_path))
        additions = monitoring.get(locale_path.name, {})
        data.setdefault('nav', {}).update(additions.get('nav', {}))
        proxy_pool_nav = PROXY_POOL_NAV_LOCALE_KEYS.get(
            locale_path.name,
            PROXY_POOL_NAV_LOCALE_KEYS['en.json'],
        )
        data.setdefault('nav', {})['proxy_pool'] = proxy_pool_nav['label']
        oauth_model_policy_nav = OAUTH_MODEL_POLICY_NAV_LOCALE_KEYS.get(
            locale_path.name,
            OAUTH_MODEL_POLICY_NAV_LOCALE_KEYS['en.json'],
        )
        data.setdefault('nav', {})['oauth_model_policy'] = oauth_model_policy_nav['label']
        data.setdefault('nav_groups', {})['pro'] = 'PRO'
        nav_additions = additions.get('nav', {})
        data.setdefault('nav_meta', {}).update(
            additions.get(
                'nav_meta',
                {
                    'monitoring_center': nav_additions.get('monitoring_center', 'Request Monitoring'),
                    'account_inspection': nav_additions.get('account_inspection', 'Account Inspection'),
                    'routing_policy': nav_additions.get('routing_policy', 'Routing Policy'),
                },
            )
        )
        data.setdefault('nav_meta', {})['proxy_pool'] = proxy_pool_nav['meta']
        data.setdefault('nav_meta', {})['oauth_model_policy'] = oauth_model_policy_nav['meta']
        data['monitoring'] = additions.get('monitoring', data.get('monitoring', {}))
        data['account_usage'] = additions.get('account_usage', data.get('account_usage', {}))
        data['usage_stats'] = additions.get('usage_stats', data.get('usage_stats', {}))
        data['routing_policy'] = additions.get('routing_policy', data.get('routing_policy', {}))
        data['proxy_pool'] = additions.get(
            'proxy_pool',
            monitoring.get('en.json', {}).get('proxy_pool', data.get('proxy_pool', {})),
        )
        data['oauth_model_policy'] = additions.get(
            'oauth_model_policy',
            monitoring.get('en.json', {}).get(
                'oauth_model_policy',
                data.get('oauth_model_policy', {}),
            ),
        )
        data.setdefault('quota_management', {}).update(QUOTA_LOCALE_KEYS.get(locale_path.name, {}))
        gemini_cli_locale = GEMINI_CLI_LOCALE_KEYS.get(locale_path.name, GEMINI_CLI_LOCALE_KEYS['en.json'])
        data.setdefault('auth_files', {})['filter_gemini-cli'] = gemini_cli_locale['auth_filter']
        data.setdefault('auth_files', {})['search_placeholder'] = AUTH_FILES_SEARCH_PLACEHOLDER_KEYS.get(
            locale_path.name,
            AUTH_FILES_SEARCH_PLACEHOLDER_KEYS['en.json'],
        )
        data.setdefault('auth_files', {})['sort_plan_desc'] = AUTH_FILES_PLAN_SORT_LABEL_KEYS.get(
            locale_path.name,
            AUTH_FILES_PLAN_SORT_LABEL_KEYS['en.json'],
        )
        data.setdefault('auth_files', {})['sort_quota_desc'] = AUTH_FILES_QUOTA_SORT_LABEL_KEYS.get(
            locale_path.name,
            AUTH_FILES_QUOTA_SORT_LABEL_KEYS['en.json'],
        )
        data.setdefault('auth_files', {})['selected_count'] = AUTH_FILES_SELECTED_COUNT_LABEL_KEYS.get(
            locale_path.name,
            AUTH_FILES_SELECTED_COUNT_LABEL_KEYS['en.json'],
        )
        data.setdefault('auth_files', {}).update(
            AUTH_FILES_BATCH_LOCALE_KEYS.get(
                locale_path.name,
                AUTH_FILES_BATCH_LOCALE_KEYS['en.json'],
            )
        )
        data.setdefault('quota_management', {}).update(
            QUOTA_DELETE_LOCALE_KEYS.get(
                locale_path.name,
                QUOTA_DELETE_LOCALE_KEYS['en.json'],
            )
        )
        data.setdefault('gemini_cli_quota', {}).update(gemini_cli_locale['quota'])
        data.setdefault('xai_quota', {}).update(
            XAI_QUOTA_LOCALE_KEYS.get(locale_path.name, XAI_QUOTA_LOCALE_KEYS['en.json'])
        )
        data.setdefault('system_info', {}).update(
            MANAGEMENT_UPDATE_LOCALE_KEYS.get(
                locale_path.name,
                MANAGEMENT_UPDATE_LOCALE_KEYS['en.json'],
            )
        )
        cloak_locale = CLAUDE_MODEL_ID_CLOAK_LOCALE_KEYS.get(
            locale_path.name,
            CLAUDE_MODEL_ID_CLOAK_LOCALE_KEYS['en.json'],
        )
        advanced_locale = (
            data.setdefault('config_management', {})
            .setdefault('visual', {})
            .setdefault('sections', {})
            .setdefault('advanced', {})
        )
        advanced_locale.update(
            {
                'claude_model_id_cloak_title': cloak_locale['title'],
                'claude_model_id_cloak_description': cloak_locale['description'],
                'claude_model_id_cloak_label': cloak_locale['label'],
                'claude_model_id_cloak_hint': cloak_locale['hint'],
                'claude_model_id_cloak_auto': cloak_locale['auto'],
                'claude_model_id_cloak_always': cloak_locale['always'],
                'claude_model_id_cloak_never': cloak_locale['never'],
            }
        )
        write(locale_path, json.dumps(data, ensure_ascii=False, indent=2) + '\n')


def _ensure_interface_field(path: Path, interface_name: str, field: str) -> None:
    text = read(path)
    start = text.find(f'export interface {interface_name} {{')
    if start == -1:
        raise RuntimeError(f'Interface not found in {path}: {interface_name}')
    end = text.find('\n}', start)
    if end == -1:
        raise RuntimeError(f'Interface end not found in {path}: {interface_name}')
    block = text[start:end]
    if field.strip() in block:
        return
    write(path, f'{text[:end]}\n{field}{text[end:]}')


def patch_quota_types_latest(target: Path) -> None:
    path = target / 'src/types/quota.ts'
    insert_once(
        path,
        '// API payload types\n',
        "// API payload types\nexport interface GeminiCliQuotaBucket {\n  modelId?: string;\n  model_id?: string;\n  tokenType?: string;\n  token_type?: string;\n  remainingFraction?: number | string;\n  remaining_fraction?: number | string;\n  remainingAmount?: number | string;\n  remaining_amount?: number | string;\n  resetTime?: string;\n  reset_time?: string;\n}\n\nexport interface GeminiCliQuotaPayload {\n  buckets?: GeminiCliQuotaBucket[];\n}\n\nexport interface GeminiCliParsedBucket {\n  modelId: string;\n  tokenType: string | null;\n  remainingFraction: number | null;\n  remainingAmount: number | null;\n  resetTime: string | undefined;\n}\n\n",
        'export interface GeminiCliQuotaBucket',
    )
    insert_once(
        path,
        'export interface CodexQuotaWindow',
        "export interface GeminiCliQuotaBucketState {\n  id: string;\n  label: string;\n  remainingFraction: number | null;\n  remainingAmount: number | null;\n  resetTime: string | undefined;\n  tokenType: string | null;\n  modelIds?: string[];\n  resetAtMs?: number | null;\n  periodHours?: number | null;\n}\n\nexport interface GeminiCliQuotaState {\n  status: 'idle' | 'loading' | 'success' | 'error';\n  buckets: GeminiCliQuotaBucketState[];\n  projectId?: string;\n  project_id?: string;\n  tierLabel?: string | null;\n  tierId?: string | null;\n  creditBalance?: number | null;\n  quotaProviderSnapshot?: boolean;\n  error?: string;\n  errorStatus?: number;\n  cachedAt?: number;\n}\n\nexport interface CodexQuotaWindow",
        'export interface GeminiCliQuotaState',
    )
    insert_once(
        path,
        'export interface XaiBillingSummary {\n',
        "export interface XaiFreeQuotaSummary {\n  source?: 'rate_limit_headers' | 'free_usage_exhausted';\n  windowKind?: 'rolling_24h' | string;\n  usedTokens?: number | string;\n  limitTokens?: number | string;\n  remainingTokens?: number | string;\n  limitRequests?: number | string;\n  remainingRequests?: number | string;\n  observedAt?: number | string;\n  exhausted?: boolean;\n  model?: string;\n}\n\nexport interface XaiBillingSummary {\n",
        'export interface XaiFreeQuotaSummary',
    )
    replace_once(
        path,
        "  planType?: 'paid';\n",
        "  planType?: 'free' | 'supergrok' | 'x-premium-plus' | 'supergrok-heavy' | 'paid' | 'paid-unknown';\n",
    )
    _ensure_interface_field(path, 'XaiBillingSummary', '  freeQuota?: XaiFreeQuotaSummary;')
    for interface_name in (
        'ClaudeQuotaState', 'AntigravityQuotaState', 'CodexQuotaState', 'KimiQuotaState', 'XaiQuotaState'
    ):
        _ensure_interface_field(path, interface_name, '  cachedAt?: number;')


def patch_quota_provider_model_latest(target: Path) -> None:
    types_path = target / 'src/features/quota/providers/types.ts'
    replace_once(types_path, '  CodexQuotaState,\n  KimiQuotaState,', '  CodexQuotaState,\n  GeminiCliQuotaState,\n  KimiQuotaState,')
    replace_once(types_path, "export type QuotaProviderType = 'antigravity' | 'claude' | 'codex' | 'kimi' | 'xai';", "export type QuotaProviderType = 'antigravity' | 'claude' | 'codex' | 'gemini-cli' | 'kimi' | 'xai';")
    replace_once(types_path, '  codexQuota: Record<string, CodexQuotaState>;\n  kimiQuota:', '  codexQuota: Record<string, CodexQuotaState>;\n  geminiCliQuota: Record<string, GeminiCliQuotaState>;\n  kimiQuota:')
    replace_once(types_path, '  setCodexQuota: (updater: QuotaUpdater<Record<string, CodexQuotaState>>) => void;\n  setKimiQuota:', '  setCodexQuota: (updater: QuotaUpdater<Record<string, CodexQuotaState>>) => void;\n  setGeminiCliQuota: (updater: QuotaUpdater<Record<string, GeminiCliQuotaState>>) => void;\n  setKimiQuota:')

    xai_paid_path = target / 'src/utils/quota/xaiPaid.ts'
    replace_once(
        xai_paid_path,
        '''export const isPaidXaiAuthFile = (file: AuthFileItem | Record<string, unknown>): boolean => {
  const records = collectAuthRecords(file);
  const usesOfficialApi = records.some((record) =>
    isTruthyValue(record.using_api ?? record.usingApi)
  );
  const prefixes = readStrings(records, ['prefix']);
''',
        '''export const isXaiUsingOfficialAPI = (file: AuthFileItem | Record<string, unknown>): boolean =>
  collectAuthRecords(file).some((record) =>
    isTruthyValue(record.using_api ?? record.usingApi)
  );

export const isPaidXaiAuthFile = (file: AuthFileItem | Record<string, unknown>): boolean => {
  const records = collectAuthRecords(file);
  const usesOfficialApi = isXaiUsingOfficialAPI(file);
  const prefixes = readStrings(records, ['prefix']);
''',
    )
    xai_data_path = target / 'src/features/quota/providers/xai/data.ts'
    replace_once(
        xai_data_path,
        'const requestXaiPaidHealth = async (authIndex: string): Promise<XaiBillingSummary> => {',
        'export const requestXaiPaidHealth = async (authIndex: string): Promise<XaiBillingSummary> => {',
    )
    replace_once(
        xai_data_path,
        '''    url,
    header,
  });
''',
        '''    url,
    header,
    useExecutor: true,
  });
''',
    )
    replace_once(
        xai_data_path,
        '''        url: XAI_API_ME_URL,
        header: XAI_API_REQUEST_HEADERS,
''',
        '''        url: XAI_API_ME_URL,
        header: XAI_API_REQUEST_HEADERS,
        useExecutor: true,
''',
    )
    replace_once(
        xai_data_path,
        '''          stream: false,
        }),
''',
        '''          stream: false,
        }),
        useExecutor: true,
''',
    )

    index_path = target / 'src/features/quota/providers/index.ts'
    replace_once(index_path, "import { XAI_CONFIG } from './xai/data';\nimport { XaiQuotaBody } from './xai/XaiQuotaBody';", "import { GEMINI_CLI_CONFIG, GeminiCliQuotaBody, PRO_XAI_CONFIG, ProXaiQuotaBody } from '@/pro/modules/quota';")
    replace_once(index_path, "  errorStatus?: number;\n}", "  errorStatus?: number;\n  cachedAt?: number;\n}")
    replace_once(index_path, "  codex: { ...CODEX_CONFIG, Body: CodexQuotaBody } as unknown as QuotaAdapter,\n  kimi:", "  codex: { ...CODEX_CONFIG, Body: CodexQuotaBody } as unknown as QuotaAdapter,\n  'gemini-cli': { ...GEMINI_CLI_CONFIG, Body: GeminiCliQuotaBody } as unknown as QuotaAdapter,\n  kimi:")
    replace_once(index_path, '  xai: { ...XAI_CONFIG, Body: XaiQuotaBody } as unknown as QuotaAdapter,', '  xai: { ...PRO_XAI_CONFIG, Body: ProXaiQuotaBody } as unknown as QuotaAdapter,')

    constants_path = target / 'src/features/quota/constants.ts'
    replace_once(constants_path, "  'codex',\n  'xai',", "  'codex',\n  'gemini-cli',\n  'xai',")

    logic_path = target / 'src/features/quota/logic.ts'
    replace_once(logic_path, "import { KIMI_CONFIG } from './providers/kimi/data';", "import { GEMINI_CLI_CONFIG } from '@/pro/modules/quota';\nimport { KIMI_CONFIG } from './providers/kimi/data';")
    replace_once(logic_path, '  codex: CODEX_CONFIG.filterFn,\n  kimi:', "  codex: CODEX_CONFIG.filterFn,\n  'gemini-cli': GEMINI_CLI_CONFIG.filterFn,\n  kimi:")

    test_path = target / 'tests/quotaPageLogic.test.ts'
    replace_once(test_path, "      codex: 2,\n      xai: 1,", "      codex: 2,\n      'gemini-cli': 0,\n      xai: 1,")


def patch_quota_page_cache_refresh(target: Path) -> None:
    path = target / 'src/features/quota/QuotaPage.tsx'
    insert_once(
        path,
        "import { readQuotaUiState, writeQuotaUiState } from './uiState';\n",
        "import { readQuotaUiState, writeQuotaUiState } from './uiState';\n"
        "import { quotaPersistenceMiddleware } from '@/pro/modules/quota';\n",
        "quotaPersistenceMiddleware } from '@/pro/modules/quota'",
    )
    insert_once(
        path,
        "  useEffect(() => {\n    void loadFiles();\n  }, [loadFiles]);\n",
        "  useEffect(() => {\n    void loadFiles();\n  }, [loadFiles]);\n\n"
        "  useEffect(() => {\n"
        "    void quotaPersistenceMiddleware.ensureFresh();\n"
        "  }, []);\n",
        'void quotaPersistenceMiddleware.ensureFresh();',
    )


def patch_quota_page_latest(target: Path) -> None:
    path = target / 'src/features/quota/QuotaPage.tsx'
    patch_quota_page_cache_refresh(target)
    insert_once(path, "import { EmptyState } from '@/components/ui/EmptyState';\n", "import { EmptyState } from '@/components/ui/EmptyState';\nimport { Input } from '@/components/ui/Input';\nimport { IconSearch } from '@/components/ui/icons';\n", 'quota_management.search_label')
    insert_once(path, "import { readQuotaUiState, writeQuotaUiState } from './uiState';\n", "import { readQuotaUiState, writeQuotaUiState } from './uiState';\nimport { buildQuotaSearchValues, matchesQuotaSearch, useBackendQuotaRefresh, useQuotaSelection } from '@/pro/modules/quota';\n", 'matchesQuotaSearch')
    replace_once(path, '  const codexQuota = useQuotaStore((state) => state.codexQuota);\n  const kimiQuota', '  const codexQuota = useQuotaStore((state) => state.codexQuota);\n  const geminiCliQuota = useQuotaStore((state) => state.geminiCliQuota);\n  const kimiQuota')
    replace_once(path, "        codex: codexQuota,\n        kimi:", "        codex: codexQuota,\n        'gemini-cli': geminiCliQuota,\n        kimi:")
    replace_once(path, '[antigravityQuota, claudeQuota, codexQuota, kimiQuota, xaiQuota]', '[antigravityQuota, claudeQuota, codexQuota, geminiCliQuota, kimiQuota, xaiQuota]')
    marker = "  const getQuota = useCallback(\n"
    search_state = "  const [search, setSearch] = useState('');\n  const quotaSearchStore = useMemo(\n    () => ({ antigravityQuota, claudeQuota, codexQuota, geminiCliQuota, kimiQuota, xaiQuota }),\n    [antigravityQuota, claudeQuota, codexQuota, geminiCliQuota, kimiQuota, xaiQuota]\n  );\n\n"
    insert_once(path, marker, search_state + marker, 'const [search, setSearch]')
    entries_marker = "  const entries = useMemo(() => classifyQuotaFiles(files), [files]);\n"
    searched_entries = entries_marker + "  const searchedEntries = useMemo(\n    () => entries.filter(({ file }) => matchesQuotaSearch(buildQuotaSearchValues(file, quotaSearchStore, t), search)),\n    [entries, quotaSearchStore, search, t]\n  );\n"
    insert_once(path, entries_marker, searched_entries, 'const searchedEntries = useMemo(')
    replace_once(
        path,
        '  const tabCounts = useMemo(() => buildTabCounts(entries), [entries]);',
        '  const tabCounts = useMemo(() => buildTabCounts(searchedEntries), [searchedEntries]);',
    )
    replace_once(
        path,
        '  const filteredEntries = useMemo(() => filterEntriesByTab(entries, tab), [entries, tab]);',
        '  const filteredEntries = useMemo(() => filterEntriesByTab(searchedEntries, tab), [searchedEntries, tab]);',
    )
    insert_once(
        path,
        "  const handleRefreshAll = useCallback(() => {\n",
        "  const backendRefresh = useBackendQuotaRefresh(tab === 'all' ? '' : tab);\n"
        "  const selection = useQuotaSelection(loadFiles);\n"
        "  const pageNames = useMemo(() => pageItems.map(({ file }) => file.name), [pageItems]);\n"
        "  const selectedPageCount = selection.selectedCountIn(pageNames);\n\n"
        "  const handleRefreshAll = useCallback(() => {\n",
        'const backendRefresh = useBackendQuotaRefresh',
    )
    replace_once(
        path,
        "  const handleRefreshAll = useCallback(() => {\n"
        "    if (disableControls) return;\n"
        "    pendingRefreshRef.current = true;\n"
        "    void loadFiles();\n"
        "  }, [disableControls, loadFiles]);\n",
        "  const handleRefreshAll = useCallback(() => {\n"
        "    if (disableControls) return;\n"
        "    if (tab !== 'all') {\n"
        "      void backendRefresh.start();\n"
        "      return;\n"
        "    }\n"
        "    pendingRefreshRef.current = true;\n"
        "    void loadFiles();\n"
        "  }, [backendRefresh, disableControls, loadFiles, tab]);\n",
    )
    replace_once(
        path,
        "        refreshing={loading || batchLoading}\n",
        "        refreshing={loading || batchLoading || backendRefresh.isRefreshing}\n"
        "        refreshProgress={backendRefresh.isRefreshing && backendRefresh.total > 0\n"
        "          ? t('quota_management.refresh_progress', { completed: backendRefresh.completed, total: backendRefresh.total })\n"
        "          : undefined}\n",
    )
    insert_once(
        path,
        "        {error && (\n",
        "        {pageNames.length > 0 && (\n"
        "          <div className={styles.selectionBar}>\n"
        "            <label className={styles.selectAllLabel}>\n"
        "              <input\n"
        "                type=\"checkbox\"\n"
        "                checked={selection.areAllSelected(pageNames)}\n"
        "                onChange={() => selection.toggleSelectAll(pageNames)}\n"
        "              />\n"
        "              {t('quota_management.select_all')}\n"
        "            </label>\n"
        "            {selectedPageCount > 0 && (\n"
        "              <Button\n"
        "                variant=\"danger\"\n"
        "                size=\"sm\"\n"
        "                onClick={() => selection.deleteSelected(pageNames)}\n"
        "                disabled={disableControls}\n"
        "              >\n"
        "                {t('quota_management.delete_selected', { count: selectedPageCount })}\n"
        "              </Button>\n"
        "            )}\n"
        "          </div>\n"
        "        )}\n\n"
        "        {error && (\n",
        'className={styles.selectionBar}',
    )
    replace_once(
        path,
        "                onReset={() => resetQuota(entry.file, QUOTA_ADAPTERS[entry.type])}\n",
        "                selected={selection.selectedNames.has(entry.file.name)}\n"
        "                onSelect={() => selection.toggleSelect(entry.file.name)}\n"
        "                onDelete={() => selection.deleteOne(entry.file.name)}\n"
        "                onReset={() => resetQuota(entry.file, QUOTA_ADAPTERS[entry.type])}\n",
    )
    insert_once(path, "        {error && (\n", "        <Input\n          type=\"search\"\n          value={search}\n          onChange={(event) => { setSearch(event.target.value); setPage(1); }}\n          placeholder={t('quota_management.search_placeholder')}\n          aria-label={t('quota_management.search_label')}\n          rightElement={<IconSearch size={18} />}\n        />\n\n        {error && (\n", 'rightElement={<IconSearch')


def patch_quota_cards_latest(target: Path) -> None:
    card_path = target / 'src/features/quota/components/QuotaCard.tsx'
    insert_once(card_path, "import { resolveQuotaErrorMessage } from '@/utils/quota';\n", "import { resolveQuotaErrorMessage } from '@/utils/quota';\nimport { QuotaCachedTime } from '@/pro/modules/quota';\n", 'import { QuotaCachedTime }')
    replace_once(card_path, "        ) : quota ? (\n          <adapter.Body quota={quota} classes={quotaClasses} />\n        ) : (", "        ) : quota ? (\n          <>\n            <adapter.Body quota={quota} classes={quotaClasses} />\n            <QuotaCachedTime quotaStatus={status} cachedAt={quota.cachedAt} />\n          </>\n        ) : (")
    replace_once(card_path, "import { IconRefreshCw } from '@/components/ui/icons';", "import { IconRefreshCw, IconTrash2 } from '@/components/ui/icons';")
    replace_once(
        card_path,
        "  resetting: boolean;\n  /** 首屏级联入场延迟；null = 不入场（切 tab / 翻页 / 刷新新挂载的卡片）。 */\n",
        "  resetting: boolean;\n  selected: boolean;\n  onSelect: () => void;\n  onDelete?: () => void;\n  /** 首屏级联入场延迟；null = 不入场（切 tab / 翻页 / 刷新新挂载的卡片）。 */\n",
    )
    replace_once(
        card_path,
        "    resetting,\n    entranceDelayMs,\n",
        "    resetting,\n    selected,\n    onSelect,\n    onDelete,\n    entranceDelayMs,\n",
    )
    replace_once(
        card_path,
        "        <span className={styles.fileName} title={file.name}>\n",
        "        <input\n"
        "          type=\"checkbox\"\n"
        "          className={styles.selectCheckbox}\n"
        "          checked={selected}\n"
        "          onChange={onSelect}\n"
        "          aria-label={t('quota_management.select_credential', { name: file.name })}\n"
        "        />\n"
        "        <span className={styles.fileName} title={file.name}>\n",
    )
    replace_once(card_path, "      {status !== 'idle' && (", "      {(status !== 'idle' || onDelete) && (")
    replace_once(
        card_path,
        "        <footer className={styles.actionRow}>\n",
        "        <footer className={styles.actionRow}>\n"
        "          <button\n"
        "            type=\"button\"\n"
        "            className={styles.actionPill}\n"
        "            onClick={onDelete}\n"
        "            title={t('quota_management.delete_one')}\n"
        "          >\n"
        "            <IconTrash2 size={13} />\n"
        "            {t('quota_management.delete_one')}\n"
        "          </button>\n",
    )

    page_styles = target / 'src/features/quota/QuotaPage.module.scss'
    insert_once(
        page_styles,
        ".errorBanner {\n",
        ".selectionBar {\n"
        "  display: flex;\n"
        "  align-items: center;\n"
        "  justify-content: space-between;\n"
        "  gap: 12px;\n"
        "  min-height: 36px;\n"
        "}\n\n"
        ".selectAllLabel {\n"
        "  display: inline-flex;\n"
        "  align-items: center;\n"
        "  gap: 8px;\n"
        "  color: var(--text-secondary);\n"
        "  font-size: 13px;\n"
        "  cursor: pointer;\n"
        "}\n\n"
        ".errorBanner {\n",
        '.selectionBar {',
    )
    card_styles = target / 'src/features/quota/components/QuotaCard.module.scss'
    insert_once(
        card_styles,
        ".fileName {\n",
        ".selectCheckbox {\n"
        "  flex: 0 0 auto;\n"
        "  width: 16px;\n"
        "  height: 16px;\n"
        "  cursor: pointer;\n"
        "}\n\n"
        ".fileName {\n",
        '.selectCheckbox {',
    )

    header_path = target / 'src/features/quota/components/QuotaHeader.tsx'
    replace_once(header_path, "  refreshing: boolean;\n  disableControls: boolean;", "  refreshing: boolean;\n  refreshProgress?: string;\n  disableControls: boolean;")
    replace_once(
        header_path,
        "  const { totalCount, loadedCount, attentionCount, refreshing, disableControls, onRefreshAll } =\n    props;\n",
        "  const { totalCount, loadedCount, attentionCount, refreshing, refreshProgress, disableControls, onRefreshAll } =\n    props;\n",
    )
    replace_once(header_path, "          {t('quota_management.refresh_all_credentials')}\n", "          {refreshProgress || t('quota_management.refresh_all_credentials')}\n")

    auth_path = target / 'src/features/authFiles/components/AuthFileQuotaSection.tsx'
    insert_once(auth_path, "import { bindQuotaClasses } from '@/features/quota/types';\n", "import { bindQuotaClasses } from '@/features/quota/types';\nimport { QuotaCachedTime } from '@/pro/modules/quota';\n", 'import { QuotaCachedTime }')
    replace_once(auth_path, "      ) : quota ? (\n        <adapter.Body quota={quota} classes={compactQuotaClasses} />\n      ) : (", "      ) : quota ? (\n        <>\n          <adapter.Body quota={quota} classes={compactQuotaClasses} />\n          <QuotaCachedTime quotaStatus={quotaStatus} cachedAt={quota.cachedAt} />\n        </>\n      ) : (")


def patch_quota_provider_timestamps_latest(target: Path) -> None:
    for provider, setter in (
        ('antigravity', 'setAntigravityQuota'), ('claude', 'setClaudeQuota'),
        ('codex', 'setCodexQuota'), ('kimi', 'setKimiQuota')
    ):
        ensure_cached_at_in_quota_success_state(
            target / f'src/features/quota/providers/{provider}/data.ts', setter
        )


def patch_auth_files_gemini_quota_latest(target: Path) -> None:
    path = target / 'src/features/authFiles/constants.ts'
    replace_once(path, "export type QuotaProviderType = 'antigravity' | 'claude' | 'codex' | 'kimi' | 'xai';", "export type QuotaProviderType = 'antigravity' | 'claude' | 'codex' | 'gemini-cli' | 'kimi' | 'xai';")
    for marker in (
        'export const QUOTA_PROVIDER_TYPES = new Set<QuotaProviderType>([',
        'export const AUTH_FILE_MANUAL_REFRESH_PROVIDERS = new Set([',
    ):
        text = read(path)
        start = text.find(marker)
        end = text.find('\n]);' if 'Set' in marker or 'PROVIDERS' in marker else '\n];', start)
        if start == -1 or end == -1:
            raise RuntimeError(f'Provider list not found in {path}: {marker}')
        block = text[start:end]
        if "'gemini-cli'" in block:
            continue
        updated = block.replace("  'codex',\n", "  'codex',\n  'gemini-cli',\n", 1)
        write(path, f'{text[:start]}{updated}{text[end:]}')
    quota_section_path = target / 'src/features/authFiles/components/AuthFileQuotaSection.tsx'
    replace_once(
        quota_section_path,
        "    if (quotaType === 'codex') return state.codexQuota[file.name] as QuotaCardState | undefined;\n    if (quotaType === 'kimi')",
        "    if (quotaType === 'codex') return state.codexQuota[file.name] as QuotaCardState | undefined;\n    if (quotaType === 'gemini-cli') return state.geminiCliQuota[file.name] as QuotaCardState | undefined;\n    if (quotaType === 'kimi')",
    )


def patch_auth_files_page_search_latest(target: Path) -> None:
    path = target / 'src/features/authFiles/AuthFilesPage.tsx'
    replace_once(path, "import { useAuthStore, useNotificationStore, useThemeStore } from '@/stores';", "import { useAuthStore, useNotificationStore, useThemeStore, useQuotaStore } from '@/stores';")
    insert_once(path, "import { useAuthStore, useNotificationStore, useThemeStore, useQuotaStore } from '@/stores';\n", "import { buildQuotaSearchValues, matchesQuotaSearch } from '@/pro/modules/quota';\nimport { useAuthStore, useNotificationStore, useThemeStore, useQuotaStore } from '@/stores';\n", 'buildQuotaSearchValues')
    insert_once(
        path,
        '  const statusBarCache = useAuthFilesStatusBarCache(files);\n',
        "  const statusBarCache = useAuthFilesStatusBarCache(files);\n\n  const antigravityQuota = useQuotaStore((state) => state.antigravityQuota);\n  const claudeQuota = useQuotaStore((state) => state.claudeQuota);\n  const codexQuota = useQuotaStore((state) => state.codexQuota);\n  const geminiCliQuota = useQuotaStore((state) => state.geminiCliQuota);\n  const kimiQuota = useQuotaStore((state) => state.kimiQuota);\n  const xaiQuota = useQuotaStore((state) => state.xaiQuota);\n  const quotaSearchStore = useMemo(\n    () => ({ antigravityQuota, claudeQuota, codexQuota, geminiCliQuota, kimiQuota, xaiQuota }),\n    [antigravityQuota, claudeQuota, codexQuota, geminiCliQuota, kimiQuota, xaiQuota]\n  );\n",
        'const quotaSearchStore',
    )
    replace_once(
        path,
        '        return matchType && matchesAuthFileSearch(item, normalizedSearch, wildcardSearch);',
        '        return matchType && (\n          matchesAuthFileSearch(item, normalizedSearch, wildcardSearch) ||\n          matchesQuotaSearch(buildQuotaSearchValues(item, quotaSearchStore, t), normalizedSearch)\n        );',
    )
    replace_once(path, '[filesMatchingStatusFilters, normalizedFilter, normalizedSearch, wildcardSearch]', '[filesMatchingStatusFilters, normalizedFilter, normalizedSearch, quotaSearchStore, t, wildcardSearch]')


def patch_auth_files_page_sorting_latest(target: Path) -> None:
    page_path = target / 'src/features/authFiles/AuthFilesPage.tsx'
    ui_state_path = target / 'src/features/authFiles/uiState.ts'
    replace_once(ui_state_path, "export const AUTH_FILES_SORT_MODES = ['default', 'az', 'priority'] as const;", "export const AUTH_FILES_SORT_MODES = ['default', 'az', 'priority', 'plan', 'quota'] as const;")
    insert_once(
        page_path,
        "import { buildQuotaSearchValues, matchesQuotaSearch } from '@/pro/modules/quota';\n",
        "import {\n"
        "  buildQuotaSearchValues,\n"
        "  compareAuthFilesByAvailableQuotaDescending,\n"
        "  compareAuthFilesByPlanDescending,\n"
        "  isAuthFilePlanSortProvider,\n"
        "  isAuthFileQuotaSortProvider,\n"
        "  matchesQuotaSearch,\n"
        "} from '@/pro/modules/quota';\n",
        'compareAuthFilesByPlanDescending',
    )
    insert_once(
        page_path,
        "  const enabledOnly = statusFilterMode === 'enabled';\n",
        "  const enabledOnly = statusFilterMode === 'enabled';\n"
        "  const planSortAvailable = isAuthFilePlanSortProvider(normalizedFilter);\n"
        "  const quotaSortAvailable = isAuthFileQuotaSortProvider(normalizedFilter);\n"
        "  const selectedSortModeAvailable =\n"
        "    (sortMode !== 'plan' || planSortAvailable) &&\n"
        "    (sortMode !== 'quota' || quotaSortAvailable);\n",
        'const planSortAvailable',
    )
    insert_once(
        page_path,
        "  const handleStatusFilterModeChange = useCallback((nextMode: AuthFilesStatusFilterMode) => {\n",
        "  useEffect(() => {\n"
        "    if (selectedSortModeAvailable) return;\n"
        "    setSortMode('default');\n"
        "    setPage(1);\n"
        "  }, [selectedSortModeAvailable]);\n\n"
        "  const handleStatusFilterModeChange = useCallback((nextMode: AuthFilesStatusFilterMode) => {\n",
        'if (selectedSortModeAvailable) return;',
    )
    replace_once(
        page_path,
        "  const sortOptions = useMemo(\n"
        "    () => [\n"
        "      { value: 'default', label: t('auth_files.sort_default') },\n"
        "      { value: 'az', label: t('auth_files.sort_az') },\n"
        "      { value: 'priority', label: t('auth_files.sort_priority') },\n"
        "    ],\n"
        "    [t]\n"
        "  );\n",
        "  const sortOptions = useMemo(() => {\n"
        "    const options: Array<{ value: AuthFilesSortMode; label: string }> = [\n"
        "      { value: 'default', label: t('auth_files.sort_default') },\n"
        "      { value: 'az', label: t('auth_files.sort_az') },\n"
        "      { value: 'priority', label: t('auth_files.sort_priority') },\n"
        "    ];\n"
        "    if (planSortAvailable) {\n"
        "      options.push({ value: 'plan', label: t('auth_files.sort_plan_desc') });\n"
        "    }\n"
        "    if (quotaSortAvailable) {\n"
        "      options.push({ value: 'quota', label: t('auth_files.sort_quota_desc') });\n"
        "    }\n"
        "    return options;\n"
        "  }, [planSortAvailable, quotaSortAvailable, t]);\n",
    )
    insert_once(
        page_path,
        '  const sorted = useMemo(() => sortAuthFiles(filtered, sortMode), [filtered, sortMode]);\n',
        "  const effectiveSortMode: AuthFilesSortMode =\n    selectedSortModeAvailable ? sortMode : 'default';\n  const sorted = useMemo(() => {\n    if (effectiveSortMode === 'plan') {\n      return [...filtered].sort((a, b) => compareAuthFilesByPlanDescending(a, b, quotaSearchStore));\n    }\n    if (effectiveSortMode === 'quota') {\n      return [...filtered].sort((a, b) => compareAuthFilesByAvailableQuotaDescending(a, b, quotaSearchStore));\n    }\n    return sortAuthFiles(filtered, effectiveSortMode);\n  }, [effectiveSortMode, filtered, quotaSearchStore]);\n",
        'const effectiveSortMode',
    )
    replace_once(page_path, '          sortMode={sortMode}\n', '          sortMode={effectiveSortMode}\n')


def patch_auth_files_batch_actions_latest(target: Path) -> None:
    hook_path = target / 'src/features/authFiles/hooks/useAuthFilesData.ts'
    replace_once(
        hook_path,
        "  batchDownload: (names: string[]) => Promise<void>;\n"
        "  batchSetStatus: (names: string[], enabled: boolean) => Promise<void>;\n"
        "  batchDelete: (names: string[]) => void;\n"
        "};\n",
        "  batchDownload: (names: string[]) => Promise<void>;\n"
        "  batchSetStatus: (names: string[], enabled: boolean) => Promise<void>;\n"
        "  batchDelete: (names: string[]) => void;\n"
        "  batchTest: (files: AuthFileItem[]) => Promise<import('./useAuthFilesBatchActions').BatchActionSummary>;\n"
        "  batchClearErrors: (files: AuthFileItem[]) => Promise<import('./useAuthFilesBatchActions').BatchActionSummary>;\n"
        "  batchTestRunning: boolean;\n"
        "  batchClearErrorsRunning: boolean;\n"
        "  batchResultType: 'test' | 'clear' | null;\n"
        "  batchResult: import('./useAuthFilesBatchActions').BatchActionSummary | null;\n"
        "  clearBatchResult: () => void;\n"
        "};\n",
    )
    insert_once(
        hook_path,
        "import { notifyAuthFilesChanged } from '@/features/authFiles/authFilesEvents';\n",
        "import { notifyAuthFilesChanged } from '@/features/authFiles/authFilesEvents';\n"
        "import { useAuthFilesBatchActions } from './useAuthFilesBatchActions';\n",
        "import { useAuthFilesBatchActions } from './useAuthFilesBatchActions';",
    )
    replace_once(
        hook_path,
        "  const [selectedFiles, setSelectedFiles] = useState<Set<string>>(new Set());\n",
        "  const [selectedFiles, setSelectedFiles] = useState<Set<string>>(new Set());\n"
        "  const batchActions = useAuthFilesBatchActions();\n",
    )
    replace_once(
        hook_path,
        "    batchDownload,\n    batchSetStatus,\n    batchDelete,\n  };\n}\n",
        "    batchDownload,\n    batchSetStatus,\n    batchDelete,\n"
        "    batchTest: batchActions.batchTest,\n"
        "    batchClearErrors: batchActions.batchClearErrors,\n"
        "    batchTestRunning: batchActions.batchTestRunning,\n"
        "    batchClearErrorsRunning: batchActions.batchClearErrorsRunning,\n"
        "    batchResultType: batchActions.batchResultType,\n"
        "    batchResult: batchActions.batchResult,\n"
        "    clearBatchResult: batchActions.clearBatchResult,\n"
        "  };\n}\n",
    )

    page_path = auth_files_page_path(target)
    insert_once(
        page_path,
        "import { BatchActionBar } from '@/features/authFiles/components/BatchActionBar';\n",
        "import { BatchActionBar } from '@/features/authFiles/components/BatchActionBar';\n"
        "import { BatchActionResultDialog } from '@/features/authFiles/components/BatchActionResultDialog';\n",
        'BatchActionResultDialog',
    )
    replace_once(
        page_path,
        "    batchDownload,\n    batchSetStatus,\n    batchDelete,\n  } = useAuthFilesData({ onFilesMutated: invalidateDerivedCaches });\n",
        "    batchDownload,\n    batchSetStatus,\n    batchDelete,\n"
        "    batchTest,\n    batchClearErrors,\n"
        "    batchTestRunning,\n    batchClearErrorsRunning,\n"
        "    batchResultType,\n    batchResult,\n    clearBatchResult,\n"
        "  } = useAuthFilesData({ onFilesMutated: invalidateDerivedCaches });\n",
    )
    replace_once(
        page_path,
        "  const selectedNames = useMemo(() => Array.from(selectedFiles), [selectedFiles]);\n",
        "  const selectedNames = useMemo(() => Array.from(selectedFiles), [selectedFiles]);\n"
        "  const selectedFileItems = useMemo(\n"
        "    () => files.filter((file) => selectedFiles.has(file.name)),\n"
        "    [files, selectedFiles]\n"
        "  );\n",
    )
    replace_once(
        page_path,
        "      <BatchActionBar\n",
        "      <BatchActionResultDialog\n"
        "        open={batchResult !== null}\n"
        "        title={batchResultType === 'clear'\n"
        "          ? t('auth_files.batch_clear_errors_title')\n"
        "          : t('auth_files.batch_test_title')}\n"
        "        summary={batchResult}\n"
        "        onClose={clearBatchResult}\n"
        "      />\n\n"
        "      <BatchActionBar\n",
    )
    replace_once(
        page_path,
        "        onDownload={() => void batchDownload(selectedNames)}\n",
        "        onTest={() => void batchTest(selectedFileItems)}\n"
        "        onClearErrors={() => void batchClearErrors(selectedFileItems).then(() => loadFiles({ background: true }))}\n"
        "        testRunning={batchTestRunning}\n"
        "        clearErrorsRunning={batchClearErrorsRunning}\n"
        "        onDownload={() => void batchDownload(selectedNames)}\n",
    )

    bar_path = target / 'src/features/authFiles/components/BatchActionBar.tsx'
    replace_once(
        bar_path,
        "  onDownload: () => void;\n",
        "  onTest: () => void;\n"
        "  onClearErrors: () => void;\n"
        "  testRunning: boolean;\n"
        "  clearErrorsRunning: boolean;\n"
        "  onDownload: () => void;\n",
    )
    replace_once(
        bar_path,
        "    onDownload,\n",
        "    onTest,\n"
        "    onClearErrors,\n"
        "    testRunning,\n"
        "    clearErrorsRunning,\n"
        "    onDownload,\n",
    )
    replace_once(
        bar_path,
        "        <div className={styles.right}>\n"
        "          <Button\n"
        "            variant=\"secondary\"\n"
        "            size=\"sm\"\n"
        "            onClick={onDownload}\n",
        "        <div className={styles.right}>\n"
        "          <Button variant=\"secondary\" size=\"sm\" onClick={onTest} disabled={disableControls || testRunning}>\n"
        "            {testRunning ? t('auth_files.batch_test_running') : t('auth_files.batch_test')}\n"
        "          </Button>\n"
        "          <Button variant=\"secondary\" size=\"sm\" onClick={onClearErrors} disabled={disableControls || clearErrorsRunning}>\n"
        "            {clearErrorsRunning ? t('auth_files.batch_clear_errors_running') : t('auth_files.batch_clear_errors')}\n"
        "          </Button>\n"
        "          <Button\n"
        "            variant=\"secondary\"\n"
        "            size=\"sm\"\n"
        "            onClick={onDownload}\n",
    )


def patch_claude_model_id_cloak_setting(target: Path) -> None:
    visual_types = target / 'src/types/visualConfig.ts'
    replace_once(
        visual_types,
        "export type DisableImageGenerationMode = 'false' | 'true' | 'chat' | 'passthrough';\n",
        "export type DisableImageGenerationMode = 'false' | 'true' | 'chat' | 'passthrough';\n"
        "export type ClaudeModelIDCloakMode = 'auto' | 'always' | 'never';\n",
    )
    replace_once(
        visual_types,
        "  forceModelPrefix: boolean;\n  passthroughHeaders: boolean;\n",
        "  forceModelPrefix: boolean;\n  claudeModelIDCloakMode: ClaudeModelIDCloakMode;\n  passthroughHeaders: boolean;\n",
    )
    replace_once(
        visual_types,
        "  forceModelPrefix: false,\n  passthroughHeaders: false,\n",
        "  forceModelPrefix: false,\n  claudeModelIDCloakMode: 'auto',\n  passthroughHeaders: false,\n",
    )

    visual_hook = target / 'src/hooks/useVisualConfig.ts'
    replace_once(
        visual_hook,
        "  DisableImageGenerationMode,\n",
        "  ClaudeModelIDCloakMode,\n  DisableImageGenerationMode,\n",
    )
    replace_once(
        visual_hook,
        "export function parseDisableImageGenerationMode(raw: unknown): DisableImageGenerationMode {\n",
        "export function parseClaudeModelIDCloakMode(raw: unknown): ClaudeModelIDCloakMode {\n"
        "  if (typeof raw === 'string') {\n"
        "    const normalized = raw.trim().toLowerCase();\n"
        "    if (normalized === 'always' || normalized === 'never') return normalized;\n"
        "  }\n"
        "  return 'auto';\n"
        "}\n\n"
        "export function parseDisableImageGenerationMode(raw: unknown): DisableImageGenerationMode {\n",
    )
    replace_once(
        visual_hook,
        "      'forceModelPrefix',\n      'requestRetry',\n",
        "      'forceModelPrefix',\n      'claudeModelIDCloakMode',\n      'requestRetry',\n",
    )
    replace_once(
        visual_hook,
        "        forceModelPrefix: Boolean(parsed['force-model-prefix']),\n"
        "        passthroughHeaders: Boolean(parsed['passthrough-headers']),\n",
        "        forceModelPrefix: Boolean(parsed['force-model-prefix']),\n"
        "        claudeModelIDCloakMode: parseClaudeModelIDCloakMode(\n"
        "          parsed['claude-model-id-cloak-mode']\n"
        "        ),\n"
        "        passthroughHeaders: Boolean(parsed['passthrough-headers']),\n",
    )
    replace_once(
        visual_hook,
        "        if (dirtyFields.has('forceModelPrefix')) {\n"
        "          setBooleanInDoc(doc, ['force-model-prefix'], values.forceModelPrefix);\n"
        "        }\n"
        "        if (dirtyFields.has('passthroughHeaders')) {\n",
        "        if (dirtyFields.has('forceModelPrefix')) {\n"
        "          setBooleanInDoc(doc, ['force-model-prefix'], values.forceModelPrefix);\n"
        "        }\n"
        "        if (dirtyFields.has('claudeModelIDCloakMode')) {\n"
        "          setStringInDoc(doc, ['claude-model-id-cloak-mode'], values.claudeModelIDCloakMode);\n"
        "        }\n"
        "        if (dirtyFields.has('passthroughHeaders')) {\n",
    )

    visual_editor = target / 'src/components/config/VisualConfigEditor.tsx'
    replace_once(
        visual_editor,
        "  const disableImageGenerationHintId = `${disableImageGenerationLabelId}-hint`;\n"
        "  const keepaliveInputId = useId();\n",
        "  const disableImageGenerationHintId = `${disableImageGenerationLabelId}-hint`;\n"
        "  const claudeModelIDCloakModeLabelId = useId();\n"
        "  const claudeModelIDCloakModeHintId = `${claudeModelIDCloakModeLabelId}-hint`;\n"
        "  const keepaliveInputId = useId();\n",
    )
    replace_once(
        visual_editor,
        "  const countErrors = useCallback(\n",
        "  const claudeModelIDCloakModeOptions = useMemo(\n"
        "    () => [\n"
        "      { value: 'auto', label: t('config_management.visual.sections.advanced.claude_model_id_cloak_auto') },\n"
        "      { value: 'always', label: t('config_management.visual.sections.advanced.claude_model_id_cloak_always') },\n"
        "      { value: 'never', label: t('config_management.visual.sections.advanced.claude_model_id_cloak_never') },\n"
        "    ],\n"
        "    [t]\n"
        "  );\n\n"
        "  const countErrors = useCallback(\n",
    )
    replace_once(
        visual_editor,
        "                <Collapsible\n"
        "                  label={t('config_management.visual.sections.headers.title')}\n",
        "                <Collapsible\n"
        "                  label={t('config_management.visual.sections.advanced.claude_model_id_cloak_title')}\n"
        "                  hint={t('config_management.visual.sections.advanced.claude_model_id_cloak_description')}\n"
        "                  defaultOpen={false}\n"
        "                >\n"
        "                  <SectionGrid>\n"
        "                    <FieldAnchor fieldId=\"claudeModelIDCloakMode\">\n"
        "                      <FieldShell\n"
        "                        label={t('config_management.visual.sections.advanced.claude_model_id_cloak_label')}\n"
        "                        labelId={claudeModelIDCloakModeLabelId}\n"
        "                        hint={t('config_management.visual.sections.advanced.claude_model_id_cloak_hint')}\n"
        "                        hintId={claudeModelIDCloakModeHintId}\n"
        "                      >\n"
        "                        <Select\n"
        "                          value={values.claudeModelIDCloakMode}\n"
        "                          options={claudeModelIDCloakModeOptions}\n"
        "                          id={`${claudeModelIDCloakModeLabelId}-select`}\n"
        "                          disabled={disabled}\n"
        "                          ariaLabelledBy={claudeModelIDCloakModeLabelId}\n"
        "                          ariaDescribedBy={claudeModelIDCloakModeHintId}\n"
        "                          onChange={(nextValue) => onChange({\n"
        "                            claudeModelIDCloakMode: nextValue as VisualConfigValues['claudeModelIDCloakMode'],\n"
        "                          })}\n"
        "                        />\n"
        "                      </FieldShell>\n"
        "                    </FieldAnchor>\n"
        "                  </SectionGrid>\n"
        "                </Collapsible>\n\n"
        "                <Collapsible\n"
        "                  label={t('config_management.visual.sections.headers.title')}\n",
    )

    search_index = target / 'src/components/config/configSearchIndex.ts'
    replace_once(
        search_index,
        '  // Claude header defaults — qualifierKey disambiguates the shared "User-Agent" label.\n',
        "  {\n"
        "    fieldId: 'claudeModelIDCloakMode',\n"
        "    sectionId: 'advanced',\n"
        "    labelKey: L('sections.advanced.claude_model_id_cloak_label'),\n"
        "    hintKey: L('sections.advanced.claude_model_id_cloak_hint'),\n"
        "    yamlKeys: ['claude-model-id-cloak-mode'],\n"
        "    keywords: ['anthropic', 'claude desktop', 'claude code', 'model id', 'cloak'],\n"
        "  },\n"
        '  // Claude header defaults — qualifierKey disambiguates the shared "User-Agent" label.\n',
    )


def main() -> None:
    if len(sys.argv) > 2:
        raise SystemExit('Usage: apply_customizations.py [target_dir]')
    target = Path(sys.argv[1] if len(sys.argv) == 2 else '.').resolve()
    if not (target / 'src').is_dir() or not (target / 'package.json').is_file():
        raise SystemExit(f'Target directory does not look like the upstream project: {target}')
    if not OVERLAY_DIR.is_dir():
        raise SystemExit(f'Overlay directory not found: {OVERLAY_DIR}')

    copy_overlay(target)
    patch_modal_focus_restore(target)
    patch_modal_scroll_lock(target)
    patch_modal_content_scrollbar_layout(target)
    patch_routes(target)
    patch_layout(target)
    patch_icons(target)
    patch_quota_types_latest(target)
    patch_quota_store(target)
    patch_quota_constants(target)
    patch_quota_provider_model_latest(target)
    patch_quota_provider_timestamps_latest(target)
    patch_antigravity_quota_builders(target)
    patch_quota_page_latest(target)
    patch_quota_cards_latest(target)
    patch_account_inspection_page(target)
    patch_auth_files_page_search_latest(target)
    patch_auth_files_page_sorting_latest(target)
    patch_auth_files_batch_actions_latest(target)
    patch_auth_files_gemini_quota_latest(target)
    patch_auth_files_runtime_state(target)
    patch_account_usage_feature(target)
    patch_runtime_detection(target)
    patch_management_update_check(target)
    patch_api_client_connection_isolation(target)
    patch_supporting_api_and_types(target)
    patch_claude_model_id_cloak_setting(target)
    patch_locales(target)
    flush_writes()
    print(f'OK: CPA-Management customization applied to {target}')


if __name__ == '__main__':
    main()
