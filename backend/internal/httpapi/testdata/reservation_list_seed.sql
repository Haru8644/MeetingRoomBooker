-- Same synthetic rows used for the d8fe3b7 capture in reservation_list_csharp.json.
INSERT INTO reservations
    (id, user_id, name, room, number_of_people, type, purpose, series_id,
     date_ticks, start_time_ticks, end_time_ticks, participant_ids, participants, repeat_type, repeat_until_ticks)
VALUES
    (42, 1, '架空の主催者', '会議室 A', 1, '社内', '通常予約', NULL,
     639158688000000000, 639159012000000000, 639159030000000000, '{}', '', 'しない', NULL),
    (7, 2, '架空の利用者', '会議室 B', 3, '来客', '定例訪問', 'synthetic-weekly-series',
     639159552000000000, 639159912000000000, 639159939000000000, '{29,5}', '参加者 29、参加者 5', '毎週', 639165600000000000),
    (8, 2, '架空の利用者', '会議室 B', 3, '来客', '定例訪問', 'synthetic-weekly-series',
     639165600000000000, 639165960000000000, 639165987000000000, '{5}', '参加者 5', '毎週', 639165600000000000),
    (53, 1, '架空の主催者', '会議室 C', 2, '社内', '旧形式の繰り返し', NULL,
     639166464000000001, 639166752001234567, 639166788003400000, '{}', '', '隔週', 639178560000000001);
