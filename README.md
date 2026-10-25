# GoGoTerminal (ggterm)

Кроссплатформенный терминал на Go: эмулятор терминала и основа для мультиплексора, файлового менеджера и мультимедиа из `GGTerm_TZ.md`.

Текущий статус — **Этап 8: плагины, документация и релиз v1.0**. Несколько панелей с произвольной вложенностью, вкладки, detach/attach через фоновый демон, конфигурация TOML/Lua, темы и hot reload, оптимизированный GPU-рендеринг, системные шрифты, настоящие bold/italic и лигатуры, изображения через Kitty Graphics и собственный протокол, файловый менеджер в стиле yazi, воспроизведение видео через ffmpeg, а также Lua-плагины с командами, биндингами и событиями.

## Что уже работает

### Терминальный эмулятор

- PTY-слой: ConPTY на Windows, `creack/pty` на Linux/macOS.
- Потоковый парсер ANSI/VT: C0, ESC, CSI, OSC, DCS/APC/PM/SOS.
- Цвета: 16 базовых, 256-цветная палитра, true color (24-bit).
- Атрибуты: bold, faint, italic, underline (одинарное, двойное и цветное), blink, reverse, strikethrough.
- Курсор: позиционирование, видимость, сохранение/восстановление, pending-wrap.
- Регион прокрутки, origin mode, вставка/удаление строк и символов.
- Альтернативный экран (`?47`, `?1047`, `?1049`) для full-screen приложений.
- Scrollback с настраиваемым лимитом, прокрутка колесом и PageUp/PageDown.
- UTF-8, широкие символы CJK и emoji в модели буфера.
- OSC-заголовок окна, OSC 7 (текущий каталог оболочки), ответы на DSR (`CSI 5n`, `CSI 6n`, `CSI ?6n`) и DA (`CSI c`).
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
- Шрифт: встроенный Go Mono, внешний `.ttf`/`.otf` через `font.path` или поиск по названию семейства в системных каталогах (Windows, Linux, macOS).
- Настоящие начертания bold/italic/bold-italic: явные пути, автоматический поиск соседних файлов и системных вариантов; при отсутствии — синтетический bold и наклон.
- Лигатуры включаются автоматически для шрифтов, которые их содержат; `font.ligatures = false` отключает их.
- Полностью настраиваемые prefix и keybindings, включая отмену и замену стандартных.
- Hot reload: изменения темы, шрифта, keybindings и оформления подхватываются без перезапуска.

### Плагины

- Плагины — это Lua-файлы в `ggterm/plugins/` (или каталоге из `[plugins]`); загружаются все `*.lua` либо выбранные через `enabled`/`disabled`.
- API: `ggterm.plugin`, `ggterm.command`, `ggterm.bind`, `ggterm.on`, `ggterm.notify`, `ggterm.status`, `ggterm.input`, `ggterm.action`, `ggterm.state`, `ggterm.active`, `ggterm.tabs`, `ggterm.version`.
- События `start`, `key`, `tick` (раз в секунду) и `shutdown`; `key` может перехватить клавишу до отправки в панель.
- Команды плагинов вызываются биндингом `plugin:<name>` из конфига, а плагин может зарегистрировать и собственный биндинг по умолчанию.
- Плагины исполняются в процессе UI рядом с конфигурационным Lua; daemon их не запускает, поэтому при detach/attach они загружаются заново.
- Файлы плагинов участвуют в hot reload; при изменении все плагины перезагружаются, а неудачная загрузка откатывает команды, биндинги и хуки этого файла.
- `ggterm.status` показывает постоянный текст плагина в правой части статус-бара, `ggterm.notify` — временное сообщение.
- Полное описание API — [`docs/PLUGINS.md`](docs/PLUGINS.md), примеры — [`examples/plugins/`](examples/plugins/).

```lua
ggterm.plugin({ name = "clock", version = "1.0.0" })
ggterm.command("clock_toggle", "toggle the clock", function()
  ggterm.status(os.date("%H:%M:%S"))
end)
ggterm.bind("t", "plugin:clock_toggle")
```

### GPU-рендеринг и производительность

