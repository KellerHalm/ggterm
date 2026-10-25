# GoGoTerminal (ggterm)

Кроссплатформенный терминал на Go: эмулятор терминала и основа для мультиплексора, файлового менеджера и мультимедиа из `GGTerm_TZ.md`.

Текущий статус — **Этап 3: кастомизация**. Несколько панелей с произвольной вложенностью, вкладки, detach/attach через фоновый демон, а также конфигурация TOML/Lua, темы, шрифты и hot reload.

## Что уже работает

### Терминальный эмулятор

- PTY-слой: ConPTY на Windows, `creack/pty` на Linux/macOS.
- Потоковый парсер ANSI/VT: C0, ESC, CSI, OSC, DCS/APC/PM/SOS.
- Цвета: 16 базовых, 256-цветная палитра, true color (24-bit).
- Атрибуты: bold, faint, italic, underline, blink, reverse, strikethrough.
- Курсор: позиционирование, видимость, сохранение/восстановление, pending-wrap.
- Регион прокрутки, origin mode, вставка/удаление строк и символов.
- Альтернативный экран (`?47`, `?1047`, `?1049`) для full-screen приложений.
- Scrollback с настраиваемым лимитом, прокрутка колесом и PageUp/PageDown.
- UTF-8, широкие символы CJK и emoji в модели буфера.
- OSC-заголовок окна, ответы на DSR (`CSI 5n`, `CSI 6n`, `CSI ?6n`) и DA (`CSI c`).
- Изменение размера окна с передачей размера в PTY.
- Автоматическое закрытие панели после выхода процесса; окно закрывается вместе с последней панелью.
- CLI-флаги для shell, размера сетки, scrollback и размера шрифта.

### Мультиплексор

- Вертикальные и горизонтальные сплиты панелей, произвольная вложенность.
- Вкладки, каждая со своим деревом панелей и собственным фокусом.
- Навигация по панелям стрелками и `hjkl`, клик мышью для фокуса панели или вкладки.
- Prefix-биндинги в стиле tmux (по умолчанию `Ctrl+B`).
- Broadcast-режим: ввод дублируется во все панели активной вкладки.
- У каждой панели свой scrollback, прокрутка и альтернативный экран.
- Сохранение раскладки (вкладки, сплиты, пропорции и каталоги запуска панелей) в JSON и восстановление при следующем запуске.

### Сессии и фоновый демон

- Обычный запуск `ggterm` подключается к сессии `default`; если демона ещё нет, он запускается автоматически в фоне.
- Несколько именованных сессий (`--session work`), каждая со своим демоном, вкладками и панелями.
- Detach по `Ctrl+B d` или закрытию окна: процессы и scrollback остаются жить в демоне.
- Attach при следующем запуске: возвращаются все вкладки, панели и содержимое скроллбэка.
- `--list` показывает запущенные сессии, `--kill` завершает выбранную.
- Демон использует локальный IPC без сетевых портов: named pipe на Windows, unix-сокет на Linux/macOS.
- `--no-daemon` запускает всё в одном процессе, как в предыдущем этапе (без detach/attach).

### Кастомизация

- Конфигурация в TOML: сессия, shell, размеры, scrollback, отступы, шрифт и keybindings.
- Опциональный Lua-конфиг (`init.lua`) для вычисляемых настроек, тем и биндингов.
- Встроенные темы (`ggterm-dark`, `dracula`, `nord`, `gruvbox-dark`, `solarized-dark`, `tokyo-night`, `catppuccin-mocha`) и собственные TOML-темы.
- Импорт схем base16 (JSON/YAML) и iTerm2 (`.itermcolors`).
- Шрифт: встроенный Go Mono, внешний `.ttf`/`.otf` через `font.path`, fallback-шрифты.
- Полностью настраиваемые prefix и keybindings, включая отмену и замену стандартных.
- Hot reload: изменения темы, шрифта, keybindings и оформления подхватываются без перезапуска.

