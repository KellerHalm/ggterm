# Конфигурация ggterm

Конфигурация собирается слоями: встроенные значения → `config.toml` →
`init.lua` → явные флаги командной строки. Флаги всегда побеждают.

Путь по умолчанию — `os.UserConfigDir()/ggterm`:

- Windows: `%AppData%\ggterm`
- Linux: `~/.config/ggterm`
- macOS: `~/Library/Application Support/ggterm`

Файлы: `config.toml` и `init.lua` (оба необязательны). `--config PATH`
читает указанный файл; `.lua` загружается как Lua-конфиг без TOML. Рядом с
явно указанным TOML всё равно ищется `init.lua`. `--no-config` отключает оба
файла.

## config.toml

```toml
[general]
session = "default"
shell = "pwsh -NoLogo"
cols = 120
rows = 36
scrollback = 20000
restore = true
padding = 8
decorated = true
font_size = 16            # краткая форма font.size
font_family = "Go Mono"   # краткая форма font.family

[font]
family = "Go Mono"
path = "C:/fonts/JetBrainsMono-Regular.ttf"
bold = "C:/fonts/JetBrainsMono-Bold.ttf"
italic = "C:/fonts/JetBrainsMono-Italic.ttf"
bold_italic = "C:/fonts/JetBrainsMono-BoldItalic.ttf"
fallback = ["Microsoft YaHei"]
size = 16
ligatures = true

[window]
transparent = false

[theme]
name = "tokyo-night"
background = "#0b1020"
palette = ["#15161e", "#f7768e", "#9ece6a", "#e0af68",
           "#7aa2f7", "#bb9af7", "#7dcfff", "#a9b1d6",
           "#414868", "#f7768e", "#9ece6a", "#e0af68",
           "#7aa2f7", "#bb9af7", "#7dcfff", "#c0caf5"]

[keys]
prefix = "ctrl+b"
replace_defaults = false
unbind = ["q"]

[[keys.bindings]]
key = "%"
action = "split_vertical"

[[keys.bindings]]
key = "1"
action = "select_tab"
arg = 1

[plugins]
dir = "plugins"
enabled = []
disabled = []
```

| Секция | Ключи |
|---|---|
| `general` | `session`, `shell`, `cols`, `rows`, `scrollback`, `restore`, `padding`, `decorated`, `font_size`, `font_family` |
| `font` | `family`, `path`, `bold`, `italic`, `bold_italic`, `size`, `fallback`, `ligatures` |
| `window` | `transparent` |
| `theme` | `name`, `file`, `background`, `foreground`, `cursor`, `bar_background`, `bar_foreground`, `tab_active_background`, `tab_active_foreground`, `border`, `border_active`, `overlay_background`, `palette` |
| `keys` | `prefix`, `replace_defaults`, `bindings` (`key`, `action`, `arg`, `desc`), `unbind` |
| `plugins` | `dir`, `enabled`, `disabled` |

Неизвестные ключи считаются ошибкой конфигурации. `theme.name` и `theme.file`
взаимоисключающие. Пути к файлам шрифтов и темам разрешаются относительно
каталога конфига.

`transparent = true` включает прозрачное окно; степень задаётся alpha-каналом
фона темы, например `background = "#0f1117d0"`. Режим окна применяется при
запуске. `session`, `shell`, `cols`, `rows`, `scrollback` и `restore` тоже
требуют перезапуска.

### Шрифт

`font.family` — имя встроенного шрифта (`Go Mono`) или семейство из системных
каталогов. Поиск идёт по именам файлов, поэтому нестандартные названия
подключайте через `font.path`. Bold/italic/bold-italic ищутся рядом с основным
файлом и в системных каталогах; если их нет, включается синтетическое
начертание. `.ttc`/`.otc` используют первую гарнитуру. Дополнительные каталоги
задаются в `GGTERM_FONT_DIRS` через разделитель путей ОС.

