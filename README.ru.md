# sing-box-agent

Лёгкий Go-сервис, который выставляет REST API с подписью запросов для
управления локальным
[sing-box](https://github.com/SagerNet/sing-box) в реальном времени.
Ставится на любой Linux-сервер — управление пользователями, инбаундами
и подписками идёт через `curl`. Без панели, без SSH, без базы.

> English version: [README.md](./README.md)

---

## Зачем

Панели под sing-box (3X-UI, S-UI, X-UI и прочие) хороши для одного
сервера, но перестают масштабироваться, как только серверов становится
больше одного: веб-интерфейс кликабельный, SSH-редактирование конфигов
не автоматизируется, а собрать из этого флот серверов трудно.

`sing-box-agent` — это противоположная форма: ни интерфейса, ни мнения
о хранилище — только маленький авторизованный HTTP API поверх локального
sing-box. Можно запустить в одиночку на одной ноде, а можно собрать
десятки агентов под свой control plane — контракт один и тот же.

## Что делает

- **CRUD пользователей и инбаундов** по HTTP (без SSH).
- **Горячий reload конфига** без перезапуска sing-box.
- **Генерация подписок** в форматах `v2ray`, `clash`, `sing-box`.
- **Синхронизация desired-state** (опционально) — пушите полный конфиг
  из вашего control plane, агент применяет его атомарно с откатом на
  старое при ошибке.
- **Наблюдаемость** — Prometheus-метрики на `/metrics`, JSON-логи,
  `/healthz` и `/readyz`.
- **Безопасность из коробки** — bearer-токен + HMAC-SHA256 подпись с
  защитой от replay (timestamp + nonce).
- **Поддержка протоколов** — VLESS, VMess, Trojan, Shadowsocks,
  Hysteria2, TUIC, ShadowTLS через один и тот же API.

## Быстрый старт (один сервер, без control plane)

```bash
# 1. Собрать агент (sing-box всё ещё нужен отдельно на хосте).
git clone https://github.com/oglenyaboss/sing-box-agent
cd sing-box-agent
make build

# 2. Сгенерировать реальные секреты в примере конфига.
cp deploy/example/agent-config.yaml ./agent-config.yaml
sed -i "s/CHANGE-ME-MIN-32-CHARACTERS-LONG-TOKEN/$(openssl rand -hex 32)/" agent-config.yaml
sed -i "s/CHANGE-ME-HMAC-SECRET-KEY/$(openssl rand -hex 32)/" agent-config.yaml

# 3. Запустить.
./sing-box-agent -config ./agent-config.yaml &

# 4. Проверить.
curl http://localhost:8080/healthz    # -> {"status":"healthy"}
```

Дальше всем CRUD-эндпоинтам хватит `curl` плюс HMAC-заголовков. Схема
подписи и полный контракт API лежат в
[`docs/integration-guide.md`](./docs/integration-guide.md).

Для `systemd`-установки есть готовый скрипт:

```bash
cd deploy/example
cp ../../sing-box-agent .
sudo ./deploy.sh
```

## VLESS + Reality

Пример конфига sing-box с VLESS+Reality (для обхода DPI в странах с
жёсткой фильтрацией) лежит в
[`deploy/example/sing-box-vless-reality.json`](./deploy/example/sing-box-vless-reality.json).
Ключи и short-id нужно сгенерировать командами из того же файла — агент
после запуска подхватит инбаунд как обычный.

## Лицензия и бинарники

Исходники — под MIT. Но **sing-box сам под GPLv3**, а агент линкуется
с ним как Go-библиотека — значит любой *скомпилированный* бинарь
является производной работой GPLv3. Поэтому в релизах этого репозитория
намеренно **нет** готовых бинарников и готовых Docker-образов: сборка
из исходников для себя под GPL свободна ("mere use"), а распространение
скомпилированных артефактов — это уже ваша ответственность по GPL.

## Документация

- [`docs/integration-guide.md`](./docs/integration-guide.md) — схема
  подписи и контракт опционального control plane.
- [`docs/deployment.md`](./docs/deployment.md) — systemd и Docker.
- [`docs/spec.md`](./docs/spec.md) — полная спецификация.

## Безопасность

Если нашли уязвимость — см. [SECURITY.md](./SECURITY.md).

## Лицензия

[MIT](./LICENSE). Подробнее про GPL-совместимость — в английском README
в разделе License.
