Выполнил Алипов Ярослав Юрьевич

# Задание 6. Системное программирование "Издание"

Сервис редакции. Источники отправляют заметки, редакция задаёт им
доверие, а сервер выпускает сюжет после набора порога

Сервер и клиент написаны на Go. Данные хранятся в SQLite. Связь идёт по TCP,
сообщения в формате JSON

## Запуск через Docker

Команды выполняются из корня репозитория. Для чистого запуска удалите старую
SQLite-базу и Docker volume

```sh
docker compose down -v --remove-orphans
make build
make init EDITOR_ID=editor EDITOR_SECRET=editor-secret
```

`docker compose down -v` удаляет все данные проекта. Если данные нужно
сохранить, не выполняйте эту команду и используйте известные учётные данные
редактора

После `make init` должен появиться текст `database initialized`. Затем создайте
источники. Регистрации через клиент нет

```sh
docker compose run --rm server client add \
  --db /data/edition.db --id staff --secret staff-secret --trust 1.00
docker compose run --rm server client add \
  --db /data/edition.db --id freelance --secret freelance-secret --trust 0.35
```

Запустите сервер в первом терминале

```sh
make up
```

Во втором терминале сначала проверьте вход редактора. Это нужно сделать до
создания заметок

```sh
docker compose --profile client run --rm client stats --addr server:9000 \
  --id editor --secret editor-secret --all --json
```

Теперь отправьте заметки

```sh
export EVENT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
export NOTE_ID="11111111-1111-4111-8111-111111111111"

docker compose --profile client run --rm client submit --addr server:9000 \
  --id staff --secret staff-secret --note-id "$NOTE_ID" \
  --section city --topic 'Мост закрыт' --occurred-at "$EVENT_AT"

docker compose --profile client run --rm client submit --addr server:9000 \
  --id staff --secret staff-secret --note-id "$NOTE_ID" \
  --section city --topic 'Мост закрыт' --occurred-at "$EVENT_AT"

docker compose --profile client run --rm client submit --addr server:9000 \
  --id freelance --secret freelance-secret \
  --section city --topic 'Мост закрыт' --occurred-at "$EVENT_AT"
```

У первой заметки доверие `1.00`, поэтому выпуска ещё нет. Повтор с тем же
`NOTE_ID` вернёт `duplicate`. После заметки второго источника сумма равна
`1.35`, что выше порога `1.05`

Прочитайте уведомления и посмотрите статистику

```sh
docker compose --profile client run --rm client inbox --addr server:9000 \
  --id staff --secret staff-secret
docker compose --profile client run --rm client inbox --addr server:9000 \
  --id freelance --secret freelance-secret

docker compose --profile client run --rm client stats --addr server:9000 \
  --id staff --secret staff-secret
docker compose --profile client run --rm client stats --addr server:9000 \
  --id editor --secret editor-secret --all --json
```

`inbox` подтверждает сообщение только после вывода. Если клиент был отключён в
момент выпуска, сообщение останется в outbox до следующего подключения

Остановить сервисы

```sh
make down
```

Том SQLite при этом сохраняется

## Правила

- Клиент сначала проходит `hello` с идентификатором и секретом
- Сюжет определяется точным совпадением `section` и `topic`
- Вклад источника даёт только первая заметка по сюжету. Доверие сохраняется в
  момент приёма и убывает от `occurred_at` с периодом полураспада 6 часов
- Сюжет выпускается один раз при сумме не меньше `1.05`
- После выпуска заметки принимаются, но не дают голос и не попадают в печать
- Первые три источника получают сообщение с рангом. Все остальные источники
  остаются в порядке выпуска и учитываются в статистике
- `stats` без `--all` показывает личные данные. `--all` доступен редактору

Время события передаётся в RFC 3339 UTC и не может быть в будущем. По умолчанию
период полураспада `6h`, число поздравляемых `3`. Эти значения меняются
флагами `--publish-threshold`, `--half-life` и `--congratulations` у `serve`

## Протокол

Первая строка соединения:

```json
{"type":"hello","client_id":"staff","secret":"staff-secret"}
```

Дальше доступны `submit`, `stats`, `pending_messages` и `ack_message`
Для `submit` нужны `note_id`, `section`, `topic` и `occurred_at`. Один и тот же
UUID с теми же данными даёт `duplicate`; с другими данными возвращается отказ

## Тесты

```sh
make test
```