## Требования

- Go 1.26 (тулчейн скачивается автоматически при сборке).
- Windows 10 1809+ для ConPTY и named pipes; Linux/macOS — стандартный PTY.
- Linux: пакеты для сборки оконных приложений (`libgl1-mesa-dev`, `xorg-dev`).
- macOS: Xcode Command Line Tools.

## Сборка и запуск

```bash
go build -o ggterm.exe ./cmd/ggterm   # Windows
go build -o ggterm ./cmd/ggterm       # Linux/macOS
```

```bash
./ggterm.exe
./ggterm --shell "pwsh -NoLogo" --cols 140 --rows 40 --scrollback 20000 --font-size 16
```

Флаги:

| Флаг | По умолчанию | Назначение |
|---|---|---|
| `--session` | `default` | Имя сессии; у каждой сессии свой демон и раскладка |
| `--shell` | системный shell | Команда shell; пустое значение — `COMSPEC`/`$SHELL` |
| `--cols` | 120 | Начальное число колонок |
| `--rows` | 36 | Начальное число строк |
| `--scrollback` | 10000 | Лимит строк истории |
| `--font-size` | 15 | Размер шрифта в пикселях |
| `--restore` | true | Восстанавливать вкладки и раскладку прошлой сессии |
| `--layout-file` | `ggterm/sessions/<session>.json` в конфиг-каталоге | Файл раскладки |
| `--no-daemon` | false | Работать в одном процессе, без detach/attach |
| `--list` | false | Показать запущенные сессии и выйти |
| `--kill` | false | Завершить выбранную сессию и выйти |
| `--config` | `ggterm/config.toml` в конфиг-каталоге | Путь к TOML- или Lua-конфигу |
| `--no-config` | false | Игнорировать файл конфигурации |

Переменная `GGTERM_SHELL` задаёт shell, если ни флаг `--shell`, ни конфиг не указали команду.

```bash
./ggterm.exe --list                               # список сессий
./ggterm.exe --session work                       # подключиться к "work" или создать
./ggterm.exe --kill --session work                # завершить сессию
./ggterm.exe --no-daemon --restore=false          # одна панель без демона
```

Раскладки (`*.json`), маркеры запущенных сессий (`*.session`, нужны для `--list` на Windows) и лог демона лежат в `ggterm/sessions/` и `ggterm/daemon.log` внутри `os.UserConfigDir()`. На Windows демон слушает named pipe `\\.\pipe\ggterm-<user>-<session>`; на Linux/macOS — unix-сокет в коротком каталоге `ggterm-<uid>` внутри `os.TempDir()` (ограничение длины пути `sun_path`).

## Конфигурация

По умолчанию читается `ggterm/config.toml`, затем `init.lua` из того же каталога `os.UserConfigDir()/ggterm` (Windows: `%AppData%\ggterm`, Linux: `~/.config/ggterm`, macOS: `~/Library/Application Support/ggterm`). Путь можно задать флагом `--config`; файл с расширением `.lua` загружается как Lua-конфиг без TOML. `--no-config` отключает файлы. Явно заданные флаги командной строки перекрывают значения из файла.

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
family = "Go Mono"        # встроенный шрифт; также можно указать путь к файлу
# path = "C:/fonts/JetBrainsMono-Regular.ttf"
# fallback = ["C:/Windows/Fonts/msyh.ttc"]
size = 16

[theme]
name = "tokyo-night"      # встроенная тема; name и file взаимоисключающие
# file = "themes/base16-nord.yaml"
background = "#0b1020"    # необязательные точечные переопределения
palette = ["#15161e", "#f7768e", "#9ece6a", "#e0af68",
           "#7aa2f7", "#bb9af7", "#7dcfff", "#a9b1d6",
           "#414868", "#f7768e", "#9ece6a", "#e0af68",
           "#7aa2f7", "#bb9af7", "#7dcfff", "#c0caf5"]