- Рендеринг идёт через GPU-конвейер Ebitengine (Direct3D/Metal/OpenGL); true color поддерживается на всех уровнях.
- Панели кэшируются в offscreen-изображения по revision и scroll offset: неподвижная панель стоит одного `DrawImage` за кадр.
- Фоны ячеек, вкладок и статус-бара собираются в один батч и отправляются одним `DrawTriangles32` вместо тысяч отдельных вызовов.
- Снимок буфера терминала не копируется каждый кадр: он запрашивается только при изменении содержимого или прокрутке.
- Поддержаны цвет и стиль подчёркивания (SGR `4`, `21`, `58`, `59`), включая 256-цветный и true-color вариант.
- Прозрачность окна: `[window] transparent = true` вместе с alpha-каналом фона темы (например `background = "#0f1117d0"`).
- Полноэкранный режим — prefix + `f`.

### Изображения и графика

- Kitty Graphics Protocol: transmit/display/place/delete, PNG/JPEG/GIF/WebP, raw RGB/RGBA, zlib, чанки `m=1`, source crop, z-order и смещение в ячейках.
- Собственный протокол `ggterm` для приложений внутри панели: `ESC _ ggterm;<params>;<base64> ESC \`.
- Изображения привязаны к абсолютным строкам и прокручиваются вместе с текстом; alt-screen хранит отдельный набор плейсментов.
- Декодированные битмапы кешируются на GPU по `Graphic.Key`; отрицательный `z` рисуется под текстом, неотрицательный — над ним.
- Лимиты: 16M пикселей, 32 MiB на передачу, 128 MiB на все изображения, 256 изображений и 4096 плейсментов на панель.

Пример Kitty (base64 PNG):

```bash
printf '\033_Ga=T,f=100,i=1,c=4,r=2;%s\033\\' "$(base64 -w0 image.png)"
```

Пример собственного протокола:

```bash
printf '\033_ggterm;action=display,id=1,cols=4,rows=2;%s\033\\' "$(base64 -w0 image.png)"
```

Параметры `ggterm`: `action=display|transmit|delete`, `id`, `format=png|jpeg|gif|webp|rgb|rgba`, `width`/`height` для raw, `cols`/`rows`, `x`/`y`, `z`, `path`, `move`. При `path` данные читаются из файловой системы демона, а payload можно не передавать.

### Видео

- Декодирование через `ffmpeg` как subprocess: форматы, которые поддерживает установленный ffmpeg (MP4, MKV, WebM, MOV, AVI и т.д.). Аудио отключается — воспроизведение визуальное.
- Превью видео в файловом менеджере запускается автоматически при выборе файла, зациклено и масштабируется по панели; внизу показываются состояние, позиция и длительность.
- Управление в файловом менеджере: `Space` — пауза/продолжить, `[`/`]` — назад/вперёд на 5 с (`Shift` — 30 с), `s` — остановить и вернуться в начало. `Enter`/`l` по-прежнему открывает файл во внешнем плеере.
- Протокол для приложений: `action=video` запускает видео в панели, `video_play`, `video_pause`, `video_seek`, `video_stop` управляют им. Каждый кадр заменяет `Graphic` и проходит через общий GPU-конвейер и Kitty-совместимый слой плейсментов.
- Пределы: протокольное видео — до 640 px по длинной стороне и 15 fps; превью — до 960 px и 30 fps. Это защищает память и IPC-трафик демона.

```bash
# запустить и зациклить видео в текущей позиции курсора (40x20 ячеек)
printf '\033_ggterm;action=video,id=1,path=/path/clip.mp4,cols=40,rows=20,loop=1\033\\'
# пауза, относительный seek на -5 с, абсолютный seek на 12.5 с
printf '\033_ggterm;action=video_pause,id=1\033\\'
printf '\033_ggterm;action=video_seek,id=1,offset=-5\033\\'
printf '\033_ggterm;action=video_seek,id=1,position=12.5\033\\'
# остановить и убрать изображение
printf '\033_ggterm;action=video_stop,id=1\033\\'
```

Для `action=video` доступны `id`, `path`, `cols`/`rows`, `x`/`y`, `z`, `loop=0|1`, `play=0|1` (автозапуск, по умолчанию включён) и `move`. `offset` в `video_seek` задаёт смещение в секундах, `position` — абсолютную позицию.

### Файловый менеджер

- Открывается оверлеем поверх активной панели по prefix + `y`; панели, вкладки и демон продолжают работать в фоне.
- Три колонки в стиле yazi: родительский каталог, текущий каталог и превью выбранного элемента.
- Текущий каталог берётся из оболочки через OSC 7 (zsh, fish, bash с соответствующим хуком), а при его отсутствии — из каталога запуска панели.
- Навигация `hjkl`/стрелками, Enter для входа в каталог и открытия файла, `h`/Backspace для возврата на уровень выше.
- Живой нечёткий фильтр по `/`: несколько слов ищутся как подпоследовательности в имени файла.
- Текстовое превью с подсветкой синтаксиса для Go, Rust, C/C++, Java/Kotlin/C#/Swift, JavaScript/TypeScript, Python, Shell, JSON, YAML, TOML/INI, Markdown, Dockerfile и Makefile.
- Превью изображений (PNG, JPEG, GIF — первый кадр, WebP, BMP, TIFF) на GPU с сохранением пропорций; бинарные файлы показываются с типом и размером.
- Операции: копирование, вырезание и вставка (включая рекурсивные каталоги), удаление, переименование, создание файла и каталога.
- Сквозной просмотр каталогов и длинного текста, показ скрытых файлов, обновление по `R`/F5.
- Открытие файла системным приложением и редактирование через `$VISUAL`/`$EDITOR` или системный редактор по умолчанию.

## Требования

- Go 1.26 (тулчейн скачивается автоматически при сборке).
- Windows 10 1809+ для ConPTY и named pipes; Linux/macOS — стандартный PTY.
- `ffmpeg` в `PATH` для видео (желательно также `ffprobe`); путь к ffmpeg можно задать переменной `GGTERM_FFMPEG`, к ffprobe — `GGTERM_FFPROBE`.
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
| `--plugins-dir` | `ggterm/plugins` в конфиг-каталоге | Каталог Lua-плагинов |
| `--no-plugins` | false | Не загружать плагины |
| `--version` | false | Показать версию и выйти |

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
family = "Go Mono"        # встроенный шрифт или семейство из системных каталогов
# path = "C:/fonts/JetBrainsMono-Regular.ttf"
# bold = "C:/fonts/JetBrainsMono-Bold.ttf"
# italic = "C:/fonts/JetBrainsMono-Italic.ttf"
# bold_italic = "C:/fonts/JetBrainsMono-BoldItalic.ttf"
# fallback = ["Microsoft YaHei"]     # путь к файлу или имя семейства
size = 16
ligatures = true          # false — рисовать по ячейкам без лигатур

[window]
transparent = false       # true + alpha в фоне темы даёт полупрозрачное окно

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

[[keys.bindings]]
key = "z"
action = "plugin:clock_toggle"   # команда плагина

[plugins]
dir = "plugins"           # относительно каталога конфига
enabled = []              # пусто — все *.lua из каталога
disabled = []
```

