CREATE INDEX work_schedule_reminder_start_time_id_idx ON work_schedule_entries (start_time_ticks, id)
    WHERE type = 0 AND start_time_ticks IS NOT NULL AND end_time_ticks IS NOT NULL;