[keys]
prefix = "ctrl+b"
replace_defaults = false  # true — начать с пустого списка биндингов
unbind = ["q"]

[[keys.bindings]]
key = "%"
action = "split_vertical"

[[keys.bindings]]
key = "1"
action = "select_tab"
arg = 1                   # номера вкладок 1..9
```

Доступные действия: `split_vertical`, `split_horizontal`, `close_pane`, `new_tab`, `close_tab`, `next_tab`, `prev_tab`, `select_tab`, `focus_next`, `focus_left`, `focus_right`, `focus_up`, `focus_down`, `toggle_broadcast`, `help`, `detach`, `quit`. Действие `none` удаляет биндинг. Клавиши записываются как `c`, `ctrl+a`, `shift+slash`, `alt+enter`, `f5`, `up`, `%`, `?`; для команд мультиплексора префикс добавлять не нужно — биндинг всегда срабатывает после prefix.

Темы можно вынести в отдельный файл (`theme.file`): TOML с полями `background`, `foreground`, `cursor`, `bar_background`, `bar_foreground`, `tab_active_background`, `tab_active_foreground`, `border`, `border_active`, `overlay_background`, `palette` (16 цветов). Также поддерживаются base16-схемы в JSON/YAML (`base00`..`base0F`) и iTerm2 `.itermcolors`.

Lua-конфиг загружается после TOML и может переопределять настройки:

```lua
ggterm.set_option("font.size", 16)
ggterm.set_option("padding", 6)
ggterm.set_prefix("ctrl+a")
ggterm.bind("c", "new_tab")
ggterm.bind("1", "select_tab", 1)
ggterm.unbind("q")
ggterm.theme("nord")
ggterm.theme_file("themes/my-theme.toml")
ggterm.set_theme({ cursor = "#ffcc66" })  -- цвета можно задать и точечно
```

Доступные опции `set_option`: `session`, `shell`, `cols`, `rows`, `scrollback`, `restore`, `padding`, `decorated`, `font.family`, `font.path`, `font.size`, `font.fallback`, `theme`, `keys.prefix`.

Hot reload опрашивает конфиг, Lua и файл темы каждые 0.5 с. Без перезапуска применяются тема, шрифт, keybindings, отступы и decorations; сообщение об успехе или ошибке показывается в статус-баре. `session`, `shell`, `cols`, `rows` и `scrollback` применяются только при следующем запуске или создании панели.

Примеры лежат в `examples/config.toml` и `examples/init.lua`.

## Управление

Обычные клавиши и комбинации отправляются активной панели.

| Действие | Клавиши |
|---|---|
| Ввод | Печатные символы Unicode |
| Выполнить / новая строка | Enter |
| Удаление | Backspace, Delete |
| Навигация | Стрелки, Home, End |
| История вывода | Колесо мыши (в alt screen — стрелки), PageUp/PageDown (Shift — локально) |
| Прервать процесс | Ctrl+C |
| Управляющие комбинации | Ctrl+A..Z, Ctrl+Space, Ctrl+[ \ ] ^ _ |
| Функциональные клавиши | F1–F12 |
| Alt-комбинации | Alt+клавиша отправляет ESC-префикс |

В альтернативном экране (vim, less, htop) PageUp/PageDown отправляются приложению, а не прокручивают историю.

Префикс мультиплексора по умолчанию — `Ctrl+B` (настраивается в `[keys]`). После него:

| Действие | Клавиши |
|---|---|
| Сплит вертикально (рядом) | `%` или `Shift+\` |
| Сплит горизонтально (друг под другом) | `"` или `-` |
| Фокус панели | `h` `j` `k` `l` или стрелки |
| Следующая панель | `o` |
| Новая вкладка | `c` |
| Следующая / предыдущая вкладка | `n` / `p` |
| Выбрать вкладку | `1`..`9` |
| Закрыть панель | `x` |
| Закрыть вкладку | `&` |
| Broadcast-ввод | `b` |
| Справка | `?` |
| Detach (сессия продолжает работать) | `d` |
| Завершить сессию и демон | `q` |

