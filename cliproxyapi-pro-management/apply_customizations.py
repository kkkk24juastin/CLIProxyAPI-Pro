#!/usr/bin/env python3
import hashlib
import json
import re
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

AUTH_FILE_CONNECTION_TEST_LOCALE_KEYS = {
    'en.json': {
        'connection_test_button': 'Test connection',
        'connection_test_title': 'Test connection - {{account}}',
        'connection_test_hint': 'Sends one minimal real text-generation request through this exact credential. The request may consume a small amount of quota.',
        'connection_test_model': 'Test model',
        'connection_test_select_model': 'Select a text model',
        'connection_test_loading_models': 'Loading available models...',
        'connection_test_load_models_failed': 'Failed to load available models',
        'connection_test_no_models': 'This credential has no available text model to test.',
        'connection_test_ready': 'Ready to test',
        'connection_test_running': 'Sending test request...',
        'connection_test_success': 'Connection test passed',
        'connection_test_failed': 'Connection test failed',
        'connection_test_start': 'Start test',
        'connection_test_retry': 'Test again',
        'connection_test_latency': '{{count}} ms',
        'connection_test_output': 'Model output',
        'connection_test_error_detail': 'Failure details',
    },
    'ru.json': {
        'connection_test_button': 'Проверить подключение',
        'connection_test_title': 'Проверка подключения - {{account}}',
        'connection_test_hint': 'Отправляет один минимальный реальный запрос генерации текста через выбранные учётные данные. Запрос может израсходовать небольшую часть квоты.',
        'connection_test_model': 'Тестовая модель',
        'connection_test_select_model': 'Выберите текстовую модель',
        'connection_test_loading_models': 'Загрузка доступных моделей...',
        'connection_test_load_models_failed': 'Не удалось загрузить доступные модели',
        'connection_test_no_models': 'Для этих учётных данных нет доступной текстовой модели.',
        'connection_test_ready': 'Готово к проверке',
        'connection_test_running': 'Отправка тестового запроса...',
        'connection_test_success': 'Проверка подключения пройдена',
        'connection_test_failed': 'Проверка подключения не пройдена',
        'connection_test_start': 'Начать проверку',
        'connection_test_retry': 'Проверить снова',
        'connection_test_latency': '{{count}} мс',
        'connection_test_output': 'Ответ модели',
        'connection_test_error_detail': 'Сведения об ошибке',
    },
    'zh-CN.json': {
        'connection_test_button': '测试连接',
        'connection_test_title': '测试连接 - {{account}}',
        'connection_test_hint': '使用当前选中的认证文件发送一次最小真实文本生成请求，测试会消耗少量额度。',
        'connection_test_model': '测试模型',
        'connection_test_select_model': '选择文本模型',
        'connection_test_loading_models': '正在加载可用模型...',
        'connection_test_load_models_failed': '加载可用模型失败',
        'connection_test_no_models': '当前认证文件没有可用于测试的文本模型。',
        'connection_test_ready': '等待开始测试',
        'connection_test_running': '正在发送测试请求...',
        'connection_test_success': '连接测试成功',
        'connection_test_failed': '连接测试失败',
        'connection_test_start': '开始测试',
        'connection_test_retry': '重新测试',
        'connection_test_latency': '{{count}} 毫秒',
        'connection_test_output': '模型输出',
        'connection_test_error_detail': '失败详情',
    },
    'zh-TW.json': {
        'connection_test_button': '測試連線',
        'connection_test_title': '測試連線 - {{account}}',
        'connection_test_hint': '使用目前選取的驗證檔案傳送一次最小真實文字生成請求，測試會消耗少量配額。',
        'connection_test_model': '測試模型',
        'connection_test_select_model': '選擇文字模型',
        'connection_test_loading_models': '正在載入可用模型...',
        'connection_test_load_models_failed': '載入可用模型失敗',
        'connection_test_no_models': '目前驗證檔案沒有可用於測試的文字模型。',
        'connection_test_ready': '等待開始測試',
        'connection_test_running': '正在傳送測試請求...',
        'connection_test_success': '連線測試成功',
        'connection_test_failed': '連線測試失敗',
        'connection_test_start': '開始測試',
        'connection_test_retry': '重新測試',
        'connection_test_latency': '{{count}} 毫秒',
        'connection_test_output': '模型輸出',
        'connection_test_error_detail': '失敗詳情',
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
        'search_placeholder': 'Search name, email, note, or plan',
        'no_search_results': 'No matching quota credentials',
        'no_search_results_desc': 'No quota credential matches the current search.',
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
        'search_placeholder': 'Поиск по имени, почте, заметке или тарифу',
        'no_search_results': 'Подходящие конфигурации квот не найдены',
        'no_search_results_desc': 'Текущему запросу не соответствует ни одна конфигурация квот.',
    },
    'zh-CN.json': {
        'cached_at': '更新于',
        'just_now': '刚刚',
        'minutes_ago': '{{count}} 分钟前',
        'hours_ago': '{{count}} 小时前',
        'days_ago': '{{count}} 天前',
        'search_label': '搜索配额配置文件',
        'search_placeholder': '搜索名称、邮箱、备注或套餐',
        'no_search_results': '没有匹配的配额配置文件',
        'no_search_results_desc': '当前搜索条件下没有可显示的配额配置文件。',
    },
    'zh-TW.json': {
        'cached_at': '更新於',
        'just_now': '剛剛',
        'minutes_ago': '{{count}} 分鐘前',
        'hours_ago': '{{count}} 小時前',
        'days_ago': '{{count}} 天前',
        'search_label': '搜尋配額設定檔',
        'search_placeholder': '搜尋名稱、電子郵件、備註或套餐',
        'no_search_results': '沒有符合的配額設定檔',
        'no_search_results_desc': '目前搜尋條件下沒有可顯示的配額設定檔。',
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
        'plan_x_basic': 'X Basic',
        'plan_x_premium': 'X Premium',
        'plan_x_premium_plus': 'X Premium+',
        'plan_supergrok_lite': 'SuperGrok Lite',
        'plan_paid_unknown': 'Paid (unknown tier)',
        'free_quota': 'Free token quota',
        'free_quota_exhausted': 'Exhausted',
        'free_quota_pending': 'Pending probe',
        'free_quota_window': 'Rolling 24 hours',
    },
    'ru.json': {
        'plan_free': 'Бесплатный',
        'plan_x_basic': 'X Basic',
        'plan_x_premium': 'X Premium',
        'plan_x_premium_plus': 'X Premium+',
        'plan_supergrok_lite': 'SuperGrok Lite',
        'plan_paid_unknown': 'Платный (неизвестный уровень)',
        'free_quota': 'Бесплатная квота токенов',
        'free_quota_exhausted': 'Исчерпана',
        'free_quota_pending': 'Ожидает проверки',
        'free_quota_window': 'Скользящие 24 часа',
    },
    'zh-CN.json': {
        'plan_free': '免费套餐',
        'plan_x_basic': 'X Basic',
        'plan_x_premium': 'X Premium',
        'plan_x_premium_plus': 'X Premium+',
        'plan_supergrok_lite': 'SuperGrok Lite',
        'plan_paid_unknown': '付费版（未知档位）',
        'free_quota': '免费 Token 额度',
        'free_quota_exhausted': '已耗尽',
        'free_quota_pending': '待探测',
        'free_quota_window': '滚动 24 小时',
    },
    'zh-TW.json': {
        'plan_free': '免費套餐',
        'plan_x_basic': 'X Basic',
        'plan_x_premium': 'X Premium',
        'plan_x_premium_plus': 'X Premium+',
        'plan_supergrok_lite': 'SuperGrok Lite',
        'plan_paid_unknown': '付費版（未知級別）',
        'free_quota': '免費 Token 配額',
        'free_quota_exhausted': '已用盡',
        'free_quota_pending': '待探測',
        'free_quota_window': '滾動 24 小時',
    },
}

