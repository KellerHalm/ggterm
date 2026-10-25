# Примеры плагинов

Скопируйте нужный `.lua` в каталог плагинов:

- Windows: `%AppData%\ggterm\plugins\`
- Linux: `~/.config/ggterm/plugins/`
- macOS: `~/Library/Application Support/ggterm/plugins/`

или укажите другой каталог в `config.toml`:

```toml
[plugins]
dir = "C:/tools/ggterm-plugins"
```

`clock.lua` показывает время в статус-баре (prefix + `t`), `hello.lua` —
минимальный пример команды, биндинга и перехвата клавиши. Полное описание API —
в [docs/PLUGINS.md](../../docs/PLUGINS.md).