Доступные действия: `split_vertical`, `split_horizontal`, `close_pane`, `new_tab`, `close_tab`, `next_tab`, `prev_tab`, `select_tab`, `focus_next`, `focus_left`, `focus_right`, `focus_up`, `focus_down`, `toggle_broadcast`, `file_manager`, `help`, `fullscreen`, `detach`, `quit`. Действие `none` удаляет биндинг. Клавиши записываются как `c`, `ctrl+a`, `shift+slash`, `alt+enter`, `f5`, `up`, `%`, `?`; для команд мультиплексора префикс добавлять не нужно — биндинг всегда срабатывает после prefix.

Если задано только `font.family`, ggterm ищет файл по названию семейства в системных каталогах шрифтов (`%WINDIR%\Fonts` и пользовательские Fonts на Windows, `/usr/share/fonts` и `~/.local/share/fonts` на Linux, `/System/Library/Fonts` и `~/Library/Fonts` на macOS). Дополнительные каталоги можно перечислить в `GGTERM_FONT_DIRS` через разделитель путей ОС. Bold/italic-файлы подхватываются автоматически, если лежат рядом с основным или в системном каталоге; иначе включается синтетическое начертание.

Темы можно вынести в отдельный файл (`theme.file`): TOML с полями `background`, `foreground`, `cursor`, `bar_background`, `bar_foreground`, `tab_active_background`, `tab_active_foreground`, `border`, `border_active`, `overlay_background`, `palette` (16 цветов). Также поддерживаются base16-схемы в JSON/YAML (`base00`..`base0F`) и iTerm2 `.itermcolors`.