AUTH_FILES_SEARCH_PLACEHOLDER_KEYS = {
    'en.json': 'Search name, email, note, or plan',
    'ru.json': 'Поиск по имени, почте, заметке или тарифу',
    'zh-CN.json': '搜索名称、邮箱、备注或套餐',
    'zh-TW.json': '搜尋名稱、電子郵件、備註或套餐',
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

PROXY_POOL_NAV_LOCALE_KEYS = {
    'en.json': {'label': 'Proxy Management', 'meta': 'Rotating upstream proxy gateway'},
    'ru.json': {'label': 'Управление прокси', 'meta': 'Шлюз ротации внешних прокси'},
    'zh-CN.json': {'label': '代理管理', 'meta': '多节点轮询与故障转移'},
    'zh-TW.json': {'label': '代理管理', 'meta': '多節點輪詢與故障轉移'},
}

OAUTH_POLICY_NAV_LOCALE_KEYS = {
    'en.json': {'label': 'Account Policy', 'meta': 'Per-plan OAuth account routing rules'},
    'ru.json': {'label': 'Политика аккаунтов', 'meta': 'Правила маршрутизации OAuth по тарифам'},
    'zh-CN.json': {'label': '账号策略', 'meta': '按套餐配置 OAuth 账号路由规则'},
    'zh-TW.json': {'label': '帳號策略', 'meta': '依套餐設定 OAuth 帳號路由規則'},
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


def patch_modal_lifecycle(target: Path) -> None:
    path = target / 'src/components/ui/Modal.tsx'
    text = read(path)
    if 'registerOverlayLayer(titleId)' in text:
        return

    replacements = (
        (
            "import { FOCUSABLE_SELECTOR, lockScroll, unlockScroll } from './scrollLock';\n",
            "import { FOCUSABLE_SELECTOR, lockScroll, unlockScroll } from './scrollLock';\n"
            "import { isTopOverlayLayer, registerOverlayLayer } from './overlayStack';\n",
            'modal overlay stack import',
        ),
        (
            "  onClose: () => void;\n",
            "  onClose: () => void | boolean | Promise<void | boolean>;\n"
            "  onAfterClose?: () => void;\n",
            'modal close contract',
        ),
        (
            "  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);\n",
            "  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);\n"
            "  const closeRequestedRef = useRef(false);\n"
            "  const onCloseRef = useRef(onClose);\n"
            "  const onAfterCloseRef = useRef(onAfterClose);\n",
            'modal close refs',
        ),
    )
    for old, new, label in replacements:
        count = text.count(old)
        if count != 1:
            raise RuntimeError(f'Expected one pattern in {path}, found {count}: {label}')
        text = text.replace(old, new, 1)

    destructure_old = "  onClose,\n  footer,\n"
    if text.count(destructure_old) != 1:
        raise RuntimeError(f'Expected one pattern in {path}, found {text.count(destructure_old)}: modal props')
    text = text.replace(destructure_old, "  onClose,\n  onAfterClose,\n  footer,\n", 1)
    focusable_marker = "  const getFocusableElements = useCallback(() => {\n"
    if text.count(focusable_marker) != 1:
        raise RuntimeError(f'Expected one pattern in {path}, found {text.count(focusable_marker)}: modal callbacks')
    text = text.replace(
        focusable_marker,
        "  useEffect(() => {\n"
        "    onCloseRef.current = onClose;\n"
        "    onAfterCloseRef.current = onAfterClose;\n"
        "  }, [onAfterClose, onClose]);\n\n"
        "  useEffect(() => {\n"
        "    if (!open && !isVisible) return;\n"
        "    return registerOverlayLayer(titleId);\n"
        "  }, [isVisible, open, titleId]);\n\n"
        + focusable_marker,
        1,
    )

    start = text.find('  const startClose = useCallback(')
    end = text.find('  useEffect(() => {\n    return () => {', start)
    if start == -1 or end == -1:
        raise RuntimeError(f'Pattern not found in {path}: modal close lifecycle')
    replacement = (
        "  const finishClose = useCallback(() => {\n"
        "    if (closeTimerRef.current !== null) return;\n"
        "    setIsClosing(true);\n"
        "    closeTimerRef.current = window.setTimeout(() => {\n"
        "      setIsVisible(false);\n"
        "      setIsClosing(false);\n"
        "      closeTimerRef.current = null;\n"
        "      closeRequestedRef.current = false;\n"
        "      onAfterCloseRef.current?.();\n"
        "    }, window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : CLOSE_ANIMATION_DURATION);\n"
        "  }, []);\n\n"
        "  useEffect(() => {\n"
        "    if (open) {\n"
        "      if (closeTimerRef.current !== null) {\n"
        "        window.clearTimeout(closeTimerRef.current);\n"
        "        closeTimerRef.current = null;\n"
        "      }\n"
        "      closeRequestedRef.current = false;\n"
        "      queueMicrotask(() => {\n"
        "        setIsVisible(true);\n"
        "        setIsClosing(false);\n"
        "      });\n"
        "      return;\n"
        "    }\n"
        "    if (isVisible) finishClose();\n"
        "  }, [finishClose, isVisible, open]);\n\n"
        "  const handleClose = useCallback(async () => {\n"
        "    if (closeRequestedRef.current || closeDisabled) return;\n"
        "    closeRequestedRef.current = true;\n"
        "    try {\n"
        "      const shouldClose = await onCloseRef.current();\n"
        "      if (shouldClose === false) {\n"
        "        return;\n"
        "      }\n"
        "    } catch {\n"
        "      // The parent owns the open state; a rejected close keeps the surface open.\n"
        "    } finally {\n"
        "      closeRequestedRef.current = false;\n"
        "    }\n"
        "  }, [closeDisabled]);\n\n"
    )
    text = text[:start] + replacement + text[end:]
    text = text.replace(
        "    const focusTimer = window.setTimeout(() => {\n",
        "    const focusTimer = window.setTimeout(() => {\n"
        "      if (!isTopOverlayLayer(titleId)) return;\n",
        1,
    )
    text = text.replace(
        "  }, [getFocusableElements, open]);\n",
        "  }, [getFocusableElements, open, titleId]);\n",
        1,
    )
    text = text.replace(
        "    const handleKeyDown = (event: KeyboardEvent) => {\n",
        "    const handleKeyDown = (event: KeyboardEvent) => {\n"
        "      if (!isTopOverlayLayer(titleId)) return;\n",
        1,
    )
    text = text.replace(
        "  }, [closeDisabled, getFocusableElements, handleClose, open]);\n",
        "  }, [closeDisabled, getFocusableElements, handleClose, open, titleId]);\n",
        1,
    )
    aria_pattern = re.compile(r'(?P<indent>\s+)role="dialog"\n(?P=indent)aria-modal="true"\n')
    matches = list(aria_pattern.finditer(text))
    if len(matches) != 1:
        raise RuntimeError(f'Expected one pattern in {path}, found {len(matches)}: modal aria state')
    match = matches[0]
    indent = match.group('indent')
    text = text[:match.start()] + (
        f"{indent}role={{open ? 'dialog' : undefined}}\n"
        f"{indent}aria-modal={{open ? 'true' : undefined}}\n"
        f"{indent}aria-hidden={{open ? undefined : true}}\n"
        f"{indent}inert={{open ? undefined : true}}\n"
    ) + text[match.end():]
    write(path, text)


def patch_sheet_lifecycle(target: Path) -> None:
    path = target / 'src/components/ui/Sheet/Sheet.tsx'
    text = read(path)
    if 'registerOverlayLayer(titleId)' in text:
        return

    replacements = (
        (
            "import { FOCUSABLE_SELECTOR, lockScroll, unlockScroll } from '../scrollLock';\n",
            "import { FOCUSABLE_SELECTOR, lockScroll, unlockScroll } from '../scrollLock';\n"
            "import { isTopOverlayLayer, registerOverlayLayer } from '../overlayStack';\n",
            'sheet overlay stack import',
        ),
        (
            "  onClose: () => void;\n",
            "  onClose: () => void | boolean | Promise<void | boolean>;\n"
            "  onAfterClose?: () => void;\n",
            'sheet close contract',
        ),
    )
    for old, new, label in replacements:
        count = text.count(old)
        if count != 1:
            raise RuntimeError(f'Expected one pattern in {path}, found {count}: {label}')
        text = text.replace(old, new, 1)

    if text.count("  onClose,\n  size = 'md',\n") != 1:
        raise RuntimeError(f'Expected one pattern in {path}: sheet props')
    text = text.replace("  onClose,\n  size = 'md',\n", "  onClose,\n  onAfterClose,\n  size = 'md',\n", 1)

    old_refs = "  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);\n"
    if text.count(old_refs) != 1:
        raise RuntimeError(f'Expected one pattern in {path}, found {text.count(old_refs)}: sheet close refs')
    text = text.replace(
        old_refs,
        old_refs
        + "  const closeRequestedRef = useRef(false);\n"
        + "  const onCloseRef = useRef(onClose);\n"
        + "  const onAfterCloseRef = useRef(onAfterClose);\n",
        1,
    )
    focusable_marker = "  const getFocusableElements = useCallback(() => {\n"
    if text.count(focusable_marker) != 1:
        raise RuntimeError(f'Expected one pattern in {path}, found {text.count(focusable_marker)}: sheet callbacks')
    text = text.replace(
        focusable_marker,
        "  useEffect(() => {\n"
        "    onCloseRef.current = onClose;\n"
        "    onAfterCloseRef.current = onAfterClose;\n"
        "  }, [onAfterClose, onClose]);\n\n"
        "  useEffect(() => {\n"
        "    if (!open && !isVisible) return;\n"
        "    return registerOverlayLayer(titleId);\n"
        "  }, [isVisible, open, titleId]);\n\n"
        + focusable_marker,
        1,
    )
    start = text.find('  const startClose = useCallback(')
    end = text.find('  useEffect(() => {\n    return () => {', start)
    if start == -1 or end == -1:
        raise RuntimeError(f'Pattern not found in {path}: sheet close lifecycle')
    replacement = (
        "  const finishClose = useCallback(() => {\n"
        "    if (closeTimerRef.current !== null) return;\n"
        "    setIsClosing(true);\n"
        "    closeTimerRef.current = window.setTimeout(() => {\n"
        "      setIsVisible(false);\n"
        "      setIsClosing(false);\n"
        "      closeTimerRef.current = null;\n"
        "      closeRequestedRef.current = false;\n"
        "      onAfterCloseRef.current?.();\n"
        "    }, window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : CLOSE_ANIMATION_DURATION);\n"
        "  }, []);\n\n"
        "  useEffect(() => {\n"
        "    if (open) {\n"
        "      if (closeTimerRef.current !== null) {\n"
        "        window.clearTimeout(closeTimerRef.current);\n"
        "        closeTimerRef.current = null;\n"
        "      }\n"
        "      closeRequestedRef.current = false;\n"
        "      queueMicrotask(() => {\n"
        "        setIsVisible(true);\n"
        "        setIsClosing(false);\n"
        "      });\n"
        "      return;\n"
        "    }\n"
        "    if (isVisible) finishClose();\n"
        "  }, [finishClose, isVisible, open]);\n\n"
        "  const handleClose = useCallback(async () => {\n"
        "    if (closeRequestedRef.current || closeDisabled) return;\n"
        "    closeRequestedRef.current = true;\n"
        "    if (confirmClose) {\n"
        "      try {\n"
        "        const ok = await confirmClose();\n"
        "        if (ok === false) {\n"
        "          closeRequestedRef.current = false;\n"
        "          return;\n"
        "        }\n"
        "      } catch {\n"
        "        closeRequestedRef.current = false;\n"
        "        return;\n"
        "      }\n"
        "    }\n"
        "    try {\n"
        "      const shouldClose = await onCloseRef.current();\n"
        "      if (shouldClose === false) return;\n"
        "    } finally {\n"
        "      closeRequestedRef.current = false;\n"
        "    }\n"
        "  }, [closeDisabled, confirmClose]);\n\n"
    )
    text = text[:start] + replacement + text[end:]
    text = text.replace(
        "    const t = window.setTimeout(() => {\n",
        "    const t = window.setTimeout(() => {\n"
        "      if (!isTopOverlayLayer(titleId)) return;\n",
        1,
    )
    text = text.replace(
        "  }, [getFocusableElements, open]);\n",
        "  }, [getFocusableElements, open, titleId]);\n",
        1,
    )
    text = text.replace(
        "    const handleKey = (event: KeyboardEvent) => {\n",
        "    const handleKey = (event: KeyboardEvent) => {\n"
        "      if (!isTopOverlayLayer(titleId)) return;\n",
        1,
    )
    text = text.replace(
        "  }, [closeDisabled, getFocusableElements, handleClose, open]);\n",
        "  }, [closeDisabled, getFocusableElements, handleClose, open, titleId]);\n",
        1,
    )
    text = text.replace(
        "        if (closeDisabled) return;\n"
        "        if (e.target === e.currentTarget) handleClose();\n",
        "        if (closeDisabled || !isTopOverlayLayer(titleId)) return;\n"
        "        if (e.target === e.currentTarget) handleClose();\n",
        1,
    )
    aria_pattern = re.compile(r'(?P<indent>\s+)role="dialog"\n(?P=indent)aria-modal="true"\n')
    matches = list(aria_pattern.finditer(text))
    if len(matches) != 1:
        raise RuntimeError(f'Expected one pattern in {path}, found {len(matches)}: sheet aria state')
    match = matches[0]
    indent = match.group('indent')
    text = text[:match.start()] + (
        f"{indent}role={{open ? 'dialog' : undefined}}\n"
        f"{indent}aria-modal={{open ? 'true' : undefined}}\n"
        f"{indent}aria-hidden={{open ? undefined : true}}\n"
        f"{indent}inert={{open ? undefined : true}}\n"
    ) + text[match.end():]
    text = text.replace(
        '    previouslyFocusedRef.current?.focus();\n',
        "    const previouslyFocused = previouslyFocusedRef.current;\n"
        "    if (previouslyFocused?.isConnected) {\n"
        "      previouslyFocused.focus({ preventScroll: true });\n"
        "    }\n",
        1,
    )
    write(path, text)


def patch_overlay_reduced_motion(target: Path) -> None:
    components_path = target / 'src/styles/components.scss'
    components = read(components_path)
    marker = "@media (prefers-reduced-motion: reduce) {\n  .modal-overlay-entering,\n"
    if marker not in components:
        components += (
            "\n@media (prefers-reduced-motion: reduce) {\n"
            "  .modal-overlay-entering,\n"
            "  .modal-overlay-closing,\n"
            "  .modal-entering,\n"
            "  .modal-closing {\n"
            "    animation: none !important;\n"
            "  }\n"
            "}\n"
        )
        write(components_path, components)

    sheet_path = target / 'src/components/ui/Sheet/Sheet.module.scss'
    sheet = read(sheet_path)
    if 'padding: var(--pro-surface-body-padding, 24px);' not in sheet:
        sheet = sheet.replace(
            ".body {\n  flex: 1;\n  overflow-y: auto;\n  padding: 24px;\n",
            ".body {\n  flex: 1;\n  overflow-y: auto;\n  padding: var(--pro-surface-body-padding, 24px);\n",
            1,
        )
        sheet = sheet.replace(
            "  .body {\n    padding: 16px;\n",
            "  .body {\n    padding: var(--pro-surface-body-padding, 16px);\n",
            1,
        )
    marker = "@media (prefers-reduced-motion: reduce) {\n  .overlay,\n"
    if marker not in sheet:
        sheet += (
            "\n@media (prefers-reduced-motion: reduce) {\n"
            "  .overlay,\n"
            "  .content {\n"
            "    transition: none !important;\n"
            "  }\n"
            "}\n"
        )
        write(sheet_path, sheet)


def patch_confirmation_queue(target: Path) -> None:
    store_path = target / 'src/stores/useNotificationStore.ts'
    text = read(store_path)
    if 'confirmationQueue:' not in text:
        text = text.replace(
            "interface ConfirmationOptions {\n",
            "interface ConfirmationOptions {\n  dedupeKey?: string;\n",
            1,
        )
        text = text.replace(
            "  confirmation: {\n    isOpen: boolean;\n    isLoading: boolean;\n    options: ConfirmationOptions | null;\n  };\n",
            "  confirmation: {\n"
            "    id: string;\n"
            "    isOpen: boolean;\n"
            "    isLoading: boolean;\n"
            "    options: ConfirmationOptions | null;\n"
            "  };\n"
            "  confirmationQueue: Array<{ id: string; options: ConfirmationOptions }>;\n",
            1,
        )
        text = text.replace(
            "  hideConfirmation: () => void;\n",
            "  hideConfirmation: () => void;\n  advanceConfirmation: () => void;\n",
            1,
        )
        text = text.replace(
            "  confirmation: {\n    isOpen: false,\n    isLoading: false,\n    options: null,\n  },\n",
            "  confirmation: {\n"
            "    id: '',\n"
            "    isOpen: false,\n"
            "    isLoading: false,\n"
            "    options: null,\n"
            "  },\n"
            "  confirmationQueue: [],\n",
            1,
        )
        start = text.find('  showConfirmation: (options) => {')
        end = text.find('  setConfirmationLoading: (loading) => {', start)
        if start == -1 or end == -1:
            raise RuntimeError(f'Pattern not found in {store_path}: confirmation actions')
        actions = (
            "  showConfirmation: (options) => {\n"
            "    set((state) => {\n"
            "      const dedupeKey = options.dedupeKey\n"
            "        ?? (typeof options.message === 'string'\n"
            "          ? `${options.title ?? ''}|${options.confirmText ?? ''}|${options.message}`\n"
            "          : undefined);\n"
            "      const currentKey = state.confirmation.options?.dedupeKey\n"
            "        ?? (state.confirmation.options\n"
            "          ? `${state.confirmation.options.title ?? ''}|${state.confirmation.options.confirmText ?? ''}|${typeof state.confirmation.options.message === 'string' ? state.confirmation.options.message : ''}`\n"
            "          : '');\n"
            "      if (dedupeKey && state.confirmation.id && currentKey === dedupeKey) return state;\n"
            "      if (dedupeKey && state.confirmationQueue.some((item) => item.options.dedupeKey === dedupeKey)) return state;\n"
            "      const item = { id: generateId(), options: { ...options, dedupeKey } };\n"
            "      if (state.confirmation.id) {\n"
            "        return { confirmationQueue: [...state.confirmationQueue, item] };\n"
            "      }\n"
            "      return {\n"
            "        confirmation: { id: item.id, isOpen: true, isLoading: false, options: item.options },\n"
            "      };\n"
            "    });\n"
            "  },\n\n"
            "  hideConfirmation: () => {\n"
            "    set((state) => ({\n"
            "      confirmation: { ...state.confirmation, isOpen: false, isLoading: false },\n"
            "    }));\n"
            "  },\n\n"
            "  advanceConfirmation: () => {\n"
            "    set((state) => {\n"
            "      if (state.confirmation.isOpen) return state;\n"
            "      const [next, ...remaining] = state.confirmationQueue;\n"
            "      return {\n"
            "        confirmation: next\n"
            "          ? { id: next.id, isOpen: true, isLoading: false, options: next.options }\n"
            "          : { id: '', isOpen: false, isLoading: false, options: null },\n"
            "        confirmationQueue: remaining,\n"
            "      };\n"
            "    });\n"
            "  },\n\n"
        )
        text = text[:start] + actions + text[end:]
        write(store_path, text)

    modal_path = target / 'src/components/common/ConfirmationModal.tsx'
    text = read(modal_path)
    if 'const submittingRef = useRef(false);' not in text:
        text = text.replace(
            "import { useTranslation } from 'react-i18next';\n",
            "import { useEffect, useRef } from 'react';\n"
            "import { useTranslation } from 'react-i18next';\n",
            1,
        )
        text = text.replace(
            "  const setConfirmationLoading = useNotificationStore((state) => state.setConfirmationLoading);\n",
            "  const setConfirmationLoading = useNotificationStore((state) => state.setConfirmationLoading);\n"
            "  const advanceConfirmation = useNotificationStore((state) => state.advanceConfirmation);\n",
            1,
        )
        text = text.replace(
            "  const { isOpen, isLoading, options } = confirmation;\n",
            "  const { id, isOpen, isLoading, options } = confirmation;\n"
            "  const submittingRef = useRef(false);\n\n"
            "  useEffect(() => {\n"
            "    submittingRef.current = false;\n"
            "  }, [id]);\n",
            1,
        )
        text = text.replace(
            "  if (!isOpen || !options) {\n    return null;\n  }\n",
            "  if (!options) {\n    return null;\n  }\n",
            1,
        )
        text = text.replace(
            "  const handleConfirm = async () => {\n    try {\n",
            "  const handleConfirm = async () => {\n"
            "    if (submittingRef.current || isLoading) return;\n"
            "    submittingRef.current = true;\n"
            "    try {\n",
            1,
        )
        text = text.replace(
            "    } finally {\n      setConfirmationLoading(false);\n    }\n",
            "    } finally {\n"
            "      submittingRef.current = false;\n"
            "      setConfirmationLoading(false);\n"
            "    }\n",
            1,
        )
        text = text.replace(
            "    <Modal open={isOpen} onClose={handleCancel} title={title} closeDisabled={isLoading}>\n",
            "    <Modal\n"
            "      open={isOpen}\n"
            "      onClose={handleCancel}\n"
            "      onAfterClose={advanceConfirmation}\n"
            "      title={title}\n"
            "      closeDisabled={isLoading}\n"
            "    >\n",
            1,
        )
        write(modal_path, text)


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
    text = read(client)
    if 'this.connectionGeneration += 1;' not in text:
        current = (
            "    this.apiBase = computeApiUrl(config.apiBase);\n"
            "    this.managementKey = config.managementKey;\n"
            "\n"
            "    if (config.timeout) {\n"
        )
        current_replacement = (
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
            "    if (config.timeout) {\n"
        )
        legacy = (
            "    if (connectionChanged) {\n"
            "      this.runtimeKind = 'unknown';\n"
            "    }\n"
        )
        legacy_replacement = (
            "    if (connectionChanged) {\n"
            "      this.connectionAbortController.abort();\n"
            "      this.connectionAbortController = new AbortController();\n"
            "      this.connectionGeneration += 1;\n"
            "      this.runtimeKind = 'unknown';\n"
            "    }\n"
        )
        matches = text.count(current) + text.count(legacy)
        if matches != 1:
            raise RuntimeError(
                f'Expected one supported connection-change shape in {client}, found {matches}'
            )
        if current in text:
            write(client, text.replace(current, current_replacement, 1))
        else:
            write(client, text.replace(legacy, legacy_replacement, 1))
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


def patch_api_call_executor_contract(target: Path) -> None:
    path = target / 'src/services/api/apiCall.ts'
    replace_once(
        path,
        "  data?: string;\n}",
        "  data?: string;\n  useExecutor?: boolean;\n  use_executor?: boolean;\n}",
    )
    replace_once(
        path,
        "export interface ApiCallResult<T = unknown> {\n  statusCode: number;\n",
        "export interface ApiCallResult<T = unknown> {\n  statusCode: number;\n  hasStatusCode: boolean;\n",
    )
    replace_once(
        path,
        "    const statusCode = Number(response?.status_code ?? 0);\n"
        "    const header = (response?.header ?? {}) as Record<string, string[]>;\n",
        "    const rawStatusCode = response?.status_code ?? response?.statusCode;\n"
        "    const hasStatusCode =\n"
        "      rawStatusCode !== undefined &&\n"
        "      rawStatusCode !== null &&\n"
        "      String(rawStatusCode).trim() !== '';\n"
        "    const statusCode = Number(rawStatusCode ?? 0);\n"
        "    const header = (response?.header ?? response?.headers ?? {}) as Record<string, string[]>;\n",
    )
    replace_once(
        path,
        "    return {\n      statusCode,\n      header,\n",
        "    return {\n      statusCode,\n      hasStatusCode,\n      header,\n",
    )


def patch_codex_account_id_resolver(target: Path) -> None:
    path = target / 'src/utils/quota/resolvers.ts'
    insert_once(
        path,
        'export function extractCodexChatgptAccountId(value: unknown): string | null {\n',
        "const resolveAccountIdCandidate = (value: unknown): string | null => {\n"
        "  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;\n"
        "  const record = value as Record<string, unknown>;\n"
        "  const candidates = [\n"
        "    record.chatgpt_account_id,\n"
        "    record.chatgptAccountId,\n"
        "    record.account_id,\n"
        "    record.accountId,\n"
        "  ];\n"
        "  for (const candidate of candidates) {\n"
        "    const accountId = normalizeStringValue(candidate);\n"
        "    if (accountId) return accountId;\n"
        "  }\n"
        "  return null;\n"
        "};\n\n"
        'export function extractCodexChatgptAccountId(value: unknown): string | null {\n',
        'const resolveAccountIdCandidate',
    )
    replace_once(
        path,
        "export function extractCodexChatgptAccountId(value: unknown): string | null {\n"
        "  const payload = parseIdTokenPayload(value);\n"
        "  if (!payload) return null;\n"
        "  return normalizeStringValue(payload.chatgpt_account_id ?? payload.chatgptAccountId);\n"
        "}\n",
        "export function extractCodexChatgptAccountId(value: unknown): string | null {\n"
        "  const direct = resolveAccountIdCandidate(value);\n"
        "  if (direct) return direct;\n\n"
        "  const payload = parseIdTokenPayload(value);\n"
        "  if (!payload) return null;\n"
        "  return resolveAccountIdCandidate(payload);\n"
        "}\n",
    )
    replace_once(
        path,
        "  const candidates = [file.id_token, metadata?.id_token, attributes?.id_token];\n",
        "  const candidates = [\n"
        "    file,\n"
        "    metadata,\n"
        "    attributes,\n"
        "    file.id_token,\n"
        "    metadata?.id_token,\n"
        "    attributes?.id_token,\n"
        "  ];\n",
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
    insert_once(
        card_path,
        '    onShowModels,\n    onDownload,\n',
        '    onShowModels,\n    onShowUsage,\n    onDownload,\n',
        '    onShowUsage,\n',
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
        "import { useProSurfaceState } from '@/pro/shared/useProSurfaceState';\n"
        "import type { AuthFileItem } from '@/types';\n",
        "useProSurfaceState } from '@/pro/shared/useProSurfaceState'",
    )
    page_text = read(page_path)
    if 'const [accountUsageFile, setAccountUsageFileState]' not in page_text:
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
                        + "  const [accountUsageFile, setAccountUsageFileState] = useState<AuthFileItem | null>(null);\n"
                        + "  const { activeSurface: activeAuthSurface, openSurface: openAuthSurface, closeSurface: closeAuthSurface } = useProSurfaceState<'usage' | 'connection-test'>();\n"
                        + "  const setAccountUsageFile = useCallback((file: AuthFileItem | null) => {\n"
                        + "    setAccountUsageFileState(file);\n"
                        + "    if (file) openAuthSurface('usage');\n"
                        + "    else if (activeAuthSurface === 'usage') closeAuthSurface();\n"
                        + "  }, [activeAuthSurface, closeAuthSurface, openAuthSurface]);\n",
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
        "      <AccountUsageModal file={activeAuthSurface === 'usage' ? accountUsageFile : null} onClose={() => setAccountUsageFile(null)} />\n\n"
        "      <AuthFileModelsModal\n",
        "<AccountUsageModal file={activeAuthSurface === 'usage' ? accountUsageFile : null}",
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


def patch_auth_file_connection_test(target: Path) -> None:
    api_path = target / 'src/services/api/authFiles.ts'
    card_path = target / 'src/features/authFiles/components/AuthFileCard.tsx'
    page_path = auth_files_page_path(target)

    insert_once(
        api_path,
        "type AuthFileStatusResponse = { status: string; disabled: boolean };\n",
        "type AuthFileStatusResponse = { status: string; disabled: boolean };\n"
        "export type AuthFileConnectionTestResponse = {\n"
        "  success: boolean;\n"
        "  model?: string;\n"
        "  latency_ms: number;\n"
        "  output?: string;\n"
        "  error?: string;\n"
        "  error_code?: string;\n"
        "  http_status?: number;\n"
        "};\n"
        "export type AuthFileConnectionTestRequest = {\n"
        "  name: string;\n"
        "  auth_index?: string;\n"
        "  model: string;\n"
        "};\n",
        'export type AuthFileConnectionTestResponse',
    )
    insert_once(
        api_path,
        "  uploadFiles: async (files: File[]): Promise<AuthFileBatchUploadResult> => {\n",
        "  testConnection: (payload: AuthFileConnectionTestRequest, signal?: AbortSignal) =>\n"
        "    apiClient.post<AuthFileConnectionTestResponse>('/auth-files/test', payload, { signal }),\n\n"
        "  uploadFiles: async (files: File[]): Promise<AuthFileBatchUploadResult> => {\n",
        "testConnection: (payload: AuthFileConnectionTestRequest, signal?: AbortSignal)",
    )

    replace_once(
        api_path,
        "  async getModelsForAuthFile(\n"
        "    name: string\n"
        "  ): Promise<{ id: string; display_name?: string; type?: string; owned_by?: string }[]> {\n"
        "    const data = await apiClient.get<Record<string, unknown>>(\n"
        "      `/auth-files/models?name=${encodeURIComponent(name)}`\n"
        "    );\n",
        "  async getModelsForAuthFile(\n"
        "    name: string,\n"
        "    authIndex?: string\n"
        "  ): Promise<{ id: string; display_name?: string; type?: string; owned_by?: string }[]> {\n"
        "    const normalizedAuthIndex = authIndex?.trim();\n"
        "    const authIndexQuery = normalizedAuthIndex\n"
        "      ? `&auth_index=${encodeURIComponent(normalizedAuthIndex)}`\n"
        "      : '';\n"
        "    const data = await apiClient.get<Record<string, unknown>>(\n"
        "      `/auth-files/models?name=${encodeURIComponent(name)}${authIndexQuery}`\n"
        "    );\n",
    )

    replace_once(
        card_path,
        "  IconModelCluster,\n  IconRefreshCw,\n",
        "  IconModelCluster,\n  IconNetwork,\n  IconRefreshCw,\n",
    )
    replace_once(
        card_path,
        "  onShowUsage: (file: AuthFileItem) => void;\n  onDownload: (name: string) => void;\n",
        "  onShowUsage: (file: AuthFileItem) => void;\n"
        "  onTestConnection: (file: AuthFileItem) => void;\n"
        "  onDownload: (name: string) => void;\n",
    )
    replace_once(
        card_path,
        "    onShowUsage,\n    onDownload,\n",
        "    onShowUsage,\n    onTestConnection,\n    onDownload,\n",
    )
    insert_once(
        card_path,
        "              {showManualRefreshButton && (\n",
        "              <Button\n"
        "                variant=\"secondary\"\n"
        "                size=\"sm\"\n"
        "                onClick={() => onTestConnection(file)}\n"
        "                className={styles.iconButton}\n"
        "                title={t('auth_files.connection_test_button')}\n"
        "                disabled={\n"
        "                  disableControls ||\n"
        "                  statusUpdating[file.name] === true ||\n"
        "                  isManualRefreshing\n"
        "                }\n"
        "              >\n"
        "                <IconNetwork size={15} />\n"
        "              </Button>\n"
        "              {showManualRefreshButton && (\n",
        "onClick={() => onTestConnection(file)}",
    )

    insert_once(
        page_path,
        "import { AccountUsageModal } from '@/pro/modules/monitoring';\n",
        "import { AccountUsageModal } from '@/pro/modules/monitoring';\n"
        "import { AuthFileConnectionTestModal } from '@/pro/authFiles/AuthFileConnectionTestModal';\n",
        "AuthFileConnectionTestModal } from '@/pro/authFiles/AuthFileConnectionTestModal'",
    )
    insert_once(
        page_path,
        "  const setAccountUsageFile = useCallback((file: AuthFileItem | null) => {\n"
        "    setAccountUsageFileState(file);\n"
        "    if (file) openAuthSurface('usage');\n"
        "    else if (activeAuthSurface === 'usage') closeAuthSurface();\n"
        "  }, [activeAuthSurface, closeAuthSurface, openAuthSurface]);\n",
        "  const setAccountUsageFile = useCallback((file: AuthFileItem | null) => {\n"
        "    setAccountUsageFileState(file);\n"
        "    if (file) openAuthSurface('usage');\n"
        "    else if (activeAuthSurface === 'usage') closeAuthSurface();\n"
        "  }, [activeAuthSurface, closeAuthSurface, openAuthSurface]);\n"
        "  const [connectionTestFile, setConnectionTestFileState] = useState<AuthFileItem | null>(null);\n"
        "  const setConnectionTestFile = useCallback((file: AuthFileItem | null) => {\n"
        "    setConnectionTestFileState(file);\n"
        "    if (file) openAuthSurface('connection-test');\n"
        "    else if (activeAuthSurface === 'connection-test') closeAuthSurface();\n"
        "  }, [activeAuthSurface, closeAuthSurface, openAuthSurface]);\n",
        'const [connectionTestFile, setConnectionTestFileState]',
    )
    insert_once(
        page_path,
        "                onShowUsage={setAccountUsageFile}\n",
        "                onShowUsage={setAccountUsageFile}\n"
        "                onTestConnection={setConnectionTestFile}\n",
        "onTestConnection={setConnectionTestFile}",
    )
    insert_once(
        page_path,
        "      <AccountUsageModal file={activeAuthSurface === 'usage' ? accountUsageFile : null} onClose={() => setAccountUsageFile(null)} />\n",
        "      <AuthFileConnectionTestModal\n"
        "        file={activeAuthSurface === 'connection-test' ? connectionTestFile : null}\n"
        "        onClose={() => setConnectionTestFile(null)}\n"
        "      />\n\n"
        "      <AccountUsageModal file={activeAuthSurface === 'usage' ? accountUsageFile : null} onClose={() => setAccountUsageFile(null)} />\n",
        '<AuthFileConnectionTestModal',
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
        "  quotaExceeded?: QuotaExceededConfig;\n  requestLog?: boolean;\n",
        "  quotaExceeded?: QuotaExceededConfig;\n  usageStatisticsEnabled?: boolean;\n  requestLog?: boolean;\n",
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
    replace_once(
        auth_files_path,
        "type AuthFileStatusResponse = { status: string; disabled: boolean };\n",
        "type AuthFileStatusResponse = { status: string; disabled: boolean };\ntype AuthFilePatchPayload = { name: string; disabled?: boolean; [key: string]: unknown };\n",
    )
    insert_once(
        auth_files_path,
        "export const authFilesApi = {\n",
        "const AUTH_FILES_LIST_CACHE_TTL_MS = 2000;\nlet authFilesListCache: { expiresAt: number; response: AuthFilesResponse } | null = null;\nlet authFilesListRequest: Promise<AuthFilesResponse> | null = null;\nlet authFilesListVersion = 0;\n\nconst cloneAuthFilesResponse = (response: AuthFilesResponse): AuthFilesResponse => ({\n  ...response,\n  files: Array.isArray(response.files) ? [...response.files] : [],\n});\n\nconst invalidateAuthFilesListCache = () => {\n  authFilesListVersion += 1;\n  authFilesListCache = null;\n  authFilesListRequest = null;\n};\n\nconst fetchAuthFilesList = async (): Promise<AuthFilesResponse> => {\n  const now = Date.now();\n  if (authFilesListCache && authFilesListCache.expiresAt > now) {\n    return cloneAuthFilesResponse(authFilesListCache.response);\n  }\n  if (!authFilesListRequest) {\n    const requestVersion = authFilesListVersion;\n    authFilesListRequest = apiClient.get<AuthFilesResponse>('/auth-files')\n      .then(normalizeAuthFilesResponse)\n      .then((response) => {\n        if (requestVersion === authFilesListVersion) {\n          authFilesListCache = {\n            expiresAt: Date.now() + AUTH_FILES_LIST_CACHE_TTL_MS,\n            response: cloneAuthFilesResponse(response),\n          };\n        }\n        return response;\n      })\n      .finally(() => {\n        if (requestVersion === authFilesListVersion) {\n          authFilesListRequest = null;\n        }\n      });\n  }\n  return cloneAuthFilesResponse(await authFilesListRequest);\n};\n\nexport const authFilesApi = {\n",
        "AUTH_FILES_LIST_CACHE_TTL_MS",
    )
    list_replacement = (
        "  list: fetchAuthFilesList,\n\n  patchFile: async (payload: AuthFilePatchPayload) => {\n    const response = await apiClient.patch<AuthFileStatusResponse>('/auth-files', payload);\n    invalidateAuthFilesListCache();\n    return response;\n  },\n\n  setStatus: async (name: string, disabled: boolean) => {\n    const response = await apiClient.patch<AuthFileStatusResponse>('/auth-files/status', { name, disabled });\n    invalidateAuthFilesListCache();\n    return response;\n  },\n"
    )
    replace_once(
        auth_files_path,
        "  list: async () =>\n"
        "    normalizeAuthFilesResponse(await apiClient.get<AuthFilesResponse>('/auth-files')),\n\n"
        "  setStatus: (name: string, disabled: boolean) =>\n"
        "    apiClient.patch<AuthFileStatusResponse>('/auth-files/status', { name, disabled }),\n\n",
        list_replacement,
    )
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
    generated = {}
    for locale_name in ('en.json', 'ru.json', 'zh-CN.json', 'zh-TW.json'):
        additions = json.loads(json.dumps(monitoring.get(locale_name, {})))
        data = additions
        proxy_pool_nav = PROXY_POOL_NAV_LOCALE_KEYS.get(
            locale_name,
            PROXY_POOL_NAV_LOCALE_KEYS['en.json'],
        )
        data.setdefault('nav', {})['proxy_pool'] = proxy_pool_nav['label']
        oauth_policy_nav = OAUTH_POLICY_NAV_LOCALE_KEYS.get(
            locale_name,
            OAUTH_POLICY_NAV_LOCALE_KEYS['en.json'],
        )
        data.setdefault('nav', {})['oauth_policy'] = oauth_policy_nav['label']
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
        data.setdefault('nav_meta', {})['oauth_policy'] = oauth_policy_nav['meta']
        data['proxy_pool'] = additions.get(
            'proxy_pool',
            monitoring.get('en.json', {}).get('proxy_pool', {}),
        )
        data['oauth_policy'] = additions.get(
            'oauth_policy',
            monitoring.get('en.json', {}).get('oauth_policy', {}),
        )
        data.setdefault('quota_management', {}).update(QUOTA_LOCALE_KEYS.get(locale_name, {}))
        gemini_cli_locale = GEMINI_CLI_LOCALE_KEYS.get(
            locale_name,
            GEMINI_CLI_LOCALE_KEYS['en.json'],
        )
        data.setdefault('auth_files', {})['filter_gemini-cli'] = gemini_cli_locale['auth_filter']
        data.setdefault('auth_files', {})['search_placeholder'] = AUTH_FILES_SEARCH_PLACEHOLDER_KEYS.get(
            locale_name,
            AUTH_FILES_SEARCH_PLACEHOLDER_KEYS['en.json'],
        )
        data.setdefault('auth_files', {})['sort_plan_desc'] = AUTH_FILES_PLAN_SORT_LABEL_KEYS.get(
            locale_name,
            AUTH_FILES_PLAN_SORT_LABEL_KEYS['en.json'],
        )
        data.setdefault('auth_files', {})['sort_quota_desc'] = AUTH_FILES_QUOTA_SORT_LABEL_KEYS.get(
            locale_name,
            AUTH_FILES_QUOTA_SORT_LABEL_KEYS['en.json'],
        )
        data.setdefault('auth_files', {})['selected_count'] = AUTH_FILES_SELECTED_COUNT_LABEL_KEYS.get(
            locale_name,
            AUTH_FILES_SELECTED_COUNT_LABEL_KEYS['en.json'],
        )
        data.setdefault('auth_files', {}).update(
            AUTH_FILE_CONNECTION_TEST_LOCALE_KEYS.get(
                locale_name,
                AUTH_FILE_CONNECTION_TEST_LOCALE_KEYS['en.json'],
            )
        )
        data.setdefault('gemini_cli_quota', {}).update(gemini_cli_locale['quota'])
        data.setdefault('xai_quota', {}).update(
            XAI_QUOTA_LOCALE_KEYS.get(locale_name, XAI_QUOTA_LOCALE_KEYS['en.json'])
        )
        data.setdefault('system_info', {}).update(
            MANAGEMENT_UPDATE_LOCALE_KEYS.get(
                locale_name,
                MANAGEMENT_UPDATE_LOCALE_KEYS['en.json'],
            )
        )
        generated[locale_name.removesuffix('.json')] = data
    write(
        target / 'src/pro/locales.generated.json',
        json.dumps(generated, ensure_ascii=False, indent=2) + '\n',
    )


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
        "export interface GeminiCliQuotaBucketState {\n  id: string;\n  label: string;\n  remainingFraction: number | null;\n  remainingAmount: number | null;\n  resetTime: string | undefined;\n  tokenType: string | null;\n  modelIds?: string[];\n  resetAtMs?: number | null;\n  periodHours?: number | null;\n}\n\nexport interface GeminiCliQuotaState {\n  status: 'idle' | 'loading' | 'success' | 'error';\n  buckets: GeminiCliQuotaBucketState[];\n  projectId?: string;\n  project_id?: string;\n  tierLabel?: string | null;\n  tierId?: string | null;\n  creditBalance?: number | null;\n  quotaProviderSnapshot?: boolean;\n  error?: string;\n  errorStatus?: number;\n}\n\nexport interface CodexQuotaWindow",
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
        "  planType?: 'free' | 'supergrok' | 'x-basic' | 'x-premium' | 'x-premium-plus' | 'supergrok-heavy' | 'supergrok-lite' | 'paid' | 'paid-unknown';\n",
    )
    _ensure_interface_field(path, 'XaiBillingSummary', '  freeQuota?: XaiFreeQuotaSummary;')
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
    insert_once(path, "import { readQuotaUiState, writeQuotaUiState } from './uiState';\n", "import { readQuotaUiState, writeQuotaUiState } from './uiState';\nimport { buildQuotaSearchValues, matchesQuotaSearch } from '@/pro/modules/quota';\n", 'matchesQuotaSearch')
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
    insert_once(path, "        {error && (\n", "        <Input\n          type=\"search\"\n          value={search}\n          onChange={(event) => { setSearch(event.target.value); setPage(1); }}\n          placeholder={t('quota_management.search_placeholder')}\n          aria-label={t('quota_management.search_label')}\n          rightElement={<IconSearch size={18} />}\n        />\n\n        {error && (\n", 'rightElement={<IconSearch')


def patch_quota_cards_latest(target: Path) -> None:
    card_path = target / 'src/features/quota/components/QuotaCard.tsx'
    insert_once(card_path, "import { resolveQuotaErrorMessage } from '@/utils/quota';\n", "import { resolveQuotaErrorMessage } from '@/utils/quota';\nimport { QuotaCachedTime } from '@/pro/modules/quota';\n", 'import { QuotaCachedTime }')
    replace_once(card_path, "        ) : quota ? (\n          <adapter.Body quota={quota} classes={quotaClasses} />\n        ) : (", "        ) : quota ? (\n          <>\n            <adapter.Body quota={quota} classes={quotaClasses} />\n            <QuotaCachedTime quotaStatus={status} cachedAt={quota.cachedAt} />\n          </>\n        ) : (")

    auth_path = target / 'src/features/authFiles/components/AuthFileQuotaSection.tsx'
    insert_once(auth_path, "import { bindQuotaClasses } from '@/features/quota/types';\n", "import { bindQuotaClasses } from '@/features/quota/types';\nimport { QuotaCachedTime } from '@/pro/modules/quota';\n", 'import { QuotaCachedTime }')
    replace_once(auth_path, "      ) : quota ? (\n        <adapter.Body quota={quota} classes={compactQuotaClasses} />\n      ) : (", "      ) : quota ? (\n        <>\n          <adapter.Body quota={quota} classes={compactQuotaClasses} />\n          <QuotaCachedTime quotaStatus={quotaStatus} cachedAt={quota.cachedAt} />\n        </>\n      ) : (")


def patch_quota_success_timestamps(target: Path) -> None:
    actions_path = target / 'src/features/quota/hooks/useQuotaActions.ts'
    insert_once(
        actions_path,
        "import { getStatusFromError } from '@/utils/quota';\n",
        "import { withQuotaCachedAt } from '@/pro/shared/quotaState';\n"
        "import { getStatusFromError } from '@/utils/quota';\n",
        "from '@/pro/shared/quotaState'",
    )
    actions_text = read(actions_path)
    old_action = 'adapter.buildSuccessState(data)'
    new_action = 'withQuotaCachedAt(adapter.buildSuccessState(data))'
    if new_action not in actions_text:
        if actions_text.count(old_action) != 2:
            raise RuntimeError(
                f'Expected two quota success commits in {actions_path}, '
                f'found {actions_text.count(old_action)}'
            )
        write(actions_path, actions_text.replace(old_action, new_action))

    batch_path = target / 'src/features/quota/hooks/useQuotaBatchLoader.ts'
    insert_once(
        batch_path,
        "import { getStatusFromError } from '@/utils/quota';\n",
        "import { withQuotaCachedAt } from '@/pro/shared/quotaState';\n"
        "import { getStatusFromError } from '@/utils/quota';\n",
        "from '@/pro/shared/quotaState'",
    )
    replace_once(
        batch_path,
        'adapter.buildSuccessState(result.data)',
        'withQuotaCachedAt(adapter.buildSuccessState(result.data))',
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
    patch_modal_lifecycle(target)
    patch_sheet_lifecycle(target)
    patch_overlay_reduced_motion(target)
    patch_confirmation_queue(target)
    patch_modal_scroll_lock(target)
    patch_modal_content_scrollbar_layout(target)
    patch_routes(target)
    patch_layout(target)
    patch_icons(target)
    patch_quota_types_latest(target)
    patch_quota_store(target)
    patch_quota_constants(target)
    patch_api_call_executor_contract(target)
    patch_codex_account_id_resolver(target)
    patch_quota_provider_model_latest(target)
    patch_quota_success_timestamps(target)
    patch_antigravity_quota_builders(target)
    patch_quota_page_latest(target)
    patch_quota_cards_latest(target)
    patch_auth_files_page_search_latest(target)
    patch_auth_files_page_sorting_latest(target)
    patch_auth_files_gemini_quota_latest(target)
    patch_auth_files_runtime_state(target)
    patch_account_usage_feature(target)
    patch_auth_file_connection_test(target)
    patch_management_update_check(target)
    patch_api_client_connection_isolation(target)
    patch_supporting_api_and_types(target)
    patch_locales(target)
    flush_writes()
    print(f'OK: CPA-Management customization applied to {target}')


if __name__ == '__main__':
    main()
