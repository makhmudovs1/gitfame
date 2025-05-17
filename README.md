## Gitfame

Консольная утилита для подсчёта статистик авторов git репозитория.

```
✗ gitfame --repository=. --extensions='.go,.md' --order-by=lines
Name                   Lines Commits Files
Joe Tsai               12154 92      49
colinnewell            130   1       1
Roger Peppe            59    1       2
A. Ishikawa            36    1       1
Tobias Klauser         33    1       2
178inaba               11    2       4
Kyle Lemons            11    1       1
Dmitri Shuralyov       8     1       2
ferhat elmas           7     1       4
Christian Muehlhaeuser 6     3       4
k.nakada               5     1       3
LMMilewski             5     1       2
Ernest Galbrun         3     1       1
Ross Light             2     1       1
Chris Morrow           1     1       1
Fiisio                 1     1       1
```

### Статистики

* Количество строк
* Количество коммитов
* Количество файлов

Все статистики считаются для состояния репозитория на момент конкретного коммита.

### Интерфейс

Утилита должна печатать результат в stdout.
При использовании невалидного значения флага или любой другой ошибке программа завершается с ненулевым кодом возврата.

Расчёт может занимать довольно длительное время.
Хорошим качеством подобной утилиты является отображение прогресса (вынесенное за флаг).

#### Расчёт

Каждой строке интересующего подмножества файлов репозитория сопоставляется последний коммит, модифицировавший эту строку.
Пустым файлам сопоставляются последние менявшие их коммиты.

После этого для каждого уникального автора, получившегося множества коммитов,
считается количество строк, уникальных коммитов и файлов, которые затрагивали коммиты автора.

Нужную информацию можно получить с помощью команды `git blame`:

```
✗ git blame utf8/README.md
f4640df4 (Fedor Korotkiy 2020-02-26 20:28:52 +0000 1) # utf8*
f4640df4 (Fedor Korotkiy 2020-02-26 20:28:52 +0000 2) 
f4640df4 (Fedor Korotkiy 2020-02-26 20:28:52 +0000 3) Задача объединяет несколько задач на строки и unicode.
f4640df4 (Fedor Korotkiy 2020-02-26 20:28:52 +0000 4) 
f4640df4 (Fedor Korotkiy 2020-02-26 20:28:52 +0000 5) Задача считается решённой, если все подзадачи решены.
```

`git blame` с флагом `--porcelain` (см. `git blame --help`) возвращает информацию в машиночитаемом формате.
Кроме того, этот формат схлопывает соседние строки, относящиеся к одному коммиту,
что может сильно сократить размер результата.

### Флаги

Утилита поддерживает следующий набор флагов:

**--repository** — путь до Git репозитория; по умолчанию текущая директория

**--revision** — указатель на коммит; HEAD по умолчанию

**--order-by** — ключ сортировки результатов; один из `lines` (дефолт), `commits`, `files`.

По умолчанию результаты сортируются по убыванию ключа `(lines, commits, files)`.
При равенстве ключей выше будет автор с лексикографически меньшим именем.
При использовании флага соответствующее поле в ключе перемещается на первое место.

**--use-committer** — булев флаг, заменяющий в расчётах автора (дефолт) на коммиттера

**--format** — формат вывода; один из `tabular` (дефолт), `csv`, `json`, `json-lines`;

`tabular`:
```
Name         Lines Commits Files
Joe Tsai     64    3       2
Ross Light   2     1       1
ferhat elmas 1     1       1
```
Human-readable формат. Для паддинга используется пробел.
см. [text/tabwriter](https://golang.org/pkg/text/tabwriter/).

`csv`:
```
Name,Lines,Commits,Files
Joe Tsai,64,3,2
Ross Light,2,1,1
ferhat elmas,1,1,1
```
[encoding/csv](https://golang.org/pkg/encoding/csv/)

`json`:
```
[{"name":"Joe Tsai","lines":64,"commits":3,"files":2},{"name":"Ross Light","lines":2,"commits":1,"files":1},{"name":"ferhat elmas","lines":1,"commits":1,"files":1}]
```
[encoding/json](https://golang.org/pkg/encoding/json/)

`json-lines`:
```
{"name":"Joe Tsai","lines":64,"commits":3,"files":2}
{"name":"Ross Light","lines":2,"commits":1,"files":1}
{"name":"ferhat elmas","lines":1,"commits":1,"files":1}
```

### Тесты

Команда для запуска тестов:
```
go test -v ./gitfame/test/integration/...
```

В [/tests/integration/testdata/bundles](test/integration/testdata/bundles) лежат запакованные git репозитории.
Каждый интеграционный тест ссылается на какой-нибудь бандл.

Как создать свой bundle? Находясь в git репозитории выполнить
```
git bundle create my.bundle --all
```

Как распаковать bundle? Находясь в пустой директории.
```
git clone /path/to/my.bundle .
```

### Сборка приложения

Как собрать приложение?
```
(cd gitfame/cmd/gitfame && go build .)
```
В `gitfame/cmd/gitfame` появится исполняемый файл с именем `gitfame`.

Как собрать приложение и установить его в `GOPATH/bin`?
```
go install ./gitfame/cmd/gitfame/...
```

Чтобы вызывать установленный бинарь без указания полного пути, нужно добавить `GOPATH/bin` в `PATH`.
```
export PATH=$GOPATH/bin:$PATH
```

После этого `gitfame` будет доступен всюду.