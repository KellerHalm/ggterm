# Плагины ggterm

Плагин — это один файл `.lua`, который выполняется в процессе UI. Плагины могут
добавлять команды, биндинги и обработчики событий, читать состояние
мультиплексора и отправлять ввод в активную панель.

Демон сессии плагины не исполняет: они относятся к клиенту. При detach/attach
плагины загружаются заново, а состояние сессии и панелей сохраняется.

## Где лежат плагины

По умолчанию читается каталог:

- Windows: `%AppData%\ggterm\plugins\`
- Linux: `~/.config/ggterm/plugins/`
- macOS: `~/Library/Application Support/ggterm/plugins/`

Загружаются все `*.lua` по алфавиту. Каталог, фильтры и отключение задаются в
`config.toml`:

```toml
[plugins]
dir = "plugins"             # относительно каталога конфига; можно абсолютный путь
enabled = ["clock"]         # пусто — загружать все файлы
disabled = ["scratch"]      # исключить по имени файла без .lua
```

Флаги командной строки:

- `--plugins-dir PATH` — другой каталог;
- `--no-plugins` — не загружать плагины.

Из Lua-конфига доступны `ggterm.plugin_dir("plugins")`,
`ggterm.enable_plugin("clock")` и `ggterm.disable_plugin("scratch")`.

## Анатомия плагина

```lua
ggterm.plugin({
  name = "clock",
  version = "1.0.0",
  description = "clock in the status bar",
})

ggterm.command("clock_toggle", "toggle the clock", function(arg)
  ggterm.notify("arg=" .. tostring(arg))
  ggterm.status(os.date("%H:%M:%S"))
end)

ggterm.bind("t", "plugin:clock_toggle")

ggterm.on("tick", function()
  ggterm.status(os.date("%H:%M:%S"))
end)
```

`ggterm.plugin` необязателен: без него именем плагина становится имя файла.
Остальные вызовы `ggterm.command`, `ggterm.bind` и `ggterm.on` должны
выполняться во время загрузки файла. Если файл завершился ошибкой, все команды,
биндинги и хуки, зарегистрированные этим файлом, откатываются.

## Команды и биндинги

```lua
ggterm.command(name, description, fn)
ggterm.command(name, fn)
ggterm.bind(key, action, arg?, description?)
```

Команда получает один числовой аргумент — `arg` из биндинга (`0`, если он не
задан). Команда доступна как действие `plugin:<name>` (синоним `command:<name>`)
в конфиге и в `ggterm.bind`:

```toml
[[keys.bindings]]
key = "z"
action = "plugin:clock_toggle"
```

Биндинги плагинов добавляются к стандартным и перекрываются пользовательскими
`unbind`/`bindings`. `replace_defaults = true` убирает стандартные биндинги, но
оставляет биндинги плагинов. Некорректный биндинг плагина игнорируется, чтобы
плагин не мешал запуску.

## События

| Событие | Когда вызывается | Аргумент |
|---|---|---|
| `start` | после успешной загрузки всех плагинов | нет |
| `key` | на каждое нажатие клавиши до обработки prefix (в обычном режиме, не в файловом менеджере) | таблица клавиши |
| `tick` | не чаще одного раза в секунду | нет |
| `shutdown` | при закрытии UI и перед перезагрузкой плагинов | нет |

`key` получает таблицу:

```lua
{
  key = "ctrl+shift+k",  -- имя в нотации конфига
  ctrl = true,
  alt = false,
  shift = true,
  prefix = false,        -- true, если уже нажат prefix мультиплексора
}
```

Верните `true`, чтобы перехватить клавишу и не отправлять её в панель. Если
несколько плагинов слушают `key`, обработчики вызываются по порядку до первого
`true`. Имена клавиш: `a`..`z`, `0`..`9`, `enter`, `tab`, `space`, `escape`,
`backspace`, `delete`, `insert`, `home`, `end`, `pageup`, `pagedown`, `up`,
`down`, `left`, `right`, `f1`..`f12`, `slash`, `backslash`, `minus`, `equal`,
`comma`, `period`, `semicolon`, `quote`, `backquote`, `leftbracket`,
`rightbracket`.

## API

| Функция | Назначение |
|---|---|
| `ggterm.notify(text)` | сообщение в правой части статус-бара на несколько секунд |
| `ggterm.status(text)` | постоянный текст плагина в статус-баре; пустая строка очищает |
| `ggterm.input(text)` | отправить строку в активную панель |
| `ggterm.action(name, arg?)` | выполнить встроенное действие; возвращает `true`, если действие известно |
| `ggterm.state()` | таблица состояния клиента |
| `ggterm.active()` | активная панель или `nil` |
| `ggterm.tabs()` | массив вкладок |
| `ggterm.version()` | версия ggterm |
| `ggterm.command`, `ggterm.bind`, `ggterm.on`, `ggterm.plugin` | регистрация, см. выше |

`ggterm.action` принимает те же имена, что и конфиг: `split_vertical`,
`split_horizontal`, `new_tab`, `select_tab`, `focus_left`, `file_manager`,
`fullscreen`, `detach`, `quit` и остальные из README. Для `select_tab`
нумерация с единицы. Команду плагина можно вызвать как
`ggterm.action("plugin:clock_toggle", 1)`. Если действие завершает клиент
(`quit`, `detach`), UI закроется после возврата из обработчика.

`ggterm.state()`:

| Поле | Значение |
|---|---|
| `session` | имя сессии |
| `cols`, `rows` | размер области панелей |
| `active_tab` | номер активной вкладки с единицы |
| `tabs`, `panes` | количество вкладок и панелей активной вкладки |
| `broadcast` | включён ли broadcast |
| `theme` | имя темы |

`ggterm.active()`: `id`, `title`, `dir`, `alive`, `scroll_offset`.
`ggterm.tabs()`: массив таблиц `{ index = 1, title = "...", active = true }`.

## Перезагрузка и ошибки

Файлы плагинов и каталог входят в hot reload. Если файл добавлен, удалён или
изменён, все плагины перезагружаются в новом Lua-состоянии, и заново вызывается
`start`. Переменные, объявленные в плагине, при перезагрузке теряются.

Ошибки загрузки и выполнения хуков показываются в статус-баре; один сломанный
плагин не мешает остальным. Все плагины делят одно Lua-состояние и глобальную
таблицу `ggterm`, поэтому имена команд должны быть уникальны.

Плагины — это произвольный код с теми же правами, что и ggterm. Устанавливайте
только доверенные плагины.
