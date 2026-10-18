# Скриншоты лабораторной работы 4

42 изображения извлечены из документа «лаба4.docx» без изменения содержимого и разрешения. Нумерация соответствует порядку изображений в документе. Основной отчёт ссылается на относящиеся к каждому эксперименту снимки.

Снимки 12–13 показывают промежуточную настройку PostgreSQL, 16–18 и 20–25 — предварительные опыты. Итоговое вытеснение API показано на 30–36, итоговый memory eviction с нагрузкой — на 37–42. Отдельного изображения таблицы KRR в документе нет; используется оригинальный текстовый отчёт `../evidence/krr-before.txt`.

| Файл | Содержание |
| --- | --- |
| [01.png](01.png) | Разнесение API по двум нодам |
| [02.png](02.png) | Исходный k6 при двух worker |
| [03.png](03.png) | SQL исходного прогона: большая задержка фоновой обработки |
| [04.png](04.png) | Масштабирование worker до шести реплик |
| [05.png](05.png) | k6 после увеличения worker до шести |
| [06.png](06.png) | SQL после увеличения worker до шести |
| [07.png](07.png) | Первое применение ресурсов и QoS, PostgreSQL ещё с CPU 100m |
| [08.png](08.png) | Неудачный нагрузочный тест при CPU PostgreSQL 100m |
| [09.png](09.png) | PostgreSQL healthy с CPU request и limit 1 |
| [10.png](10.png) | Успешный k6 после повышения CPU PostgreSQL |
| [11.png](11.png) | Helm revision 18: включение приоритетов |
| [12.png](12.png) | Промежуточное состояние приоритетов: PostgreSQL ещё priority 0 |
| [13.png](13.png) | Промежуточное расхождение priorityClassName CR и существующего пода PostgreSQL |
| [14.png](14.png) | Итоговые приоритеты, QoS и PDB, PostgreSQL priority 10000 |
| [15.png](15.png) | Нехватка ресурсов: batch 6 из 7 готовых реплик |
| [16.png](16.png) | Предварительный preemption тестовым probe: события |
| [17.png](17.png) | Предварительный preemption тестовым probe: жертва и состояния подов |
| [18.png](18.png) | Восстановление после предварительного preemption |
| [19.png](19.png) | Контрольный OOMKilled: лимит памяти 128Mi, нагрузка 256Mi |
| [20.png](20.png) | Предварительный memory eviction отдельного пода при пороге 5850Mi |
| [21.png](21.png) | Восстановление kubelet после предварительного memory eviction |
| [22.png](22.png) | Ранний опыт с probe, не итоговое доказательство API preemption |
| [23.png](23.png) | Ранний опыт с probe: событие и состояние Deployment |
| [24.png](24.png) | Успешный предварительный k6 одновременно с probe |
| [25.png](25.png) | SQL успешного предварительного опыта с probe |
| [26.png](26.png) | ShopApiHighErrorRatio firing, health ok |
| [27.png](27.png) | Предварительный SLA с probe: память, batch и готовность приложений |
| [28.png](28.png) | Предварительный SLA с probe: CPU, память и batch |
| [29.png](29.png) | Предварительный SLA с probe: HTTP-графики |
| [30.png](30.png) | Итоговый preemption самим API: события и готовность приложений |
| [31.png](31.png) | Итоговый API preemption: k6 и период теста |
| [32.png](32.png) | Итоговый API preemption: UID инициаторов сопоставлены с API |
| [33.png](33.png) | Итоговый API preemption: SQL, p95 обработки 159,05 мс |
| [34.png](34.png) | Итоговый API preemption: HTTP-графики за правильный период |
| [35.png](35.png) | Итоговый API preemption: CPU, throttling, память, batch |
| [36.png](36.png) | Итоговый API preemption: память, batch, готовность приложений |
| [37.png](37.png) | Helm memory eviction: нагрузка 512Mi и измерения памяти до порога |
| [38.png](38.png) | Helm memory eviction: два Evicted и восстановление порога |
| [39.png](39.png) | Helm memory eviction: успешный k6, HTTP p95 6,18 мс |
| [40.png](40.png) | Helm memory eviction: HTTP-графики за правильный период |
| [41.png](41.png) | Helm memory eviction: CPU, throttling, память, смена batch-подов |
| [42.png](42.png) | Helm memory eviction: состояния batch и готовность API/worker |