Lua-конфиг загружается после TOML и может переопределять настройки:

```lua
ggterm.set_option("font.size", 16)
ggterm.set_option("font.ligatures", true)
ggterm.set_option("window.transparent", false)
ggterm.set_option("padding", 6)
ggterm.set_prefix("ctrl+a")
ggterm.bind("c", "new_tab")
ggterm.bind("1", "select_tab", 1)
ggterm.unbind("q")
ggterm.theme("nord")
ggterm.theme_file("themes/my-theme.toml")
ggterm.set_theme({ cursor = "#ffcc66" })  -- цвета можно задать и точечно
ggterm.plugin_dir("plugins")              -- каталог плагинов
ggterm.enable_plugin("clock")
ggterm.disable_plugin("scratch")
```

Доступные опции `set_option`: `session`, `shell`, `cols`, `rows`, `scrollback`, `restore`, `padding`, `decorated`, `font.family`, `font.path`, `font.bold`, `font.italic`, `font.bold_italic`, `font.size`, `font.fallback`, `font.ligatures`, `window.transparent`, `theme`, `keys.prefix`.

Hot reload опрашивает конфиг, Lua и файл темы каждые 0.5 с. Без перезапуска применяются тема, шрифт, keybindings, отступы и decorations; сообщение об успехе или ошибке показывается в статус-баре. `session`, `shell`, `cols`, `rows`, `scrollback` и прозрачность окна применяются только при следующем запуске; `window.transparent` требует перезапуска, потому что режим окна задаётся до старта графического цикла.

Примеры лежат в `examples/config.toml`, `examples/init.lua` и `examples/plugins/`. Полный справочник — [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md) и [`docs/PLUGINS.md`](docs/PLUGINS.md).

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
| Файловый менеджер | `y` |
| Полноэкранный режим | `f` |
| Справка | `?` |
| Detach (сессия продолжает работать) | `d` |
| Завершить сессию и демон | `q` |

Повторное нажатие prefix-клавиши отправляет в панель соответствующий символ (по умолчанию `Ctrl+B`). Клик мышью по панели или вкладке переключает фокус. Закрытие окна тоже отсоединяет клиент, не завершая сессию.

Файловый менеджер:

| Действие | Клавиши |
|---|---|
| Перемещение по списку | `j` `k` / `↑` `↓` |
| Войти в каталог / открыть файл | `l` `o` Enter / `→`, двойной клик |
| На уровень выше | `h` Backspace / `←` |
| В начало / конец списка | `g` / `G` |
| Нечёткий фильтр | `/` (живой ввод, Esc очищает) |
| Копировать / вырезать / вставить | `y` / `x` / `p` |
| Удалить (с подтверждением) | `d` |
| Переименовать | `r` |
| Создать файл / каталог | `a` / `A` |
| Показать скрытые файлы | `.` |
| Обновить | `R` / F5 |
| Прокрутка превью | `J` `K`, `Ctrl+U` `Ctrl+D`, колесо мыши |
| Видео: пауза / продолжить | `Space` |
| Видео: назад / вперёд | `[` / `]` (`Shift` — 30 с) |
| Видео: остановить и вернуться в начало | `s` |
| Открыть в системном приложении | `e` Enter |
| Открыть в редакторе | `E` |
| Закрыть | `q` / Esc |

В режиме подтверждения удаления нажмите `y` и Enter (или `n` для отмены). При вводе имени Enter подтверждает, Esc отменяет. На Windows файлы открываются через `cmd /c start`, на macOS — через `open`, на Linux — через `xdg-open`; редактор берётся из `$VISUAL`, затем `$EDITOR`, иначе используется системный по умолчанию.