Для CJK укажите `font.fallback` — путь к файлу или имя семейства, совпадающее с
именем файла.

### Темы

Встроенные: `ggterm-dark`, `dracula`, `nord`, `gruvbox-dark`,
`solarized-dark`, `tokyo-night`, `catppuccin-mocha`.

`theme.file` принимает:

- TOML с полями из таблицы `theme`;
- base16 JSON/YAML (`base00`..`base0F`);
- iTerm2 `.itermcolors`.

Цвета — `#RGB`, `#RRGGBB` или `#RRGGBBAA`.

### Биндинги

Клавиши записываются как `c`, `ctrl+a`, `shift+slash`, `alt+enter`, `f5`,
`up`, `%`, `?`, `prefix+c`. Биндинг всегда срабатывает после prefix. Для команд
плагинов используйте `plugin:<name>`.

Действия: `split_vertical`, `split_horizontal`, `close_pane`, `new_tab`,
`close_tab`, `next_tab`, `prev_tab`, `select_tab`, `focus_next`, `focus_left`,
`focus_right`, `focus_up`, `focus_down`, `toggle_broadcast`, `file_manager`,
`help`, `fullscreen`, `detach`, `quit`, `none` (удалить биндинг). `select_tab`
принимает номер вкладки в `arg` (1..9).

`replace_defaults = true` очищает стандартные биндинги; биндинги плагинов
остаются. `unbind` убирает биндинг по клавише, включая стандартный или
плагинный.

## init.lua

Lua-конфиг выполняется после TOML и может переопределять настройки.

```lua
ggterm.set_option("font.size", 16)
ggterm.set_option("font.ligatures", true)
ggterm.set_option("font.fallback", { "C:/Windows/Fonts/msyh.ttc" })
ggterm.set_option("window.transparent", false)
ggterm.set_option("padding", 6)

ggterm.set_prefix("ctrl+a")
ggterm.bind("c", "new_tab")
ggterm.bind("1", "select_tab", 1)
ggterm.bind("z", "plugin:clock_toggle")
ggterm.unbind("q")
ggterm.clear_bindings()

ggterm.plugin_dir("plugins")
ggterm.enable_plugin("clock")
ggterm.disable_plugin("scratch")

ggterm.theme("nord")
ggterm.theme_file("themes/my-theme.toml")
ggterm.set_theme({ cursor = "#ffcc66", palette = { "#000000" } })
```

`set_option` принимает:

| Ключ | Тип |
|---|---|
| `session`, `shell` | строка |
| `cols`, `rows`, `scrollback`, `padding` | число |
| `restore`, `decorated` | boolean |
| `font.family`, `font.path`, `font.bold`, `font.italic`, `font.bold_italic` | строка |
| `font.size` | число |
| `font.fallback`, `plugins.enabled`, `plugins.disabled` | таблица строк |
| `font.ligatures`, `window.transparent` | boolean |
| `theme`, `keys.prefix`, `plugins.dir` | строка |

`theme_file` и `set_option("font.path", ...)` разрешают относительные пути
относительно каталога Lua-файла.

## Hot reload

Каждые 0.5 с проверяются конфиг, Lua, файлы тем и плагинов. Без перезапуска
применяются:

- тема и отдельные цвета;
- шрифт и лигатуры;
- keybindings;
- отступы и decorations;
- плагины (при изменении файлов они перезагружаются).

При следующем запуске применяются `session`, `shell`, `cols`, `rows`,
`scrollback`, `restore` и `window.transparent`. Результат перезагрузки или
ошибка видны в статус-баре.

## Переменные окружения

| Переменная | Назначение |
|---|---|
| `GGTERM_SHELL` | shell, если не задан флагом или конфигом |
| `GGTERM_FONT_DIRS` | дополнительные каталоги шрифтов |
| `GGTERM_FFMPEG` | путь к ffmpeg |
| `GGTERM_FFPROBE` | путь к ffprobe |
