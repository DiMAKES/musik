# Журнал изменений

Все заметные изменения проекта. Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/),
версии — [Semantic Versioning](https://semver.org/lang/ru/). У каждой записи
указаны коммиты, исходный pull request (если есть) и автор.

## [Не выпущено]

### Библиотека, мобильный интерфейс и воспроизведение

Автор — [@DiMAKES](https://github.com/DiMAKES); перенесено из закрытых #15–#21.

#### Добавлено
- Соавторы разбираются при скане (`Сплин / Би-2`), у каждого своя страница; дедуп различает издания и ремастеры — [#15](https://github.com/torwin-job/musik/pull/15), [`8998826`](https://github.com/torwin-job/musik/commit/8998826ec484b071e70e2a8ba0168de7d21c0981) ([@DiMAKES](https://github.com/DiMAKES))
- Инструменты библиотеки: разрезание CUE, чистка дублей, поиск обложек онлайн, скрипты для Windows — [#16](https://github.com/torwin-job/musik/pull/16), [`a259799`](https://github.com/torwin-job/musik/commit/a259799fa29640e357963d416da5c2af2ff381d0) ([@DiMAKES](https://github.com/DiMAKES))
- Мобильная вёрстка, Media Session и иконки PWA — [#17](https://github.com/torwin-job/musik/pull/17), [`f626611`](https://github.com/torwin-job/musik/commit/f6266111691c8fe891e4f78c002fad8f5725848c) ([@DiMAKES](https://github.com/DiMAKES))
- Продолжение воспроизведения на другом устройстве; артист открывается страницей вместо мгновенного запуска — [#18](https://github.com/torwin-job/musik/pull/18), [`b16ad47`](https://github.com/torwin-job/musik/commit/b16ad47029b4abc72502ffe08183e367597c004c) ([@DiMAKES](https://github.com/DiMAKES))
- Обложки из папок альбомов и фото артистов — [#19](https://github.com/torwin-job/musik/pull/19), [`5e72588`](https://github.com/torwin-job/musik/commit/5e7258839058a84a8dd622bfac8048552ea370a6) ([@DiMAKES](https://github.com/DiMAKES))
- Настройки сервера в интерфейсе, QR для приложений, установка одной командой для Windows и Linux — [#20](https://github.com/torwin-job/musik/pull/20), [`aaa91a4`](https://github.com/torwin-job/musik/commit/aaa91a4aa78c3d2903860606d8251f3a41467e08) ([@DiMAKES](https://github.com/DiMAKES))

#### Исправлено
- Поиск обложек в iTunes — [#19](https://github.com/torwin-job/musik/pull/19), [`5e72588`](https://github.com/torwin-job/musik/commit/5e7258839058a84a8dd622bfac8048552ea370a6) ([@DiMAKES](https://github.com/DiMAKES))
- Радио пропускает FLAC с ID3-тегом в начале и треки, которые браузер не может открыть — [#21](https://github.com/torwin-job/musik/pull/21), [`c83ad7f`](https://github.com/torwin-job/musik/commit/c83ad7f6406a0a37f313e93ed668d2e1ce8c36c0) ([@DiMAKES](https://github.com/DiMAKES))