Повторное нажатие prefix-клавиши отправляет в панель соответствующий символ (по умолчанию `Ctrl+B`). Клик мышью по панели или вкладке переключает фокус. Закрытие окна тоже отсоединяет клиент, не завершая сессию.

## Архитектура

```
cmd/ggterm            точка входа, флаги, режимы attach/daemon/list/kill
internal/pty          ConPTY / Unix PTY, resize, wait/close
internal/terminal     парсер VT/ANSI, сетка ячеек, scrollback, ответы
internal/mux          панели, дерево сплитов, вкладки, процессы панелей, layout JSON
internal/config       TOML/Lua-конфигурация, темы, шрифты, hot reload
internal/session      интерфейс, который UI использует для управления мультиплексором
internal/daemon       фоновый демон, IPC-протокол, удалённый клиент, запуск процесса
internal/ui           окно Ebitengine, ввод, keybindings, отрисовка панелей и вкладок
```

Поток данных в режиме демона:

```
daemon: shell <-> PTY <-> Pane.pump <-> terminal.Terminal <-> frame encoder
                                                        |
                              local IPC: unix socket / named pipe (gob)
                                                        |
client: ui.Draw <- snapshot cache <- frame decoder <-> ключи/мышь -> requests
```

`internal/terminal` не зависит от PTY и UI, `internal/mux` не зависит от UI, а `internal/daemon` отделяет фоновые процессы от окна. Благодаря этому раскладка, панели и демон тестируются без графики.

## Проверка

```bash
go test -race -shuffle=on -count=1 ./...
go vet ./...
```

CI (`.github/workflows/ci.yml`) собирает и тестирует проект на Windows, Linux и macOS.

## Ограничения текущего этапа

- К сессии одновременно подключён один клиент: новое подключение вытесняет предыдущее (в стиле `tmux attach`), совместный просмотр пока не поддерживается.
- Scrollback живёт только в памяти демона: после его завершения история теряется, восстанавливается лишь раскладка.
- Hot reload применяет только визуальные настройки и keybindings; `session`, `shell`, размеры и scrollback требуют перезапуска.
- По имени доступен только встроенный Go Mono; другие шрифты подключаются через `font.path`/`font.fallback`. Системный поиск шрифтов, лигатуры и настоящие bold/italic — Этап 4.
- Нет файлового менеджера, графического протокола, Kitty Graphics и видео — Этапы 5–7.
- Нет лигатур, настоящего жирного начертания и курсива: Go Mono не содержит этих начертаний, bold имитируется сдвигом.
- Fallback-шрифт для CJK нужно указать вручную через `font.fallback`; без него широкие символы занимают две ячейки, но могут отображаться как отсутствующий глиф.
- Нет буфера обмена, повтора клавиш и настройки прозрачности окна.
- Combining marks (диакритика) в модели ячейки не накапливаются.
- Unix-часть собирается и покрыта тестами, но разрабатывается на Windows — проверка в CI.

## Дорожная карта

- [x] Этап 1. MVP терминального эмулятора
- [x] Этап 2. Мультиплексор: панели, вкладки, layout, сохранение раскладки
- [x] Этап 2b. Detach/attach и фоновый демон сессий
- [x] Этап 3. Кастомизация: TOML, темы, шрифты, Lua, hot reload
- [ ] Этап 4. GPU-полировка: лигатуры, оптимизация, прозрачность
- [ ] Этап 5. Графика: свой протокол и Kitty Graphics
- [ ] Этап 6. Файловый менеджер в стиле yazi
- [ ] Этап 7. Видео через ffmpeg/libav
- [ ] Этап 8. Плагины, документация, релиз v1.0

Полное техническое задание — `GGTerm_TZ.md`.