## Архитектура

```
cmd/ggterm            точка входа, флаги, режимы attach/daemon/list/kill
internal/pty          ConPTY / Unix PTY, resize, wait/close
internal/terminal     парсер VT/ANSI, сетка ячеек, scrollback, графика, ответы
internal/mux          панели, дерево сплитов, вкладки, процессы панелей, layout JSON
internal/fm           модель файлового менеджера, превью, подсветка, файловые операции
internal/video        ffmpeg-декодер: probe, кадры RGBA, play/pause/seek/stop
internal/config       TOML/Lua-конфигурация, темы, шрифты, плагины, hot reload
internal/session      интерфейс, который UI использует для управления мультиплексором
internal/daemon       фоновый демон, IPC-протокол, удалённый клиент, запуск процесса
internal/plugin       Lua-плагины: команды, биндинги, события, доступ к состоянию
internal/version      версия, коммит и дата сборки
internal/ui           окно Ebitengine, GPU-батчинг, кэш панелей и изображений, шрифты, ввод, keybindings
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
- Hot reload применяет только визуальные настройки и keybindings; `session`, `shell`, размеры, scrollback и прозрачность окна требуют перезапуска.
- GIF в превью показывает только первый кадр, Kitty animation frames не поддерживаются. Видео идёт через ffmpeg без звука и без аппаратного декодирования. Нет Sixel и iTerm2 inline images.
- Файловый менеджер работает с одним выделенным элементом, без корзины и без фонового прогресса длительных операций; удаление необратимо.
- Видео требует внешнего `ffmpeg`; звук отключён, аппаратное декодирование не используется. Превью декодирует видео в процессе UI, поэтому при открытии файла возможна короткая задержка.
- Текущий каталог файлового менеджера зависит от OSC 7: оболочки, которые его не отправляют, открывают менеджер в каталоге запуска панели.
- Плагины выполняются в процессе UI в общем Lua-состоянии, без изоляции и песочницы; при hot reload их переменные сбрасываются, а при detach/attach они загружаются заново. Демон плагины не исполняет.
- Нет менеджера установки и репозитория плагинов: файлы `.lua` кладутся в каталог вручную; API плагинов пока не стабилен между крупными версиями.
- Поиск системных шрифтов опирается на имена файлов; шрифты с нестандартными именами подключаются через `font.path`. Из коллекций `.ttc`/`.otc` используется первая гарнитура. Шрифты, установленные после запуска, находятся после перезапуска (индекс каталогов кэшируется).
- Для CJK по-прежнему нужен `font.fallback` (путь или имя семейства, если оно совпадает с именем файла); без fallback широкие символы занимают две ячейки, но могут отображаться как отсутствующий глиф.
- Размытие фона не поддерживается: доступны только прозрачность через alpha-канал фона и decorations окна.
- Нет буфера обмена и повтора клавиш.
- Combining marks (диакритика) в модели ячейки не накапливаются.
- Unix-часть собирается и покрыта тестами, но разрабатывается на Windows — проверка в CI.

## Дорожная карта

- [x] Этап 1. MVP терминального эмулятора
- [x] Этап 2. Мультиплексор: панели, вкладки, layout, сохранение раскладки
- [x] Этап 2b. Detach/attach и фоновый демон сессий
- [x] Этап 3. Кастомизация: TOML, темы, шрифты, Lua, hot reload
- [x] Этап 4. GPU-полировка: лигатуры, оптимизация, прозрачность
- [x] Этап 5. Графика: свой протокол и Kitty Graphics
- [x] Этап 6. Файловый менеджер в стиле yazi
- [x] Этап 7. Видео через ffmpeg/libav
- [x] Этап 8. Плагины, документация, релиз v1.0

Полное техническое задание — `GGTerm_TZ.md`. Документация: [`docs/CONFIGURATION.md`](docs/CONFIGURATION.md), [`docs/PLUGINS.md`](docs/PLUGINS.md), [`docs/RELEASE.md`](docs/RELEASE.md), [`CHANGELOG.md`](CHANGELOG.md).
