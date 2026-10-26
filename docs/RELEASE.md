# Сборка и релиз ggterm

## Проверка

Перед релизом:

```bash
gofmt -l .
go vet ./...
go test -race -shuffle=on -count=1 ./...
go build ./...
```

CI (`.github/workflows/ci.yml`) выполняет сборку и тесты на Windows, Linux и
macOS. Для Linux ставятся `libgl1-mesa-dev` и `xorg-dev`.

## Версия

Версия хранится в `internal/version`:

```go
var (
	Version = "1.0.0"
	Commit  = "dev"
	Date    = "unknown"
)
```

Релизная сборка подставляет значения через `-ldflags`:

```bash
go build -trimpath \
  -ldflags "-s -w \
    -X ggterm/internal/version.Version=1.0.0 \
    -X ggterm/internal/version.Commit=$(git rev-parse --short HEAD) \
    -X ggterm/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o ggterm ./cmd/ggterm
```

`ggterm --version` печатает версию, коммит и дату (если они заданы).

## Релиз

1. Обновите `internal/version/version.go` и `CHANGELOG.md`.
2. Убедитесь, что `go vet` и тесты проходят.
3. Поставьте тег и отправьте его:

   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

4. `.github/workflows/release.yml` соберёт `ggterm` на Windows, Linux и macOS,
   упакует архивы, посчитает SHA-256 и создаст GitHub Release.

Для каждой ОС собираются `amd64` и `arm64`. Архивы называются
`ggterm_<version>_<os>_<arch>.zip` (Windows) и
`ggterm_<version>_<os>_<arch>.tar.gz` (Linux/macOS). Внутри — бинарник, README,
CHANGELOG, `docs/` и `examples/`. Рядом с релизом лежит `checksums.txt` с
SHA-256.

## Замечания по платформам

- Windows использует ConPTY (Windows 10 1809+) и named pipes; отдельные
  зависимости не нужны.
- Linux требует заголовки X11/OpenGL для сборки UI.
- macOS требует Xcode Command Line Tools.
- `ffmpeg` и `ffprobe` не входят в поставку; видео работает, если они есть в
  `PATH` или заданы через `GGTERM_FFMPEG`/`GGTERM_FFPROBE`.
