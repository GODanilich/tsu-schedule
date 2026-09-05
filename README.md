# TSU Schedule

MVP REST API для получения расписания группы ТГУ через InTime.

## Требования

- Go 1.26+
- Интернет

## Запуск

```bash
cp .env.example .env
go mod tidy
go run ./cmd/server
```

По умолчанию сервер запускается на:

```text
http://localhost:8080
```

## Конфигурация

`.env`:

```env
HTTP_PORT=8080
INTIME_BASE_URL=https://intime.tsu.ru/api/web/v1
GROUP_ID=4d9906f9-2f03-11ef-815e-005056bc52bb
TIMEZONE=Asia/Tomsk
HTTP_TIMEOUT=10s
```

## API

### Health

```bash
curl http://localhost:8080/health
```

### Сегодня

```bash
curl http://localhost:8080/schedule/today
```

### Конкретный день

```bash
curl "http://localhost:8080/schedule/day?date=2026-09-03"
```

### Конкретная неделя

Дата может быть любым днём недели. API вернёт понедельник-воскресенье.

```bash
curl "http://localhost:8080/schedule/week?date=2026-09-03"
```

## Архитектура

```text
HTTP handler
    ↓
Schedule service
    ↓
InTime client
    ↓
InTime API
```

InTime-specific JSON модели находятся только в `internal/intime`.
Доменная модель находится в `internal/schedule`, поэтому позже можно
добавить другой источник расписания без изменения HTTP API.

## Важное замечание о времени

Исходный Python-код вручную прибавляет 7 часов к `starts`/`ends`.
В этом проекте предполагается, что `starts` и `ends` — секунды от начала
локального дня. Поэтому дополнительный `+7` не применяется.

Если фактический ответ InTime использует другой формат времени, функцию
`timeFromSeconds` в `internal/intime/client.go` нужно адаптировать.
