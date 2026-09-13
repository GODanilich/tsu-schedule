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
GOOGLE_CREDENTIALS_FILE=google-service-account.json
GOOGLE_CALENDAR_ID=calendar-id@group.calendar.google.com
SQLITE_FILE=schedule.db
```

HTTP API получает расписание из InTime с применением blacklist. SQLite-файл `schedule.db` хранит исходное расписание и правила blacklist: при запуске приложение в фоне загружает текущую неделю и четыре следующие, по одной неделе, а затем повторяет проверку каждый час. Прогресс фоновой загрузки пишется в лог каждые 10 процентов. Исходные пары не удаляются из кэша blacklist-фильтром, поэтому их можно восстановить после удаления правила.

## Blacklist пар

Blacklist хранится в таблице `blacklist_rules` и управляется через API. Правило с `subject` полностью исключает предмет по точному названию. Правило с `date` и `number` исключает конкретную пару в конкретный день.

Добавить правило для предмета:

```bash
curl -X POST http://localhost:8080/blacklist/ \
    -H 'Content-Type: application/json' \
    -d '{"subject":"Математика"}'
```

Добавить правило для конкретной пары:

```bash
curl -X POST http://localhost:8080/blacklist/ \
    -H 'Content-Type: application/json' \
    -d '{"date":"2026-09-07","number":2}'
```

Получить и удалить правила:

```bash
curl http://localhost:8080/blacklist/
curl -X DELETE http://localhost:8080/blacklist/1
```

После добавления или удаления правила приложение сразу пересинхронизирует текущую и четыре следующие недели. Перед изменением Google Calendar оно проверяет существующие события: заблокированные события удаляются, а после снятия правила отсутствующие события создаются заново. Новые ответы расписания и последующие фоновые синхронизации также учитывают blacklist.

```bash
curl -X POST "http://localhost:8080/calendar/sync?date=2026-09-07"
```

## Google Calendar

Синхронизация выполняется от имени service account. Приложение создает или обновляет события пар, не дублирует их при повторном запуске и удаляет устаревшие события, которые были созданы им ранее.

### Подключение отдельного календаря

1. В [Google Calendar](https://calendar.google.com/) создайте календарь, например `Расписание ТГУ`, и скопируйте его ID из раздела **Интеграция с календарем**.
2. В [Google Cloud Console](https://console.cloud.google.com/) создайте проект и включите **Google Calendar API**.
3. В разделе **IAM и администрирование** → **Сервисные аккаунты** создайте service account и скачайте для него JSON-ключ в корень проекта под именем `google-service-account.json`.
4. В настройках календаря, в разделе **Общий доступ для отдельных пользователей**, добавьте email service account и выдайте разрешение **Вносить изменения в мероприятия**.
5. Укажите ID календаря в `.env` и запустите сервер. Google Calendar берет расписание из SQLite-кэша:

    ```bash
    go run ./cmd/server
    curl -X POST "http://localhost:8080/calendar/sync?date=2026-09-03"
    ```

Параметр `date` необязателен: без него синхронизируется текущая неделя. Не добавляйте JSON-ключ service account в Git.

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

### Ручное обновление SQLite-кэша

```bash
curl -X PUT "http://localhost:8080/schedule/cache?date=2026-09-03"
```

`date` необязателен. Приложение загрузит из InTime неделю, содержащую указанную дату, и атомарно заменит ее в SQLite. Ответ содержит начало недели и число сохраненных пар.

### Синхронизация с Google Calendar

```bash
curl -X POST "http://localhost:8080/calendar/sync?date=2026-09-03"
```

Ответ содержит начало недели и количество добавленных или обновленных пар.

### Очистка Google Calendar

```bash
curl -X DELETE "http://localhost:8080/calendar/"
```

Эта операция необратимо удаляет **все** события из календаря, указанного в `GOOGLE_CALENDAR_ID`, включая созданные вручную.

## Архитектура

```text
HTTP handler
    ↓
Schedule service
    ↓
InTime client
    ↓
InTime API

Calendar sync handler
    ↓
SQLite schedule cache + Google Calendar API
```

InTime-specific JSON модели находятся только в `internal/intime`.
Доменная модель находится в `internal/schedule`, поэтому позже можно
добавить другой источник расписания без изменения HTTP API.

## Важное замечание о времени

InTime передает `starts` и `ends` как секунды от начала дня в UTC. Клиент
переводит их в часовую зону из `TIMEZONE`, поэтому занятия отображаются в
томском времени без ручного смещения в настройках.
